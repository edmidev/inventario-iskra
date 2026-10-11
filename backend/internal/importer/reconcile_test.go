package importer

import "testing"

func TestReconcileAddsOnlyStockRowsAtLocationsTheItemNeverHad(t *testing.T) {
	row := []any{"ISK-MD1-A", "uno", nil, nil, nil, 4}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows([][]any{row}, [][]any{row})...))
	src := byRow(p, sheetCase, 4).ImportSource()

	// Imported earlier with only the maleta row: the workshop row is missing.
	p.Reconcile(Existing{ByCode: map[string]ExistingItem{"ISK-MD1-A": {ID: "x", Description: "uno", ImportSource: src, Locations: []string{LocationCase}}}}, nil, nil)
	it := byRow(p, sheetCase, 4)
	if it.Action != ActionAlreadyImported || len(it.NeedsStock) != 1 || it.NeedsStock[0].Location != "Taller" || !hasFlag(it, FlagNeedsStock) {
		t.Fatalf("item = %+v", it)
	}
	if len(p.Locations) != 1 || p.Locations[0].Name != "Taller" {
		t.Fatalf("locations = %+v", p.Locations)
	}

	// Stock consumed to zero at the workshop leaves a movement there: nothing to add.
	q := parseSynthetic(t, Options{}, caseSheet(consumableRows([][]any{row}, [][]any{row})...))
	q.Reconcile(Existing{ByCode: map[string]ExistingItem{"ISK-MD1-A": {ID: "x", Description: "uno", ImportSource: src, Locations: []string{LocationCase, "taller"}}}}, nil, nil)
	if it := byRow(q, sheetCase, 4); len(it.NeedsStock) != 0 || hasFlag(it, FlagNeedsStock) {
		t.Fatalf("item = %+v", it)
	}
}

func TestReconcileCodeConflictSkipsTheRepeatOccurrenceToo(t *testing.T) {
	row := []any{"ISK-MD1-A", "uno", nil, nil, nil, 4}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows([][]any{row}, [][]any{row})...))
	p.Reconcile(Existing{ByCode: map[string]ExistingItem{"ISK-MD1-A": {ID: "x", Description: "another thing"}}}, nil, nil)
	if byRow(p, sheetCase, 4).Action != ActionSkipConflict || byRow(p, sheetCase, 5).Action != ActionSkipConflict {
		t.Fatalf("actions = %s %s", byRow(p, sheetCase, 4).Action, byRow(p, sheetCase, 5).Action)
	}
}
