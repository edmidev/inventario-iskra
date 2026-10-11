package importer

import (
	"fmt"
	"strings"
	"time"
)

type sheetRole string

const (
	roleUnknown  sheetRole = ""
	roleAssets   sheetRole = "assets"
	roleCase     sheetRole = "case"
	roleInactive sheetRole = "inactive"
)

// detectRole recognizes the three worksheets by name, ignoring case and
// accents. "inactivos" must be tested before "activos" because it contains it.
func detectRole(name string) sheetRole {
	n := Fold(name)
	switch {
	case strings.Contains(n, "inactivo") || strings.Contains(n, "reparacion"):
		return roleInactive
	case strings.Contains(n, "maleta") || strings.Contains(n, "caja"):
		return roleCase
	case strings.Contains(n, "activo"):
		return roleAssets
	}
	return roleUnknown
}

// columns maps header labels to zero-based column indexes; -1 when absent.
type columns struct {
	code, desc, category, state, availability, stock, lastMaint, failDate, destination int
}

// headerLabels maps a folded header label (prefix match) to a column setter.
func (c *columns) set(label string, idx int) {
	switch {
	case strings.HasPrefix(label, "codigo"):
		c.code = idx
	case strings.HasPrefix(label, "descripcion"):
		c.desc = idx
	case strings.HasPrefix(label, "categoria"):
		c.category = idx
	case strings.HasPrefix(label, "estado"):
		c.state = idx
	case strings.HasPrefix(label, "disponibilidad"):
		c.availability = idx
	case strings.HasPrefix(label, "stock"):
		c.stock = idx
	case strings.HasPrefix(label, "ultimo mantenimiento"):
		c.lastMaint = idx
	case strings.HasPrefix(label, "fecha"):
		c.failDate = idx
	case strings.HasPrefix(label, "destino"):
		c.destination = idx
	}
}

// findHeader looks for the header row in the first rows: the first one that
// has both a Código and a Descripción label.
func findHeader(s rawSheet) (row int, c columns, err error) {
	for r := 1; r <= len(s.rows) && r <= 15; r++ {
		cols := columns{-1, -1, -1, -1, -1, -1, -1, -1, -1}
		for i, cell := range s.rows[r-1] {
			cols.set(Fold(cell), i)
		}
		if cols.code >= 0 && cols.desc >= 0 {
			return r, cols, nil
		}
	}
	return 0, c, fmt.Errorf("sheet %q: no header row with Código and Descripción found in its first 15 rows", s.name)
}

type parseCtx struct {
	plan *Plan
	loc  *time.Location
}

func (p *parseCtx) parseSheet(s rawSheet, role sheetRole) ([]PlannedItem, int, error) {
	header, cols, err := findHeader(s)
	if err != nil {
		return nil, 0, err
	}
	var items []PlannedItem
	add := func(it PlannedItem) {
		it.Sheet, it.sheetRole, it.seq = s.name, role, len(items)
		if it.Attributes == nil {
			it.Attributes = map[string]any{}
		}
		it.Attributes["import_source"] = it.ImportSource()
		if it.Action == "" {
			it.Action = ActionCreate
		}
		items = append(items, it)
	}
	inConsumables := false
	for r := header + 1; r <= len(s.rows); r++ {
		row := rowView{s: s, r: r}
		if row.blank() {
			continue
		}
		var it PlannedItem
		switch role {
		case roleAssets:
			it = p.assetRow(row, cols)
		case roleCase:
			if row.isSection("consumibles") {
				inConsumables = true
				continue
			}
			it = p.caseRow(row, cols, inConsumables)
		case roleInactive:
			it = p.inactiveRow(row, cols)
		}
		it.Row = r
		add(it)
	}
	return items, header, nil
}

// rowView gives trimmed access to one spreadsheet row.
type rowView struct {
	s rawSheet
	r int
}

func (v rowView) get(col int) string {
	if col < 0 {
		return ""
	}
	return strings.TrimSpace(v.s.cell(v.r, col))
}

func (v rowView) blank() bool {
	for _, c := range v.s.rows[v.r-1] {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// isSection reports whether the row is only a section label such as
// CONSUMIBLES: its single non-empty cell folds to label.
func (v rowView) isSection(label string) bool {
	n := 0
	match := false
	for _, c := range v.s.rows[v.r-1] {
		if strings.TrimSpace(c) == "" {
			continue
		}
		n++
		match = Fold(c) == label
	}
	return n == 1 && match
}
