package importer

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// rawSheet is one worksheet as raw strings: numbers and dates keep their
// stored value (dates are Excel serial numbers). rows[0] is spreadsheet row 1.
type rawSheet struct {
	name string
	rows [][]string
}

func (s rawSheet) cell(row, col int) string {
	if row < 1 || row > len(s.rows) || col < 0 || col >= len(s.rows[row-1]) {
		return ""
	}
	return s.rows[row-1][col]
}

// readWorkbook loads every worksheet of an .xlsx file in workbook order.
func readWorkbook(path string) ([]rawSheet, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open workbook: %w", err)
	}
	defer f.Close()
	var out []rawSheet
	for _, name := range f.GetSheetList() {
		rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, fmt.Errorf("read sheet %q: %w", name, err)
		}
		out = append(out, rawSheet{name: name, rows: rows})
	}
	return out, nil
}
