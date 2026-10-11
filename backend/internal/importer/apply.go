package importer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	maintdomain "github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
)

// Catalog is the part of the catalog service the importer uses.
type Catalog interface {
	ListCategories(ctx context.Context) ([]catalogdomain.Category, error)
	CreateCategory(ctx context.Context, name, prefix string) (catalogdomain.Category, error)
	ListLocations(ctx context.Context) ([]catalogdomain.Location, error)
	CreateLocation(ctx context.Context, name string, kind catalogdomain.LocationKind, parentID *int64) (catalogdomain.Location, error)
	CreateItem(ctx context.Context, in catalogapp.CreateItemInput) (catalogdomain.Item, error)
}

// Maintenance is the part of the maintenance service the importer uses.
type Maintenance interface {
	Create(ctx context.Context, itemID, userID string, req maintdomain.CreateRequest) (maintdomain.Record, error)
	List(ctx context.Context, f maintdomain.ListFilter, limit, offset int) ([]maintdomain.Record, int, maintdomain.Page, error)
}

// ExistingItem is an item already in the database.
type ExistingItem struct {
	ID           string
	Description  string
	ImportSource string
	Deleted      bool
}

// Existing is what the database already holds for the codes and import
// sources of a plan.
type Existing struct {
	ByCode   map[string]ExistingItem
	BySource map[string]ExistingItem
}

// Lookup finds existing items by legacy code and by import source. It must
// include soft-deleted items, whose codes stay reserved.
type Lookup interface {
	Find(ctx context.Context, codes, sources []string) (Existing, error)
}

// Deps are the application services Apply writes through.
type Deps struct {
	Catalog     Catalog
	Maintenance Maintenance
	Lookup      Lookup
}

// Actor is the user the import acts as (maintenance reporter, opening
// movements). Permissions travel in the context like an API request's.
type Actor struct {
	UserID      string
	Role        string
	Permissions []string
}

// Inspect loads what the database already holds and reconciles the plan with
// it: rows already imported are marked, collisions with other items become
// conflicts, and the category/location tables learn what exists. It writes
// nothing.
func Inspect(ctx context.Context, plan *Plan, d Deps) error {
	var codes, sources []string
	for _, it := range plan.Items {
		if it.Action != ActionCreate {
			continue
		}
		if it.Code != "" {
			codes = append(codes, it.Code)
		}
		sources = append(sources, it.ImportSource())
	}
	ex, err := d.Lookup.Find(ctx, codes, sources)
	if err != nil {
		return fmt.Errorf("look up existing items: %w", err)
	}
	cats, err := d.Catalog.ListCategories(ctx)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	locs, err := d.Catalog.ListLocations(ctx)
	if err != nil {
		return fmt.Errorf("list locations: %w", err)
	}
	plan.Reconcile(ex, cats, locs)
	return nil
}

// Reconcile merges what the database holds into the plan.
func (p *Plan) Reconcile(ex Existing, cats []catalogdomain.Category, locs []catalogdomain.Location) {
	p.DatabaseChecked = true
	for i := range p.Items {
		it := &p.Items[i]
		if it.Action != ActionCreate {
			continue
		}
		if it.Code != "" {
			e, ok := ex.ByCode[it.Code]
			switch {
			case !ok:
			case e.ImportSource == it.ImportSource() || Fold(e.Description) == Fold(it.Description):
				it.Action, it.existingID = ActionAlreadyImported, e.ID
				if e.Deleted {
					it.flag(FlagSuspicious, "already imported but the item was deleted since; left deleted")
				}
			default:
				it.Action = ActionSkipConflict
				it.flag(FlagConflict, "code %s already belongs to another item in the database (%q); this row is skipped, never renumbered", it.Code, e.Description)
			}
			continue
		}
		if e, ok := ex.BySource[it.ImportSource()]; ok {
			it.Action, it.existingID = ActionAlreadyImported, e.ID
		}
	}
	collectNeeds(p)
	have := map[string]bool{}
	for _, c := range cats {
		have[foldKey(c.Name)] = true
	}
	for i := range p.Categories {
		v := have[foldKey(p.Categories[i].Name)]
		p.Categories[i].Exists = &v
	}
	haveLoc := map[string]bool{}
	for _, l := range locs {
		haveLoc[Fold(l.Name)] = true
	}
	for i := range p.Locations {
		v := haveLoc[Fold(p.Locations[i].Name)]
		p.Locations[i].Exists = &v
	}
}

// Apply writes the plan: missing categories and locations, then every row
// marked create, through the catalog and maintenance services. Rows are
// independent: one failing row is reported in the result and does not stop
// the others. The returned error is only for failures that make continuing
// pointless (the catalog tables cannot be read or written).
func Apply(ctx context.Context, plan *Plan, d Deps, actor Actor) (*Result, error) {
	if actor.UserID == "" {
		return nil, errors.New("apply needs the acting user")
	}
	ctx = httpx.WithPrincipal(ctx, httpx.Principal{UserID: actor.UserID, Role: actor.Role, Permissions: actor.Permissions})
	if err := Inspect(ctx, plan, d); err != nil {
		return nil, err
	}
	res := &Result{}
	catIDs, err := ensureCategories(ctx, plan, d.Catalog, res)
	if err != nil {
		return res, err
	}
	locIDs, err := ensureLocations(ctx, plan, d.Catalog, res)
	if err != nil {
		return res, err
	}

	// Legacy codes first so generated codes never take a number a later
	// legacy code needs; both keep workbook order.
	order := make([]int, 0, len(plan.Items))
	for pass := 0; pass < 2; pass++ {
		for i, it := range plan.Items {
			if (it.Code != "") == (pass == 0) {
				order = append(order, i)
			}
		}
	}
	for _, i := range order {
		it := &plan.Items[i]
		switch it.Action {
		case ActionAlreadyImported:
			res.AlreadyImported++
			if it.Failure != nil && it.existingID != "" {
				if err := ensureFailure(ctx, d, actor, *it, it.existingID, res); err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", it.Ref(), err))
				}
			}
		case ActionSkipConflict:
			res.Conflicts++
		case ActionCreate:
			createRow(ctx, d, actor, it, catIDs, locIDs, res)
		}
	}
	return res, nil
}

func ensureCategories(ctx context.Context, plan *Plan, c Catalog, res *Result) (map[string]int64, error) {
	existing, err := c.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	ids := map[string]int64{}
	for _, e := range existing {
		ids[foldKey(e.Name)] = e.ID
	}
	for _, need := range neededCategories(plan) {
		if _, ok := ids[foldKey(need)]; ok {
			continue
		}
		cat, err := c.CreateCategory(ctx, need, categoryPrefixes[need])
		if err != nil {
			return nil, fmt.Errorf("create category %q: %w", need, err)
		}
		ids[foldKey(need)] = cat.ID
		res.CategoriesCreated++
	}
	return ids, nil
}

// neededCategories are the categories of rows that are created now or whose
// failure record may still need creating.
func neededCategories(plan *Plan) []string {
	set := map[string]bool{}
	for _, it := range plan.Items {
		if it.Action == ActionCreate {
			set[it.Category] = true
		}
	}
	var out []string
	for _, n := range categoryOrder {
		if set[n] {
			out = append(out, n)
		}
	}
	return out
}

func ensureLocations(ctx context.Context, plan *Plan, c Catalog, res *Result) (map[string]int64, error) {
	existing, err := c.ListLocations(ctx)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	ids := map[string]int64{}
	for _, e := range existing {
		ids[Fold(e.Name)] = e.ID
	}
	set := map[string]bool{}
	for _, it := range plan.Items {
		if it.Action == ActionCreate && it.Location != "" {
			set[it.Location] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if _, ok := ids[Fold(n)]; ok {
			continue
		}
		kind := catalogdomain.LocationStorage
		if n == LocationCase {
			kind = catalogdomain.LocationContainer
		}
		loc, err := c.CreateLocation(ctx, n, kind, nil)
		if err != nil {
			return nil, fmt.Errorf("create location %q: %w", n, err)
		}
		ids[Fold(n)] = loc.ID
		res.LocationsCreated++
	}
	return ids, nil
}

func createRow(ctx context.Context, d Deps, actor Actor, it *PlannedItem, catIDs, locIDs map[string]int64, res *Result) {
	fail := func(format string, args ...any) {
		it.Action = ActionFailed
		res.Errors = append(res.Errors, fmt.Sprintf("%s (%s): %s", it.Ref(), codeOrGenerated(it.Code), fmt.Sprintf(format, args...)))
	}
	in := catalogapp.CreateItemInput{Code: it.Code, NewItemInput: catalogdomain.NewItemInput{
		Description:       it.Description,
		CategoryID:        catIDs[foldKey(it.Category)],
		Kind:              catalogdomain.Kind(it.Kind),
		Status:            catalogdomain.Status(it.Status),
		LastMaintenanceAt: it.LastMaintenanceAt,
		Attributes:        it.Attributes,
		Notes:             it.Notes,
	}}
	if it.Location != "" {
		id := locIDs[Fold(it.Location)]
		in.LocationID = &id
	}
	if it.Kind == string(catalogdomain.KindConsumable) {
		unit, stock := it.Unit, it.Stock
		in.Unit, in.StockCurrent = &unit, &stock
	}
	created, err := d.Catalog.CreateItem(ctx, in)
	if err != nil {
		if errors.Is(err, catalogdomain.ErrCodeTaken) {
			it.Action = ActionSkipConflict
			it.flag(FlagConflict, "code %s was taken while importing; row skipped", it.Code)
			res.Conflicts++
			return
		}
		fail("%v", err)
		return
	}
	it.Action = ActionCreated
	res.ItemsCreated++
	if created.Kind == catalogdomain.KindAsset {
		res.AssetsCreated++
	} else {
		res.ConsumablesCreated++
		if created.StockCurrent > 0 {
			res.OpeningMovements++
		}
	}
	res.Created = append(res.Created, CreatedItem{Sheet: it.Sheet, Row: it.Row, Code: created.Code, Generated: it.Code == ""})
	if it.Failure != nil {
		if err := openFailure(ctx, d, actor, *it, created.ID, res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s (%s): item created but its failure record was not: %v", it.Ref(), created.Code, err))
		}
	}
}

// ensureFailure opens the failure record of an already-imported item only if
// the item has no maintenance history at all, so a repair closed since the
// first import is never reopened.
func ensureFailure(ctx context.Context, d Deps, actor Actor, it PlannedItem, itemID string, res *Result) error {
	_, total, _, err := d.Maintenance.List(ctx, maintdomain.ListFilter{ItemID: itemID}, 1, 0)
	if err != nil {
		return err
	}
	if total > 0 {
		return nil
	}
	return openFailure(ctx, d, actor, it, itemID, res)
}

func openFailure(ctx context.Context, d Deps, actor Actor, it PlannedItem, itemID string, res *Result) error {
	f := it.Failure
	req := maintdomain.CreateRequest{
		Kind:        maintdomain.KindCorrective,
		Description: f.Description,
		ReportedAt:  f.ReportedAt,
		ItemStatus:  maintdomain.ItemStatus(f.ItemStatus),
	}
	if dest := strings.TrimSpace(f.Destination); dest != "" {
		req.Destination = &dest
	}
	if _, err := d.Maintenance.Create(ctx, itemID, actor.UserID, req); err != nil {
		return err
	}
	res.MaintenanceOpened++
	return nil
}

func codeOrGenerated(code string) string {
	if code == "" {
		return "generated code"
	}
	return code
}
