package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	return ve.Fields
}

func TestNormalizeCategory(t *testing.T) {
	n, p, err := NormalizeCategory("  Herramientas ", " her ")
	if err != nil || n != "Herramientas" || p != "HER" {
		t.Fatalf("got %q %q %v", n, p, err)
	}
	for _, prefix := range []string{"", "A", "ABCDEFGHI", "AB-C", "ñu"} {
		if _, _, err := NormalizeCategory("x", prefix); fieldsOf(t, err)["code_prefix"] == "" {
			t.Errorf("prefix %q accepted", prefix)
		}
	}
	if _, _, err := NormalizeCategory("   ", "AB"); fieldsOf(t, err)["name"] == "" {
		t.Error("blank name accepted")
	}
}

func TestValidateCode(t *testing.T) {
	for _, ok := range []string{"ISK-HER-001", "ISK-A1", "ISK-X-Y-Z-9"} {
		if err := ValidateCode(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "isk-her-001", "ISK-", "ISK--1", "HER-001", "ISK-HER_001", "ISK-" + strings.Repeat("A", 70)} {
		if err := ValidateCode(bad); fieldsOf(t, err)["code"] == "" {
			t.Errorf("%q accepted", bad)
		}
	}
	if got := GenerateCode("HER", 7); got != "ISK-HER-007" {
		t.Errorf("got %s", got)
	}
	if got := GenerateCode("HER", 1234); got != "ISK-HER-1234" {
		t.Errorf("got %s", got)
	}
}

func TestNewItemAsset(t *testing.T) {
	it, err := NewItem(NewItemInput{Description: " Taladro ", CategoryID: 1, Kind: KindAsset})
	if err != nil {
		t.Fatal(err)
	}
	if it.StockCurrent != 1 || it.StockMin != 0 || it.Unit != nil || it.Status != StatusFunctional || it.Description != "Taladro" {
		t.Fatalf("asset = %+v", it)
	}
	if it.Attributes == nil {
		t.Error("attributes should default to an empty object")
	}
	_, err = NewItem(NewItemInput{Description: "x", CategoryID: 1, Kind: KindAsset,
		Unit: ptr("caja"), StockCurrent: ptr(5), StockMin: 2})
	f := fieldsOf(t, err)
	if f["unit"] == "" || f["stock_current"] == "" || f["stock_min"] == "" {
		t.Fatalf("fields = %v", f)
	}
}

func TestNewItemConsumable(t *testing.T) {
	it, err := NewItem(NewItemInput{Description: "Cinta", CategoryID: 1, Kind: KindConsumable,
		Unit: ptr(" rollo "), StockCurrent: ptr(10), StockMin: 3})
	if err != nil || *it.Unit != "rollo" || it.StockCurrent != 10 || it.StockMin != 3 {
		t.Fatalf("got %+v %v", it, err)
	}
	if _, err := NewItem(NewItemInput{Description: "Cinta", CategoryID: 1, Kind: KindConsumable}); fieldsOf(t, err)["unit"] == "" {
		t.Error("unit should be required")
	}
	_, err = NewItem(NewItemInput{Description: "Cinta", CategoryID: 1, Kind: KindConsumable, Unit: ptr("u"),
		StockCurrent: ptr(-1), StockMin: -2})
	if f := fieldsOf(t, err); f["stock_current"] == "" || f["stock_min"] == "" {
		t.Errorf("fields = %v", f)
	}
	zero, err := NewItem(NewItemInput{Description: "Cinta", CategoryID: 1, Kind: KindConsumable, Unit: ptr("u")})
	if err != nil || zero.StockCurrent != 0 {
		t.Fatalf("default opening balance: %+v %v", zero, err)
	}
}

func TestNewItemCommonValidation(t *testing.T) {
	_, err := NewItem(NewItemInput{Description: "", Kind: "gadget", Status: "broken",
		MaintenanceIntervalDays: ptr(0), Notes: strings.Repeat("n", MaxNotesLen+1),
		Attributes: map[string]any{"blob": strings.Repeat("x", MaxAttributesSize)}})
	f := fieldsOf(t, err)
	for _, k := range []string{"description", "category_id", "kind", "status", "maintenance_interval_days", "notes", "attributes"} {
		if f[k] == "" {
			t.Errorf("missing field error %q in %v", k, f)
		}
	}
}

func TestAvailabilityIsDerivedFromStatus(t *testing.T) {
	cases := map[Status]Availability{StatusFunctional: Available, StatusInoperative: Unavailable, StatusInRepair: Unavailable}
	for st, want := range cases {
		if got := (Item{Status: st}).Availability(); got != want {
			t.Errorf("%s: %s", st, got)
		}
	}
	if DeriveAvailability(AvailabilityInputs{Status: StatusFunctional, ActiveLoan: true}) != Unavailable {
		t.Error("an active loan must make a functional item unavailable")
	}
}

func TestAvailabilityReflectsActiveLoan(t *testing.T) {
	loan := &LoanInfo{AssigneeLocationID: 7, AssigneeName: "Alejandro"}
	for st, want := range map[Status]Availability{StatusFunctional: Unavailable, StatusInoperative: Unavailable, StatusInRepair: Unavailable} {
		it := Item{Status: st, Loan: loan}
		if !it.OnLoan() || it.Availability() != want {
			t.Errorf("%s on loan: availability %s", st, it.Availability())
		}
	}
	if (Item{Status: StatusFunctional}).OnLoan() {
		t.Error("item without loan reported on loan")
	}
}

func TestLoanOverdue(t *testing.T) {
	now := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Minute), now.Add(time.Minute)
	if !(LoanInfo{DueAt: &past}).Overdue(now) || (LoanInfo{DueAt: &future}).Overdue(now) || (LoanInfo{}).Overdue(now) {
		t.Error("overdue only when a due date lapsed")
	}
}

func TestApplyWhileOnLoan(t *testing.T) {
	loc := int64(7)
	it := Item{Kind: KindAsset, Status: StatusFunctional, Description: "Drill", CategoryID: 1, LocationID: &loc, Loan: &LoanInfo{AssigneeLocationID: 7}}
	for name, p := range map[string]ItemPatch{
		"move":  {LocationID: Nullable[int64]{Set: true, Value: 9}},
		"clear": {LocationID: Nullable[int64]{Set: true, Null: true}},
	} {
		if _, err := it.Apply(p); !errors.Is(err, ErrItemOnLoan) {
			t.Errorf("%s: err = %v, want ErrItemOnLoan", name, err)
		}
	}
	if _, err := it.Apply(ItemPatch{LocationID: Nullable[int64]{Set: true, Value: 7}}); err != nil {
		t.Errorf("re-sending the same location must be accepted: %v", err)
	}
	desc := "Drill 18V"
	if out, err := it.Apply(ItemPatch{Description: &desc}); err != nil || out.Loan == nil {
		t.Errorf("other edits stay allowed and keep the loan: %v", err)
	}
}

func TestLowStock(t *testing.T) {
	c := func(cur, min int) Item { return Item{Kind: KindConsumable, StockCurrent: cur, StockMin: min} }
	if !c(2, 2).LowStock() || !c(0, 1).LowStock() {
		t.Error("at or below minimum is low")
	}
	if c(3, 2).LowStock() || c(0, 0).LowStock() {
		t.Error("above minimum, or no minimum, is not low")
	}
	if (Item{Kind: KindAsset, StockCurrent: 1}).LowStock() {
		t.Error("assets are never low stock")
	}
}

func TestApplyPatch(t *testing.T) {
	asset := Item{Code: "ISK-A-001", Kind: KindAsset, Description: "Taladro", CategoryID: 1, Status: StatusFunctional, StockCurrent: 1,
		LocationID: ptr(int64(4)), MaintenanceIntervalDays: ptr(90), Attributes: map[string]any{"color": "rojo"}}

	var loc Nullable[int64]
	_ = json.Unmarshal([]byte("null"), &loc)
	var interval Nullable[int]
	_ = json.Unmarshal([]byte("30"), &interval)
	got, err := asset.Apply(ItemPatch{Description: ptr(" Taladro 2 "), Status: ptr(StatusInRepair), LocationID: loc,
		MaintenanceIntervalDays: interval, Attributes: map[string]any{"marca": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != "Taladro 2" || got.Status != StatusInRepair || got.LocationID != nil ||
		*got.MaintenanceIntervalDays != 30 || got.Attributes["marca"] != "x" || got.Attributes["color"] != nil {
		t.Fatalf("got %+v", got)
	}
	if asset.Status != StatusFunctional || asset.LocationID == nil {
		t.Fatal("Apply must not mutate the receiver")
	}

	untouched, err := asset.Apply(ItemPatch{})
	if err != nil || untouched.LocationID == nil || *untouched.LocationID != 4 {
		t.Fatalf("empty patch changed the item: %+v %v", untouched, err)
	}
}

func TestApplyRejectsImmutableFields(t *testing.T) {
	it := Item{Code: "ISK-A-001", Kind: KindAsset, Description: "x", CategoryID: 1, Status: StatusFunctional, StockCurrent: 1}
	_, err := it.Apply(ItemPatch{Code: ptr("ISK-A-002"), Kind: ptr(KindConsumable), StockCurrent: ptr(9)})
	f := fieldsOf(t, err)
	if f["code"] == "" || f["kind"] == "" || f["stock_current"] == "" {
		t.Fatalf("fields = %v", f)
	}
	// Sending the same code/kind back is harmless.
	if _, err := it.Apply(ItemPatch{Code: ptr("ISK-A-001"), Kind: ptr(KindAsset)}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyKindRules(t *testing.T) {
	asset := Item{Code: "ISK-A-001", Kind: KindAsset, Description: "x", CategoryID: 1, Status: StatusFunctional, StockCurrent: 1}
	_, err := asset.Apply(ItemPatch{Unit: ptr("caja"), StockMin: ptr(3)})
	if f := fieldsOf(t, err); f["unit"] == "" || f["stock_min"] == "" {
		t.Fatalf("fields = %v", f)
	}

	cons := Item{Code: "ISK-C-001", Kind: KindConsumable, Description: "x", CategoryID: 1, Status: StatusFunctional, Unit: ptr("rollo"), StockCurrent: 5}
	got, err := cons.Apply(ItemPatch{Unit: ptr("caja"), StockMin: ptr(2)})
	if err != nil || *got.Unit != "caja" || got.StockMin != 2 || got.StockCurrent != 5 {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := cons.Apply(ItemPatch{Unit: ptr("  ")}); fieldsOf(t, err)["unit"] == "" {
		t.Error("consumable unit cannot be cleared")
	}
}

func TestCheckNoCycle(t *testing.T) {
	p := func(v int64) *int64 { return &v }
	parents := map[int64]*int64{1: nil, 2: p(1), 3: p(2)}
	if err := CheckNoCycle(1, p(3), parents); !errors.Is(err, ErrLocationCycle) {
		t.Errorf("1 under 3 should cycle: %v", err)
	}
	if err := CheckNoCycle(2, p(2), parents); !errors.Is(err, ErrLocationCycle) {
		t.Errorf("self parent should cycle: %v", err)
	}
	if err := CheckNoCycle(3, p(1), parents); err != nil {
		t.Errorf("valid re-parent rejected: %v", err)
	}
	if err := CheckNoCycle(3, nil, parents); err != nil {
		t.Errorf("clearing parent rejected: %v", err)
	}
}

func TestFilterNormalize(t *testing.T) {
	f, err := ItemFilter{Q: "  taladro "}.Normalize()
	if err != nil || f.Limit != DefaultLimit || f.Sort != SortCode || f.Q != "taladro" {
		t.Fatalf("defaults: %+v %v", f, err)
	}
	_, err = ItemFilter{Status: "x", Kind: "y", Availability: "z", Sort: "color", Limit: 101, Offset: -1}.Normalize()
	fl := fieldsOf(t, err)
	for _, k := range []string{"status", "kind", "availability", "sort", "limit", "offset"} {
		if fl[k] == "" {
			t.Errorf("missing %q in %v", k, fl)
		}
	}
}

func TestFilterMatches(t *testing.T) {
	it := Item{Code: "ISK-HER-001", Description: "Taladro percutor", CategoryID: 2, Kind: KindAsset,
		Status: StatusFunctional, LocationID: ptr(int64(5))}
	yes := []ItemFilter{
		{}, {Q: "her-001"}, {Q: "PERCUTOR"}, {CategoryID: ptr(int64(2))}, {Status: StatusFunctional},
		{Kind: KindAsset}, {Availability: Available}, {LocationID: ptr(int64(5))},
	}
	for i, f := range yes {
		if !f.Matches(it) {
			t.Errorf("yes[%d] should match", i)
		}
	}
	no := []ItemFilter{
		{Q: "martillo"}, {CategoryID: ptr(int64(3))}, {Status: StatusInRepair}, {Kind: KindConsumable},
		{Availability: Unavailable}, {LocationID: ptr(int64(6))}, {LowStock: true},
	}
	for i, f := range no {
		if f.Matches(it) {
			t.Errorf("no[%d] should not match", i)
		}
	}
}

func TestFilterMatchesTreatsLoanAsUnavailable(t *testing.T) {
	it := Item{Code: "ISK-HER-001", Description: "Drill", Kind: KindAsset, Status: StatusFunctional, Loan: &LoanInfo{AssigneeLocationID: 7}}
	if (ItemFilter{Availability: Available}).Matches(it) || !(ItemFilter{Availability: Unavailable}).Matches(it) {
		t.Error("a functional item on loan must be unavailable")
	}
	if !(ItemFilter{Status: StatusFunctional}).Matches(it) {
		t.Error("the status filter keeps ignoring loans")
	}
}

func TestMaintenanceDueAndOverdue(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	created := now.Add(-100 * day)
	last := now.Add(-10 * day)
	tests := []struct {
		name    string
		item    Item
		due     *time.Time
		overdue bool
	}{
		{name: "no interval", item: Item{Kind: KindAsset, CreatedAt: created}},
		{name: "consumable ignores interval", item: Item{Kind: KindConsumable, CreatedAt: created, MaintenanceIntervalDays: ptr(30)}},
		{name: "never serviced counts from creation (overdue)", item: Item{Kind: KindAsset, CreatedAt: created, MaintenanceIntervalDays: ptr(30)}, due: ptr(created.Add(30 * day)), overdue: true},
		{name: "never serviced, not yet due", item: Item{Kind: KindAsset, CreatedAt: created, MaintenanceIntervalDays: ptr(365)}, due: ptr(created.Add(365 * day))},
		{name: "last maintenance wins over creation", item: Item{Kind: KindAsset, CreatedAt: created, LastMaintenanceAt: &last, MaintenanceIntervalDays: ptr(30)}, due: ptr(last.Add(30 * day))},
		{name: "due exactly now is not overdue", item: Item{Kind: KindAsset, CreatedAt: created, LastMaintenanceAt: ptr(now.Add(-30 * day)), MaintenanceIntervalDays: ptr(30)}, due: ptr(now)},
	}
	for _, tc := range tests {
		got := tc.item.MaintenanceDueAt()
		if (tc.due == nil) != (got == nil) || (got != nil && !got.Equal(*tc.due)) {
			t.Errorf("%s: due = %v, want %v", tc.name, got, tc.due)
		}
		if o := tc.item.MaintenanceOverdue(now); o != tc.overdue {
			t.Errorf("%s: overdue = %v, want %v", tc.name, o, tc.overdue)
		}
	}
}

func TestNewItemConsumableStockForms(t *testing.T) {
	base := NewItemInput{Description: "Tornillos", CategoryID: 1, Kind: KindConsumable, Unit: ptr("u")}

	t.Run("multi-location opening balance", func(t *testing.T) {
		in := base
		in.Stocks = []StockInput{{LocationID: 1, Quantity: 5}, {LocationID: 2, Quantity: 5}}
		it, err := NewItem(in)
		if err != nil || it.StockCurrent != 10 || len(it.Stocks) != 2 || it.LocationID != nil {
			t.Fatalf("got %+v %v", it, err)
		}
	})
	t.Run("legacy form with a location", func(t *testing.T) {
		in := base
		in.StockCurrent, in.LocationID = ptr(7), ptr(int64(3))
		it, err := NewItem(in)
		if err != nil || it.StockCurrent != 7 || len(it.Stocks) != 1 || it.Stocks[0].LocationID != 3 || it.Stocks[0].Quantity != 7 || it.LocationID != nil {
			t.Fatalf("got %+v %v", it, err)
		}
	})
	t.Run("legacy form without a location targets the default location", func(t *testing.T) {
		in := base
		in.StockCurrent = ptr(4)
		it, err := NewItem(in)
		if err != nil || len(it.Stocks) != 1 || it.Stocks[0].LocationID != 0 {
			t.Fatalf("got %+v %v", it, err)
		}
	})
	t.Run("no stock means no rows", func(t *testing.T) {
		in := base
		in.StockCurrent, in.LocationID = ptr(0), ptr(int64(3))
		it, err := NewItem(in)
		if err != nil || len(it.Stocks) != 0 || it.LocationID != nil {
			t.Fatalf("got %+v %v", it, err)
		}
	})
	t.Run("both forms at once are rejected", func(t *testing.T) {
		for _, in := range []NewItemInput{
			{Stocks: []StockInput{{LocationID: 1, Quantity: 1}}, StockCurrent: ptr(1)},
			{Stocks: []StockInput{{LocationID: 1, Quantity: 1}}, LocationID: ptr(int64(1))},
			{Stocks: []StockInput{}, StockCurrent: ptr(0)},
		} {
			in.Description, in.CategoryID, in.Kind, in.Unit = base.Description, base.CategoryID, base.Kind, base.Unit
			if _, err := NewItem(in); fieldsOf(t, err)["stocks"] == "" {
				t.Errorf("%+v accepted", in)
			}
		}
	})
	t.Run("invalid rows", func(t *testing.T) {
		in := base
		in.Stocks = []StockInput{{LocationID: 0, Quantity: 1}, {LocationID: 2, Quantity: 0}, {LocationID: 2, Quantity: 3}, {LocationID: 4, Quantity: MaxStock}}
		f := fieldsOf(t, mustErr(NewItem(in)))
		for _, k := range []string{"stocks[0].location_id", "stocks[1].quantity", "stocks[2].location_id", "stocks"} {
			if f[k] == "" {
				t.Errorf("missing error for %s: %v", k, f)
			}
		}
	})
	t.Run("assets have no per-location stock", func(t *testing.T) {
		_, err := NewItem(NewItemInput{Description: "Taladro", CategoryID: 1, Kind: KindAsset, Stocks: []StockInput{{LocationID: 1, Quantity: 1}}})
		if fieldsOf(t, err)["stocks"] == "" {
			t.Error("asset with stocks accepted")
		}
	})
}

func mustErr(_ Item, err error) error { return err }

func TestApplyRejectsLocationChangesOnConsumables(t *testing.T) {
	cons := Item{Code: "ISK-CON-001", Kind: KindConsumable, Description: "Tornillos", CategoryID: 1, Status: StatusFunctional,
		Unit: ptr("u"), StockCurrent: 4, Stocks: []ItemStock{{LocationID: 1, Quantity: 4}}}
	if _, err := cons.Apply(ItemPatch{LocationID: Nullable[int64]{Set: true, Value: 2}}); !errors.Is(err, ErrNotApplicable) {
		t.Errorf("setting a location: %v", err)
	}
	// Clearing is a no-op for a consumable (it never has one), so it is tolerated.
	if got, err := cons.Apply(ItemPatch{LocationID: Nullable[int64]{Set: true, Null: true}, Notes: ptr("x")}); err != nil || got.Notes != "x" || got.LocationID != nil {
		t.Errorf("null location: %+v %v", got, err)
	}
	asset := Item{Code: "ISK-HER-001", Kind: KindAsset, Description: "Taladro", CategoryID: 1, Status: StatusFunctional, StockCurrent: 1}
	if got, err := asset.Apply(ItemPatch{LocationID: Nullable[int64]{Set: true, Value: 2}}); err != nil || got.LocationID == nil || *got.LocationID != 2 {
		t.Errorf("assets can still be edited: %+v %v", got, err)
	}
}

func TestFilterMatchesConsumablesByStockLocation(t *testing.T) {
	it := Item{Kind: KindConsumable, Description: "Tornillos", StockCurrent: 10,
		Stocks: []ItemStock{{LocationID: 1, Quantity: 5}, {LocationID: 2, Quantity: 5}}}
	for _, loc := range []int64{1, 2} {
		if !(ItemFilter{LocationID: ptr(loc)}).Matches(it) {
			t.Errorf("location %d should match", loc)
		}
	}
	if (ItemFilter{LocationID: ptr(int64(3))}).Matches(it) {
		t.Error("location 3 should not match")
	}
}
