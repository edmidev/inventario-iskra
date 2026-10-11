package importer

import (
	"fmt"
	"sort"
	"time"

	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
)

// Action is what the importer will do with one workbook row.
type Action string

// Row actions.
const (
	ActionCreate          Action = "create"
	ActionStockRow        Action = "stock_row" // repeat occurrence: becomes a stock row of an earlier item
	ActionSkipConflict    Action = "skip_conflict"
	ActionSkipInvalid     Action = "skip_invalid"
	ActionAlreadyImported Action = "already_imported"
	ActionCreated         Action = "created" // after a successful Apply
	ActionFailed          Action = "failed"  // Apply could not create it
)

// Flag classes: each one becomes a section of the report.
const (
	FlagExtraOccurrence    = "extra_occurrence"
	FlagStockSplit         = "stock_split"
	FlagNeedsStock         = "needs_stock"
	FlagConflict           = "conflict"
	FlagInconsistency      = "inconsistency"
	FlagSuspicious         = "suspicious"
	FlagNoCode             = "no_code"
	FlagFused              = "fused"
	FlagCategoryNormalized = "category_normalized"
	FlagDateConverted      = "date_converted"
	FlagNoFailureRecord    = "no_failure_record"
	FlagInvalid            = "invalid"
)

// Flag is one observation about a row that the reviewer should see.
type Flag struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

// Failure is the open corrective maintenance record created for an item that
// the workbook lists as inactive or under repair.
type Failure struct {
	ReportedAt  *time.Time `json:"reported_at,omitempty"`
	Destination string     `json:"destination,omitempty"`
	ItemStatus  string     `json:"item_status"`
	Description string     `json:"description"`
}

// PlannedItem is one workbook row and what will happen to it.
type PlannedItem struct {
	Sheet             string         `json:"sheet"`
	Row               int            `json:"row"`
	Code              string         `json:"code,omitempty"` // legacy code; empty means one is generated
	Description       string         `json:"description"`
	Kind              string         `json:"kind"`
	Category          string         `json:"category"`
	Status            string         `json:"status"`
	Stock             int            `json:"stock"` // consumables: total over Stocks
	Stocks            []StockRow     `json:"stocks,omitempty"`
	MergedInto        string         `json:"merged_into,omitempty"`
	NeedsStock        []StockRow     `json:"needs_stock,omitempty"`
	Unit              string         `json:"unit,omitempty"`
	Location          string         `json:"location,omitempty"`
	LastMaintenanceAt *time.Time     `json:"last_maintenance_at,omitempty"`
	Attributes        map[string]any `json:"attributes"`
	Notes             string         `json:"notes,omitempty"`
	Failure           *Failure       `json:"failure,omitempty"`
	Action            Action         `json:"action"`
	Flags             []Flag         `json:"flags,omitempty"`

	existingID string // set when the row was matched with an item in the database
	sheetRole  sheetRole
	seq        int // position among the sheet's data rows
	owner      int // for ActionStockRow: index in Plan.Items of the item it feeds
}

// StockRow is the quantity of a consumable at one location and the workbook
// row it was read from.
type StockRow struct {
	Location string `json:"location"`
	Quantity int    `json:"quantity"`
	Sheet    string `json:"sheet"`
	Row      int    `json:"row"`
}

// Ref is a short "sheet row N" reference used in messages.
func (p PlannedItem) Ref() string { return fmt.Sprintf("%s r%d", p.Sheet, p.Row) }

// ImportSource is the provenance tag stored in the item attributes.
func (p PlannedItem) ImportSource() string { return fmt.Sprintf("xlsx:%s:r%d", p.Sheet, p.Row) }

func (p *PlannedItem) flag(class, format string, args ...any) {
	p.Flags = append(p.Flags, Flag{Class: class, Message: fmt.Sprintf(format, args...)})
}

// HasFlag reports whether the row carries a flag of the class.
func (p PlannedItem) HasFlag(class string) bool {
	for _, f := range p.Flags {
		if f.Class == class {
			return true
		}
	}
	return false
}

// PlannedCategory is a category the import needs. Exists is nil until the
// database was consulted.
type PlannedCategory struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	Exists *bool  `json:"exists,omitempty"`
}

// PlannedLocation is a location the import needs.
type PlannedLocation struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Exists *bool  `json:"exists,omitempty"`
}

// SheetSummary describes how one worksheet was understood.
type SheetSummary struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	HeaderRow int    `json:"header_row"`
	RowsRead  int    `json:"rows_read"`
}

// Plan is the full, reviewable outcome of reading the workbook.
type Plan struct {
	File               string            `json:"file"`
	Timezone           string            `json:"timezone"`
	DefaultLocation    string            `json:"default_location"`
	FirstBlockLocation string            `json:"first_block_location"`
	RepeatLocation     string            `json:"repeat_location"`
	Sheets             []SheetSummary    `json:"sheets"`
	Items              []PlannedItem     `json:"items"`
	Categories         []PlannedCategory `json:"categories"`
	Locations          []PlannedLocation `json:"locations"`
	Warnings           []string          `json:"warnings,omitempty"`
	DatabaseChecked    bool              `json:"database_checked"`
}

// Options tune parsing.
type Options struct {
	// DefaultLocation names the storage location of sheet 1 and 3 items; empty
	// means those items get no location.
	DefaultLocation string
	// FirstBlockLocation is where the first occurrence of every sheet 2 item
	// lives (default LocationCase); RepeatLocation is where the second
	// occurrence lives (default DefaultRepeatLocation). They must differ.
	FirstBlockLocation, RepeatLocation string
	// Location is the time zone used to turn dates into timestamps; nil means
	// time.Local.
	Location *time.Location
}

// Parse reads the workbook at path into a plan. It touches nothing else.
func Parse(path string, opts Options) (*Plan, error) {
	sheets, err := readWorkbook(path)
	if err != nil {
		return nil, err
	}
	p, err := buildPlan(sheets, opts)
	if err != nil {
		return nil, err
	}
	p.File = path
	return p, nil
}

func buildPlan(sheets []rawSheet, opts Options) (*Plan, error) {
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	tz := loc.String()
	if loc == time.Local {
		abbr, _ := time.Now().In(loc).Zone()
		tz = "Local (" + abbr + ")"
	}
	plan := &Plan{Timezone: tz, DefaultLocation: Clean(opts.DefaultLocation),
		FirstBlockLocation: Clean(opts.FirstBlockLocation), RepeatLocation: Clean(opts.RepeatLocation)}
	if plan.FirstBlockLocation == "" {
		plan.FirstBlockLocation = LocationCase
	}
	if plan.RepeatLocation == "" {
		plan.RepeatLocation = DefaultRepeatLocation
	}
	if Fold(plan.FirstBlockLocation) == Fold(plan.RepeatLocation) {
		return nil, fmt.Errorf("the first-block and repeat locations must differ (both are %q)", plan.FirstBlockLocation)
	}
	ctx := &parseCtx{plan: plan, loc: loc}
	found := map[sheetRole]bool{}
	for _, s := range sheets {
		role := detectRole(s.name)
		if role == roleUnknown {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("sheet %q is not recognized and was ignored", s.name))
			continue
		}
		if found[role] {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("sheet %q duplicates the role %s and was ignored", s.name, role))
			continue
		}
		found[role] = true
		sum := SheetSummary{Name: s.name, Role: string(role)}
		items, header, err := ctx.parseSheet(s, role)
		if err != nil {
			return nil, err
		}
		sum.HeaderRow, sum.RowsRead = header, len(items)
		plan.Sheets = append(plan.Sheets, sum)
		plan.Items = append(plan.Items, items...)
	}
	if len(plan.Sheets) == 0 {
		return nil, fmt.Errorf("none of the expected worksheets was found (looked for activos, maleta, inactivos)")
	}
	for _, r := range []sheetRole{roleAssets, roleCase, roleInactive} {
		if !found[r] {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("expected a %s worksheet but none was found", r))
		}
	}
	groupConsumables(plan)
	markCodeConflicts(plan)
	propagateOwnerSkips(plan)
	collectNeeds(plan)
	return plan, nil
}

// markCodeConflicts keeps the first row of every legacy code and marks later
// rows that reuse it (and are not exact repeats) as conflicts.
func markCodeConflicts(plan *Plan) {
	first := map[string]*PlannedItem{}
	for i := range plan.Items {
		it := &plan.Items[i]
		if it.Code == "" || it.Action != ActionCreate {
			continue
		}
		if f, ok := first[it.Code]; ok {
			it.Action = ActionSkipConflict
			it.flag(FlagConflict, "code %s is already used by %s (%q); this row is skipped, never renumbered", it.Code, f.Ref(), f.Description)
			continue
		}
		first[it.Code] = it
	}
}

// collectNeeds lists the categories and locations the rows to create need,
// plus the locations of stock rows still to be added to existing items.
func collectNeeds(plan *Plan) {
	cats, locs := map[string]bool{}, map[string]bool{}
	for _, it := range plan.Items {
		for _, r := range it.NeedsStock {
			locs[r.Location] = true
		}
		if it.Action != ActionCreate {
			continue
		}
		cats[it.Category] = true
		if it.Kind == "consumable" {
			for _, r := range it.Stocks {
				if r.Quantity > 0 {
					locs[r.Location] = true
				}
			}
		} else if it.Location != "" {
			locs[it.Location] = true
		}
	}
	plan.Categories = plan.Categories[:0]
	for _, n := range categoryOrder {
		if cats[n] {
			plan.Categories = append(plan.Categories, PlannedCategory{Name: n, Prefix: categoryPrefixes[n]})
		}
	}
	plan.Locations = plan.Locations[:0]
	names := make([]string, 0, len(locs))
	for n := range locs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		plan.Locations = append(plan.Locations, PlannedLocation{Name: n, Kind: string(plan.locationKind(n))})
	}
}

// locationKind is container for the first-block location (the maleta) and
// storage for every other location.
func (p *Plan) locationKind(name string) catalogdomain.LocationKind {
	if Fold(name) == Fold(p.FirstBlockLocation) {
		return catalogdomain.LocationContainer
	}
	return catalogdomain.LocationStorage
}

// Count returns how many rows have the given action.
func (p *Plan) Count(a Action) int {
	n := 0
	for _, it := range p.Items {
		if it.Action == a {
			n++
		}
	}
	return n
}
