package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func workbook(t *testing.T) string {
	t.Helper()
	f := excelize.NewFile()
	_ = f.SetSheetName("Sheet1", "Activos y Disponibles")
	_ = f.SetSheetRow("Activos y Disponibles", "A1", &[]any{nil, "Código", "Descripción", "Categoría", "Estado", "Disponibilidad"})
	_ = f.SetSheetRow("Activos y Disponibles", "A3", &[]any{nil, "ISK-A-001", "martillo", "herramientas", "funcional", "disponible"})
	path := filepath.Join(t.TempDir(), "w.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, func(string) string { return "" })
	return code, out.String(), errb.String()
}

func TestDryRunPrintsTheReportAndNeedsNoDatabase(t *testing.T) {
	code, out, errs := runCLI("--file", workbook(t), "--timezone", "UTC")
	if code != 0 || !strings.Contains(out, "dry-run: nothing was written") || !strings.Contains(out, "ISK-A-001") {
		t.Fatalf("code=%d err=%q out=%q", code, errs, out)
	}
}

func TestReportFileAndJSONFormat(t *testing.T) {
	report := filepath.Join(t.TempDir(), "r.json")
	code, out, errs := runCLI("--file", workbook(t), "--format", "json", "--report", report)
	if code != 0 || out != "" || !strings.Contains(errs, "report written") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	b, err := os.ReadFile(report)
	if err != nil || !json.Valid(b) {
		t.Fatalf("report invalid: %v", err)
	}
}

func TestBadArgumentsAndFatalErrors(t *testing.T) {
	if code, _, _ := runCLI(); code != 2 {
		t.Errorf("missing --file: exit %d, want 2", code)
	}
	if code, _, _ := runCLI("--file", "x", "--format", "xml"); code != 2 {
		t.Errorf("bad format: exit %d, want 2", code)
	}
	if code, _, _ := runCLI("--file", "x", "--timezone", "Mars/Base"); code != 2 {
		t.Errorf("bad timezone: exit %d, want 2", code)
	}
	if code, _, errs := runCLI("--file", filepath.Join(t.TempDir(), "missing.xlsx")); code != 1 || !strings.Contains(errs, "open workbook") {
		t.Errorf("missing file: exit %d err %q, want 1", code, errs)
	}
	bad := filepath.Join(t.TempDir(), "bad.xlsx")
	_ = os.WriteFile(bad, []byte("not a zip"), 0o600)
	if code, _, _ := runCLI("--file", bad); code != 1 {
		t.Errorf("corrupt file: exit %d, want 1", code)
	}
}

func TestLocationFlagsDefaultsAndValidation(t *testing.T) {
	code, out, errs := runCLI("--file", workbook(t), "--timezone", "UTC")
	if code != 0 || !strings.Contains(out, "placed at **`Taller`**") || !strings.Contains(out, "`Maleta y caja de Alejandro` (kind container; `--first-block-location`)") ||
		!strings.Contains(out, "`Taller` (kind storage; `--repeat-location`)") {
		t.Fatalf("defaults not reported: code=%d err=%q\n%s", code, errs, out)
	}
	code, out, _ = runCLI("--file", workbook(t), "--timezone", "UTC", "--default-location", "Bodega", "--first-block-location", "Maleta", "--repeat-location", "Mesa")
	if code != 0 || !strings.Contains(out, "placed at **`Bodega`**") || !strings.Contains(out, "`Maleta` (kind container") || !strings.Contains(out, "`Mesa` (kind storage") {
		t.Fatalf("flags not honored:\n%s", out)
	}
	code, out, _ = runCLI("--file", workbook(t), "--default-location", "")
	if code != 0 || !strings.Contains(out, "**no location**") {
		t.Fatalf("empty default location:\n%s", out)
	}
	if code, _, errs := runCLI("--file", workbook(t), "--first-block-location", "Taller", "--repeat-location", "taller"); code != 2 || !strings.Contains(errs, "must differ") {
		t.Errorf("equal locations: exit %d %q, want 2", code, errs)
	}
}
