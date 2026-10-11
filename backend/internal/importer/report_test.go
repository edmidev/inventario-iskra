package importer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// issuesPlan reproduces every issue class in one synthetic workbook.
func issuesPlan(t *testing.T) *Plan {
	t.Helper()
	return parseSynthetic(t, Options{DefaultLocation: "Almacén principal"},
		assetsSheet(
			[]any{nil, "ISK-A-001", "taladro de banco", "maquinaria/", "VACIO", "disponible", nil, 46283},
			[]any{nil, "ISK-A-002", "cortadora", "maquinaria", "inoperativo", "no disponible"},
			[]any{nil, "ISK-A-003", "martillo", "herramientas", "funcional", "no disponible"},
			[]any{nil, "ISK-A-003", "otro martillo", "herramientas", "funcional", "disponible"},
			[]any{nil, nil, "sin codigo", "herramientas", "funcional", "disponible"},
		),
		caseSheet(
			[]any{"ISK-M16-PAR      pinza", nil, nil, nil, nil, 1},
			[]any{nil, "CONSUMIBLES"},
			[]any{"ISK-MD1-X", "amarras", nil, nil, nil, 5},
			[]any{"cables", "electricidad", nil, nil, 3},
			[]any{"ISK-MD1-X", "amarras", nil, nil, nil, 5},
			[]any{"cables", "electricidad", nil, nil, 3},
			[]any{"ISK-MD1-X", "amarras", nil, nil, nil, 5},
			[]any{"solo taller", nil, nil, nil, 4},
		),
		inactiveSheet(
			[]any{"ISK-HER-013", "Llave (fisura)", "Herramientas", "No Funcional", "28/08/2026", "Taller interno"},
		))
}

func TestMarkdownReportHasASectionForEveryIssueClass(t *testing.T) {
	md := RenderMarkdown(issuesPlan(t), nil)
	for _, want := range []string{
		"# Excel import report (dry-run: nothing was written)",
		"## Totals per sheet", "| Activos y Disponibles (header row 1) | 5 |", "**Total**",
		"## Categories and locations", "| category | Maquinaria | MAQ |", "| location | Maleta y caja de Alejandro | container |",
		"## Items", "(generated)", "ISK-M16-PAR",
		"Assumption to confirm (sheets 1 and 3)", "placed at **`Almacén principal`**",
		"## Stock split across locations", "- With two stock rows (Maleta y caja de Alejandro and Taller): 2",
		"- Only in the repeated block (Taller, informational): 1", "- Third or later occurrences (issues, skipped): 1",
		"| ISK-MD1-X | amarras | 5 | 5 | 10 | r5 + r7 | create |", "| (generated) | cables | 3 | 3 | 6 | r6 + r8 | create |",
		"| (generated) | solo taller | - | 4 | 4 | r10 | create |",
		"### Stock per location (after the import)", "| Taller | 0 | 3 | 12 |",
		"### Third or later occurrences of a consumable", "already has stock rows from",
		"| Became a stock row |", "stock row of Maleta y caja de alejandro r5",
		"### Code conflicts", "code ISK-A-003 is already used by",
		"### Inconsistencies between Estado and Disponibilidad", "no disponible but Estado is functional",
		"### Suspicious values", "VACIO on something that is not a cylinder",
		"### Inoperative items without a failure record", "consider reporting a failure",
		"### Rows without a code", "### Rows with the code and description fused",
		"### Normalized categories", `"maquinaria/" normalized to Maquinaria: 1 row(s)`,
		"### Date conversions", "converted to 2026-09-18", "converted to 2026-08-28",
		"## Maintenance records to open (sheet 3)", "in_repair", "2026-08-28 00:00",
		"## Review checklist", "- [ ]",
		"Database not consulted (offline)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report misses %q", want)
		}
	}
	// A repeat occurrence does not repeat the observations its first row carries.
	if n := strings.Count(md, `"cables": no code`); n != 1 {
		t.Errorf("uncoded repeat listed %d times in the no-code section, want 1", n)
	}
	if strings.Contains(md, "## Apply result") {
		t.Error("dry-run report must not have an apply result")
	}
}

func TestMarkdownReportSaysWhenThereAreNoIssues(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet([]any{nil, "ISK-A-001", "uno", "herramientas", "funcional", "disponible"}))
	md := RenderMarkdown(p, nil)
	for _, want := range []string{"- Third or later occurrences (issues, skipped): 0", "No code conflicts.", "No inconsistencies.", "No suspicious values."} {
		if !strings.Contains(md, want) {
			t.Errorf("report misses %q", want)
		}
	}
}

func TestMarkdownReportEscapesTableCells(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet([]any{nil, "ISK-A-001", "a|b", "herramientas", "funcional", "disponible"}))
	if md := RenderMarkdown(p, nil); !strings.Contains(md, `a\|b`) {
		t.Fatal("pipe not escaped")
	}
}

func TestJSONReportIsValidAndCarriesThePlan(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteReport(&buf, issuesPlan(t), nil, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Mode string
		Plan struct {
			Items      []map[string]any
			Categories []map[string]any
		}
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got.Mode != "dry-run" || len(got.Plan.Items) == 0 || len(got.Plan.Categories) == 0 {
		t.Fatalf("json = %+v", got)
	}
	if err := WriteReport(&buf, issuesPlan(t), nil, "xml"); err == nil {
		t.Fatal("unknown format must fail")
	}
}
