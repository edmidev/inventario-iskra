package importer

import "fmt"

// groupConsumables turns the repeated block of a sheet into stock rows.
//
// In the workbook the same consumable is listed twice: the first block is
// what is in the maleta and the repeat is what is in the workshop. One item
// is identified by its legacy code when it has one, otherwise by its
// normalized description. The first occurrence becomes the item with a stock
// row at the first-block location, the second one adds a stock row at the
// repeat location. A third occurrence is an issue and is skipped.
//
// The repeated block starts at the first row that repeats an earlier item;
// an item first seen from that row on exists only in the repeat block.
func groupConsumables(plan *Plan) {
	boundary := map[string]int{} // sheet -> seq of the first repeating row
	seen := map[string]bool{}
	for _, it := range plan.Items {
		if !isGroupable(it) {
			continue
		}
		k := it.Sheet + "|" + identity(it)
		if seen[k] {
			if _, ok := boundary[it.Sheet]; !ok {
				boundary[it.Sheet] = it.seq
			}
		}
		seen[k] = true
	}

	owners := map[string]int{} // identity -> index in plan.Items
	for i := range plan.Items {
		it := &plan.Items[i]
		if !isGroupable(*it) {
			continue
		}
		k := it.Sheet + "|" + identity(*it)
		oi, ok := owners[k]
		if !ok {
			loc := plan.FirstBlockLocation
			if b, has := boundary[it.Sheet]; has && it.seq >= b {
				loc = plan.RepeatLocation
				it.flag(FlagStockSplit, "only listed in the repeated block; its single stock row is at %s", loc)
			}
			it.Location = loc
			it.Stocks = []StockRow{{Location: loc, Quantity: it.Stock, Sheet: it.Sheet, Row: it.Row}}
			owners[k] = i
			continue
		}
		o := &plan.Items[oi]
		switch {
		case it.Code != "" && Fold(it.Description) != Fold(o.Description):
			it.Action = ActionSkipConflict
			it.flag(FlagConflict, "code %s is already used by %s (%q); this row is skipped, never renumbered", it.Code, o.Ref(), o.Description)
		case len(o.Stocks) != 1 || Fold(o.Stocks[0].Location) != Fold(plan.FirstBlockLocation):
			it.Action = ActionSkipConflict
			it.flag(FlagExtraOccurrence, "%s already has stock rows from %s; this occurrence is not merged (third or later listing of the same item)", describe(*o), o.Ref())
		default:
			it.Action, it.owner, it.MergedInto = ActionStockRow, oi, o.Ref()
			o.Stocks = append(o.Stocks, StockRow{Location: plan.RepeatLocation, Quantity: it.Stock, Sheet: it.Sheet, Row: it.Row})
			o.Attributes["import_source_repeat"] = it.ImportSource()
			if it.Category != o.Category {
				it.flag(FlagSuspicious, "category %s differs from the first occurrence (%s); the first one is kept", it.Category, o.Category)
			}
		}
	}
	for i := range plan.Items {
		it := &plan.Items[i]
		if it.Kind != "consumable" || len(it.Stocks) == 0 {
			continue
		}
		it.Stock = 0
		for _, r := range it.Stocks {
			it.Stock += r.Quantity
		}
	}
}

// isGroupable reports whether the row may be grouped: a consumable that is
// still to be created.
func isGroupable(it PlannedItem) bool {
	return it.Kind == "consumable" && it.Action == ActionCreate
}

// identity is the grouping key: the legacy code, else the folded description.
func identity(it PlannedItem) string {
	if it.Code != "" {
		return "code:" + it.Code
	}
	return "desc:" + Fold(it.Description)
}

func describe(it PlannedItem) string {
	if it.Code != "" {
		return it.Code
	}
	return fmt.Sprintf("%q", it.Description)
}

// propagateOwnerSkips skips the stock-row occurrences of an item that is
// itself skipped (code conflict), so no orphan stock rows remain.
func propagateOwnerSkips(plan *Plan) {
	for i := range plan.Items {
		it := &plan.Items[i]
		if it.Action != ActionStockRow {
			continue
		}
		if o := plan.Items[it.owner]; o.Action == ActionSkipConflict {
			it.Action = ActionSkipConflict
			it.flag(FlagConflict, "its item %s is skipped; this occurrence is skipped with it", o.Ref())
		}
	}
}
