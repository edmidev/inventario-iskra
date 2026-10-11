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
	rows, assets, consumables, create, stockRows, conflict, invalid, already int
}

func statsFor(plan *Plan, sheet string) sheetStats {
	var s sheetStats
	for _, it := range plan.Items {
		if sheet != "" && it.Sheet != sheet {
			continue
		}
		s.rows++
		switch it.Action {
		case ActionCreate, ActionCreated:
			s.create++
		case ActionStockRow:
			s.stockRows++
		case ActionSkipConflict:
			s.conflict++
		case ActionSkipInvalid:
			s.invalid++
		case ActionAlreadyImported:
			s.already++
		}
		if it.Action == ActionCreate || it.Action == ActionCreated || it.Action == ActionAlreadyImported {
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
	b.WriteString("\n> **Assumption to confirm (sheets 1 and 3).** The workbook does not say where the items of \"Activos y Disponibles\" and \"Inactivos y Reparación\" are kept. ")
	if plan.DefaultLocation == "" {
		b.WriteString("They get **no location** (`--default-location` is empty). Pass `--default-location <name>` to place them.\n\n")
	} else {
		fmt.Fprintf(&b, "They are placed at **`%s`** (kind storage; `--default-location`, an assumption, not data from the workbook). Change it with `--default-location` before applying; an empty value means no location.\n\n", plan.DefaultLocation)
	}
	fmt.Fprintf(&b, "- **Sheet 2, first block** (first occurrence of each consumable, and its tools) -> location `%s` (kind container; `--first-block-location`)\n", plan.FirstBlockLocation)
	fmt.Fprintf(&b, "- **Sheet 2, repeated block** (second occurrence of the same consumable) -> a second stock row at location `%s` (kind storage; `--repeat-location`)\n", plan.RepeatLocation)
	if plan.DatabaseChecked {
		b.WriteString("- Database consulted read-only: existing codes, categories and locations are taken into account.\n")
	} else {
		b.WriteString("- Database not consulted (offline): categories and locations are assumed missing and code collisions with existing items cannot be detected.\n")
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(&b, "- Warning: %s\n", w)
	}

	b.WriteString("\n## Totals per sheet\n\n")
	b.WriteString("| Sheet | Rows read | Assets | Consumables | To create / created | Already imported | Became a stock row | Skipped: conflict / extra | Skipped: invalid |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	row := func(name string, s sheetStats) {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d | %d | %d |\n", mdCell(name), s.rows, s.assets, s.consumables, s.create, s.already, s.stockRows, s.conflict, s.invalid)
	}
	for _, sh := range plan.Sheets {
		row(fmt.Sprintf("%s (header row %d)", sh.Name, sh.HeaderRow), statsFor(plan, sh.Name))
	}
	row("**Total**", statsFor(plan, ""))
	b.WriteString("\nAssets and consumables count the rows that will be created or are already imported. \"Became a stock row\" counts repeat occurrences of a consumable that add a stock row to the item of its first occurrence (they are not items of their own).\n")

	b.WriteString("\n## Categories and locations\n\n")
	b.WriteString("| Kind | Name | Code prefix / kind | Will be created |\n|---|---|---|---|\n")
	for _, c := range plan.Categories {
		fmt.Fprintf(&b, "| category | %s | %s | %s |\n", c.Name, c.Prefix, existsText(c.Exists, plan.DatabaseChecked))
	}
	for _, l := range plan.Locations {
		fmt.Fprintf(&b, "| location | %s | %s | %s |\n", mdCell(l.Name), l.Kind, existsText(l.Exists, plan.DatabaseChecked))
	}

	renderStockSplit(&b, plan)

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
		loc := it.Location
		if len(it.Stocks) > 0 {
			names := make([]string, len(it.Stocks))
			for i, s := range it.Stocks {
				names[i] = fmt.Sprintf("%s: %d", s.Location, s.Quantity)
			}
			loc = strings.Join(names, " + ")
		}
		if it.MergedInto != "" {
			loc = "stock row of " + it.MergedInto
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			mdCell(shortSheet(it.Sheet)), it.Row, code, mdCell(it.Description), it.Kind, it.Category, it.Status, stock,
			mdCell(loc), it.Action, mdCell(flagSummary(it)))
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

- [ ] **Assumption:** the default location for sheets 1 and 3 is where those items really are (the workbook does not say).
- [ ] The first block of sheet 2 is what is in the maleta and the repeated block is what is in the workshop, with the quantities as written (see "Stock split across locations").
- [ ] Items listed three times, or only in the repeated block, are understood.
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
	{FlagExtraOccurrence, "Third or later occurrences of a consumable (skipped, not merged)", "None."},
	{FlagNeedsStock, "Stock rows to add to items already in the database", "None."},
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
	for _, sec := range issueSections {
		fmt.Fprintf(b, "\n### %s\n\n", sec.title)
		if sec.class == FlagCategoryNormalized {
			renderGrouped(b, plan, sec.class, sec.empty)
			continue
		}
		n := 0
		for _, it := range plan.Items {
			for _, f := range it.Flags {
				if f.Class != sec.class || skippedAsRepeat(it, f) {
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
			if f.Class != class || skippedAsRepeat(it, f) {
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
	fmt.Fprintf(b, "- Opening movements recorded: %d (items with two stock rows: %d)\n", r.OpeningMovements, r.SplitItems)
	fmt.Fprintf(b, "- Stock rows added to items already in the database: %d\n", r.StockRowsAdded)
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

// skippedAsRepeat hides the observations of a repeat occurrence that its first
// occurrence already carries (no code, normalized category).
func skippedAsRepeat(it PlannedItem, f Flag) bool {
	return it.Action == ActionStockRow && (f.Class == FlagNoCode || f.Class == FlagCategoryNormalized)
}

// splitStats counts how the consumables are spread over the two blocks.
type splitStats struct {
	items, two, onlyFirst, onlySecond, extra int
}

func (p *Plan) splitStats() splitStats {
	var s splitStats
	for _, it := range p.Items {
		if it.HasFlag(FlagExtraOccurrence) {
			s.extra++
		}
		if it.Kind != "consumable" || len(it.Stocks) == 0 || it.Action == ActionSkipConflict || it.Action == ActionSkipInvalid {
			continue
		}
		s.items++
		switch {
		case len(it.Stocks) == 2:
			s.two++
		case Fold(it.Stocks[0].Location) == Fold(p.RepeatLocation):
			s.onlySecond++
		default:
			s.onlyFirst++
		}
	}
	return s
}

// stockAt returns the planned quantity of a consumable at a location and
// whether it has a stock row there.
func (it PlannedItem) stockAt(loc string) (int, bool) {
	for _, r := range it.Stocks {
		if Fold(r.Location) == Fold(loc) {
			return r.Quantity, true
		}
	}
	return 0, false
}

func renderStockSplit(b *strings.Builder, plan *Plan) {
	st := plan.splitStats()
	b.WriteString("\n## Stock split across locations\n\n")
	fmt.Fprintf(b, "The same consumable listed in both blocks of sheet 2 becomes **one item with one stock row per location** (quantities as written, added together in the total). Matching is by legacy code, or by normalized description when there is no code.\n\n")
	fmt.Fprintf(b, "- Consumable items: %d\n- With two stock rows (%s and %s): %d\n- Only in the first block (%s): %d\n- Only in the repeated block (%s, informational): %d\n- Third or later occurrences (issues, skipped): %d\n\n",
		st.items, plan.FirstBlockLocation, plan.RepeatLocation, st.two, plan.FirstBlockLocation, st.onlyFirst, plan.RepeatLocation, st.onlySecond, st.extra)
	if st.items > 0 {
		fmt.Fprintf(b, "| Code | Description | Qty in %s | Qty in %s | Total | Source rows | Action |\n|---|---|---:|---:|---:|---|---|\n", mdCell(plan.FirstBlockLocation), mdCell(plan.RepeatLocation))
		for _, it := range plan.Items {
			if it.Kind != "consumable" || len(it.Stocks) == 0 || it.Action == ActionSkipConflict || it.Action == ActionSkipInvalid {
				continue
			}
			code := it.Code
			if code == "" {
				code = "(generated)"
			}
			cell := func(loc string) string {
				if q, ok := it.stockAt(loc); ok {
					return fmt.Sprint(q)
				}
				return "-"
			}
			rows := make([]string, len(it.Stocks))
			for i, r := range it.Stocks {
				rows[i] = fmt.Sprintf("r%d", r.Row)
			}
			fmt.Fprintf(b, "| %s | %s | %s | %s | %d | %s | %s |\n", code, mdCell(it.Description), cell(plan.FirstBlockLocation), cell(plan.RepeatLocation), it.Stock, strings.Join(rows, " + "), it.Action)
		}
	}
	b.WriteString("\n### Stock per location (after the import)\n\n")
	type tot struct{ assets, items, units int }
	totals := map[string]*tot{}
	var order []string
	get := func(loc string) *tot {
		k := Fold(loc)
		if totals[k] == nil {
			totals[k] = &tot{}
			order = append(order, loc)
		}
		return totals[k]
	}
	for _, it := range plan.Items {
		switch it.Action {
		case ActionCreate, ActionCreated, ActionAlreadyImported:
		default:
			continue
		}
		if it.Kind == "consumable" {
			for _, r := range it.Stocks {
				if r.Quantity > 0 {
					t := get(r.Location)
					t.items++
					t.units += r.Quantity
				}
			}
		} else if it.Location != "" {
			get(it.Location).assets++
		}
	}
	if len(order) == 0 {
		b.WriteString("None.\n")
		return
	}
	sort.Strings(order)
	b.WriteString("| Location | Assets | Consumable items with stock | Consumable units |\n|---|---:|---:|---:|\n")
	for _, loc := range order {
		t := totals[Fold(loc)]
		fmt.Fprintf(b, "| %s | %d | %d | %d |\n", mdCell(loc), t.assets, t.items, t.units)
	}
}
