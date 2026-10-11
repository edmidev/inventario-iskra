package importer

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// sheetSpec is a synthetic worksheet: rows[i] is spreadsheet row i+1 and a
// nil row is blank. Values may be string, int, float64 or time.Time.
type sheetSpec struct {
	name string
	rows [][]any
}

// writeWorkbook builds an .xlsx file in a temp dir from synthetic sheets.
func writeWorkbook(t *testing.T, sheets ...sheetSpec) string {
	t.Helper()
	f := excelize.NewFile()
	for i, s := range sheets {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", s.name); err != nil {
				t.Fatal(err)
			}
		} else if _, err := f.NewSheet(s.name); err != nil {
			t.Fatal(err)
		}
		for r, row := range s.rows {
			for c, v := range row {
				if v == nil {
					continue
				}
				cell, err := excelize.CoordinatesToCellName(c+1, r+1)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.SetCellValue(s.name, cell, v); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	path := filepath.Join(t.TempDir(), "synthetic.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

var utc = time.UTC

func parseSynthetic(t *testing.T, opts Options, sheets ...sheetSpec) *Plan {
	t.Helper()
	if opts.Location == nil {
		opts.Location = utc
	}
	p, err := Parse(writeWorkbook(t, sheets...), opts)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}

func byRow(p *Plan, sheet string, row int) PlannedItem {
	for _, it := range p.Items {
		if it.Sheet == sheet && it.Row == row {
			return it
		}
	}
	panic("no such row")
}

func hasFlag(it PlannedItem, class string) bool { return it.HasFlag(class) }

// Names of the synthetic sheets (same shapes as the real workbook).
const (
	sheetAssets   = "Activos y Disponibles"
	sheetCase     = "Maleta y caja de alejandro"
	sheetInactive = "Inactivos y Reparación"
)

func assetsSheet(rows ...[]any) sheetSpec {
	head := [][]any{
		{nil, "Código", "Descripción", "Categoría", "Estado", "Disponibilidad", "Stock Actual", "Último Mantenimiento"},
		nil,
	}
	return sheetSpec{sheetAssets, append(head, rows...)}
}

func caseSheet(rows ...[]any) sheetSpec {
	head := [][]any{
		nil,
		{"Código", "Descripción", "Categoría", "Estado", "Disponibilidad", "Stock Actual", "Último Mantenimiento"},
	}
	return sheetSpec{sheetCase, append(head, rows...)}
}

func inactiveSheet(rows ...[]any) sheetSpec {
	head := [][]any{{"Código", "Descripción", "Categoría", "Estado", "Fecha de Falla", "Destino Inactivos"}}
	return sheetSpec{sheetInactive, append(head, rows...)}
}
