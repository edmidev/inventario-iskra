package domain_test

import (
	"testing"

	"github.com/edgar/inventario/backend/internal/dashboard/domain"
)

func TestSuggestedQty(t *testing.T) {
	cases := []struct{ cur, min, pending, want int }{
		{2, 10, 0, 18},  // 2x10 - 2
		{10, 10, 0, 10}, // at the minimum
		{2, 10, 18, 0},  // fully covered by pending orders
		{2, 10, 30, 0},  // over-covered never goes negative
		{0, 5, 4, 6},
	}
	for _, c := range cases {
		if got := domain.SuggestedQty(c.cur, c.min, c.pending); got != c.want {
			t.Errorf("SuggestedQty(%d,%d,%d) = %d, want %d", c.cur, c.min, c.pending, got, c.want)
		}
	}
}

func i64(v int64) *int64   { return &v }
func str(v string) *string { return &v }

func TestBuildReorderGroups(t *testing.T) {
	cands := []domain.ReorderCandidate{
		{ItemID: "a", Code: "A", StockCurrent: 1, StockMin: 5, SupplierID: i64(2), SupplierName: str("zeta")},
		{ItemID: "b", Code: "B", StockCurrent: 1, StockMin: 5},
		{ItemID: "c", Code: "C", StockCurrent: 1, StockMin: 5, SupplierID: i64(1), SupplierName: str("Alfa")},
		{ItemID: "d", Code: "D", StockCurrent: 1, StockMin: 5, PendingOrdered: 9}, // covered: omitted
		{ItemID: "e", Code: "E", StockCurrent: 0, StockMin: 3, SupplierID: i64(2), SupplierName: str("zeta")},
	}
	got := domain.BuildReorderGroups(cands)
	if len(got) != 3 {
		t.Fatalf("groups = %d, want 3: %+v", len(got), got)
	}
	if *got[0].SupplierName != "Alfa" || len(got[0].Lines) != 1 || got[0].Lines[0].ItemID != "c" {
		t.Errorf("first group = %+v", got[0])
	}
	if *got[1].SupplierName != "zeta" || len(got[1].Lines) != 2 || got[1].Lines[0].ItemID != "a" || got[1].Lines[1].ItemID != "e" {
		t.Errorf("second group = %+v", got[1])
	}
	if got[2].SupplierID != nil || len(got[2].Lines) != 1 || got[2].Lines[0].ItemID != "b" {
		t.Errorf("unassigned group must be last: %+v", got[2])
	}
	if got[0].Lines[0].SuggestedQty != 9 || got[1].Lines[1].SuggestedQty != 6 {
		t.Errorf("suggested quantities wrong: %+v", got)
	}
}

func TestBuildReorderGroupsNothingToOrder(t *testing.T) {
	got := domain.BuildReorderGroups(nil)
	if got == nil || len(got) != 0 {
		t.Fatalf("want empty non-nil slice, got %#v", got)
	}
}
