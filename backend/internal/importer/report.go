package importer

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
)

// Format selects the report encoding.
type Format string

// Report formats.
const (
	FormatMarkdown Format = "md"
	FormatJSON     Format = "json"
)

// WriteReport renders the plan (and, after an apply, its result) to w.
func WriteReport(w io.Writer, plan *Plan, result *Result, format Format) error {
	switch format {
	case FormatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Mode   string  `json:"mode"`
			Plan   *Plan   `json:"plan"`
			Result *Result `json:"result,omitempty"`
		}{modeName(result), plan, result})
	case FormatMarkdown, "":
		_, err := io.WriteString(w, RenderMarkdown(plan, result))
		return err
	}
	return fmt.Errorf("unknown report format %q (use md or json)", format)
}

func modeName(r *Result) string {
	if r == nil {
		return "dry-run"
	}
	return "apply"
}

// sheetStats are the per-sheet counters of the summary table.
type sheetStats struct {
	rows, assets, consumables, create, dup, conflict, invalid, already int
}

func statsFor(plan *Plan, sheet string) sheetStats {
	var s sheetStats
	for _, it := range plan.Items {
		if sheet != "" && it.Sheet != sheet {
			continue
		}
		s.rows++
		switch it.Action {
		case ActionCreate:
			s.create++
		case ActionSkipDuplicate:
			s.dup++
		case ActionSkipConflict:
			s.conflict++
		case ActionSkipInvalid:
			s.invalid++
		case ActionAlreadyImported:
			s.already++
		}
		if it.Action == ActionCreate || it.Action == ActionAlreadyImported {
			if it.Kind == "asset" {
				s.assets++
			} else {
				s.consumables++
			}
		}
	}
	return s
}

// RenderMarkdown renders the review report.
func RenderMarkdown(plan *Plan, result *Result) string {
	var b strings.Builder
	title := "Excel import report (dry-run: nothing was written)"
	if result != nil {
		title = "Excel import report (applied)"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Workbook: `%s`\n", filepath.Base(plan.File))
	fmt.Fprintf(&b, "- Time zone for dates: `%s` (dates become midnight of that day)\n", plan.Timezone)
	if plan.DefaultLocation == "" {
		b.WriteString("- **Default location for sheets 1 and 3: none** (those items get no location)\n")
	} else {
		fmt.Fprintf(&b, "- **Default location for sheets 1 and 3: `%s`** (kind storage; change with `--default-location`)\n", plan.DefaultLocation)
	}
	fmt.Fprintf(&b, "- **Sheet 2 items go to location `%s`** (kind container)\n", LocationCase)
	if plan.DatabaseChecked {
		b.WriteString("- Database consulted read-only: existing codes, categories and locations are taken into account.\n")
	} else {
		b.WriteString("- Database not consulted (offline): categories and locations are assumed missing and code collisions with existing items cannot be detected.\n")
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(&b, "- Warning: %s\n", w)
	}

	b.WriteString("\n## Totals per sheet\n\n")
	b.WriteString("| Sheet | Rows read | Assets | Consumables | To create | Already imported | Skipped: duplicate | Skipped: conflict | Skipped: invalid |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	row := func(name string, s sheetStats) {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d | %d | %d |\n", mdCell(name), s.rows, s.assets, s.consumables, s.create, s.already, s.dup, s.conflict, s.invalid)
	}
	for _, sh := range plan.Sheets {
		row(fmt.Sprintf("%s (header row %d)", sh.Name, sh.HeaderRow), statsFor(plan, sh.Name))
	}
	row("**Total**", statsFor(plan, ""))
	b.WriteString("\nAssets and consumables count the rows that will be created or are already imported.\n")

	b.WriteString("\n## Categories and locations\n\n")
	b.WriteString("| Kind | Name | Code prefix / kind | Will be created |\n|---|---|---|---|\n")
	for _, c := range plan.Categories {
		fmt.Fprintf(&b, "| category | %s | %s | %s |\n", c.Name, c.Prefix, existsText(c.Exists, plan.DatabaseChecked))
	}
	for _, l := range plan.Locations {
		fmt.Fprintf(&b, "| location | %s | %s | %s |\n", mdCell(l.Name), l.Kind, existsText(l.Exists, plan.DatabaseChecked))
	}

	b.WriteString("\n## Items\n\n")
	b.WriteString("| Sheet | Row | Code | Description | Kind | Category | Status | Stock | Location | Action | Notes / flags |\n|---|---:|---|---|---|---|---|---:|---|---|---|\n")
	for _, it := range plan.Items {
		code := it.Code
		if code == "" {
			code = "(generated)"
		}
		stock := fmt.Sprint(it.Stock)
		if it.Kind == "consumable" {
			stock += " " + it.Unit
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			mdCell(shortSheet(it.Sheet)), it.Row, code, mdCell(it.Description), it.Kind, it.Category, it.Status, stock,
			mdCell(it.Location), it.Action, mdCell(flagSummary(it)))
	}

	b.WriteString("\n## Issues to review\n")
	renderIssues(&b, plan)

	b.WriteString("\n## Maintenance records to open (sheet 3)\n\n")
	b.WriteString("Rule: every item of the inactive sheet becomes an asset with an **open corrective maintenance record**: reported_at is the Fecha de Falla at local midnight, the description is the item description (which includes its fault text) and the destination is Destino Inactivos. The item status is `in_repair` when the destination contains \"taller\" (case and accent insensitive), otherwise `inoperative`.\n\n")
	n := 0
	for _, it := range plan.Items {
		if it.Failure == nil {
			continue
		}
		if n == 0 {
			b.WriteString("| Code | Reported at | Destination | Item status | Action |\n|---|---|---|---|---|\n")
		}
		n++
		at := "(import time)"
		if it.Failure.ReportedAt != nil {
			at = it.Failure.ReportedAt.Format("2006-01-02 15:04 MST")
		}
		code := it.Code
		if code == "" {
			code = "(generated)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", code, at, mdCell(it.Failure.Destination), it.Failure.ItemStatus, it.Action)
	}
	if n == 0 {
		b.WriteString("None.\n")
	}

	if result != nil {
		renderResult(&b, result)
	}
	b.WriteString(checklist)
	return b.String()
}

const checklist = `
## Review checklist

- [ ] The default location (and the case location for sheet 2) are the right places for these items.
- [ ] The probable duplicate block(s) are really accidental copies and may be skipped.
- [ ] Every code conflict is understood; conflicting rows are skipped, never renumbered.
- [ ] Inconsistencies between Estado and Disponibilidad are resolved or accepted (availability is derived by the system, not imported).
- [ ] Suspicious values (for example a VACIO on something that is not a cylinder, or a borrowed "prestamo" item) are understood.
- [ ] Rows without a code may receive generated codes (shown as "(generated)").
- [ ] The inoperative items without a failure record are acceptable, or a failure should be reported after the import.
- [ ] The maintenance records of sheet 3 and the last-maintenance dates look right.
- [ ] Re-run with ` + "`--apply`" + ` and ` + "`--user-email`" + ` only after the points above are settled; re-running is safe (nothing is duplicated).
`

func existsText(exists *bool, checked bool) string {
	switch {
	case !checked:
		return "yes (database not consulted)"
	case exists != nil && *exists:
		return "no (already exists)"
	}
	return "yes"
}

func shortSheet(s string) string {
	switch detectRole(s) {
	case roleAssets:
		return "1 Activos"
	case roleCase:
		return "2 Maleta"
	case roleInactive:
		return "3 Inactivos"
	}
	return s
}

func flagSummary(it PlannedItem) string {
	parts := make([]string, 0, len(it.Flags)+1)
	if it.Notes != "" {
		parts = append(parts, "notes: "+it.Notes)
	}
	if lvl, ok := it.Attributes["nivel"]; ok {
		parts = append(parts, fmt.Sprintf("attributes.nivel = %v", lvl))
	}
	for _, f := range it.Flags {
		parts = append(parts, "["+f.Class+"] "+f.Message)
	}
	return strings.Join(parts, "; ")
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	return strings.ReplaceAll(s, "\n", " ")
}

// issue sections in report order.
var issueSections = []struct{ class, title, empty string }{
	{FlagConflict, "Code conflicts", "No code conflicts."},
	{FlagInconsistency, "Inconsistencies between Estado and Disponibilidad", "No inconsistencies."},
	{FlagSuspicious, "Suspicious values", "No suspicious values."},
	{FlagNoFailureRecord, "Inoperative items without a failure record", "None."},
	{FlagNoCode, "Rows without a code (codes will be generated)", "None."},
	{FlagFused, "Rows with the code and description fused in one cell", "None."},
	{FlagCategoryNormalized, "Normalized categories", "None."},
	{FlagDateConverted, "Date conversions", "None."},
	{FlagInvalid, "Invalid rows (skipped)", "None."},
}

func renderIssues(b *strings.Builder, plan *Plan) {
	b.WriteString("\n### Probable duplicate blocks (skipped)\n\n")
	if len(plan.Duplicates) == 0 {
		b.WriteString("No repeated blocks found.\n")
	}
	for _, d := range plan.Duplicates {
		fmt.Fprintf(b, "- Sheet \"%s\": rows %d-%d (%d items) exactly repeat rows %d-%d (same code, description and quantity). They look like an accidental paste and are **skipped**; confirm that they are not real second units.\n",
			d.Sheet, d.FirstRow, d.LastRow, d.Items, d.OfFirstRow, d.OfLastRow)
	}
	for _, sec := range issueSections {
		fmt.Fprintf(b, "\n### %s\n\n", sec.title)
		if sec.class == FlagCategoryNormalized {
			renderGrouped(b, plan, sec.class, sec.empty)
			continue
		}
		n := 0
		for _, it := range plan.Items {
			for _, f := range it.Flags {
				if f.Class != sec.class {
					continue
				}
				n++
				code := it.Code
				if code == "" {
					code = "(no code)"
				}
				fmt.Fprintf(b, "- %s row %d, %s %q: %s\n", shortSheet(it.Sheet), it.Row, code, it.Description, f.Message)
			}
		}
		if n == 0 {
			b.WriteString(sec.empty + "\n")
		}
	}
}

// renderGrouped lists flags of a class grouped by message with their rows.
func renderGrouped(b *strings.Builder, plan *Plan, class, empty string) {
	groups := map[string][]string{}
	var order []string
	for _, it := range plan.Items {
		for _, f := range it.Flags {
			if f.Class != class {
				continue
			}
			if _, ok := groups[f.Message]; !ok {
				order = append(order, f.Message)
			}
			groups[f.Message] = append(groups[f.Message], fmt.Sprintf("%s r%d", shortSheet(it.Sheet), it.Row))
		}
	}
	if len(order) == 0 {
		b.WriteString(empty + "\n")
		return
	}
	sort.Strings(order)
	for _, m := range order {
		fmt.Fprintf(b, "- %s: %d row(s) (%s)\n", m, len(groups[m]), strings.Join(groups[m], ", "))
	}
}

func renderResult(b *strings.Builder, r *Result) {
	b.WriteString("\n## Apply result\n\n")
	fmt.Fprintf(b, "- Categories created: %d\n- Locations created: %d\n", r.CategoriesCreated, r.LocationsCreated)
	fmt.Fprintf(b, "- Items created: %d (assets %d, consumables %d)\n", r.ItemsCreated, r.AssetsCreated, r.ConsumablesCreated)
	fmt.Fprintf(b, "- Opening movements recorded: %d\n", r.OpeningMovements)
	fmt.Fprintf(b, "- Maintenance records opened: %d\n", r.MaintenanceOpened)
	fmt.Fprintf(b, "- Already imported (skipped): %d\n- Conflicts (skipped): %d\n", r.AlreadyImported, r.Conflicts)
	fmt.Fprintf(b, "- Errors: %d\n", len(r.Errors))
	for _, e := range r.Errors {
		fmt.Fprintf(b, "  - %s\n", e)
	}
	if len(r.Created) > 0 {
		b.WriteString("\nGenerated codes:\n\n")
		n := 0
		for _, c := range r.Created {
			if c.Generated {
				fmt.Fprintf(b, "- %s r%d -> %s\n", shortSheet(c.Sheet), c.Row, c.Code)
				n++
			}
		}
		if n == 0 {
			b.WriteString("None.\n")
		}
	}
}
