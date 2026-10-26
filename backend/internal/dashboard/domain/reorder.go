package domain

import (
	"sort"
	"strings"
)

// TargetStockFactor is how many times the minimum stock a reorder aims for.
const TargetStockFactor = 2

// ReorderCandidate is a low-stock consumable with what is already on order and
// the terms of its latest purchase. Supplier and cost fields are nil when the
// item was never bought from a live, active supplier.
type ReorderCandidate struct {
	ItemID            string
	Code              string
	Description       string
	Unit              string
	StockCurrent      int
	StockMin          int
	PendingOrdered    int
	SupplierID        *int64
	SupplierName      *string
	LastUnitCostMinor *int64
	LastCurrency      *string
}

// ReorderLine is a candidate with the quantity to order.
type ReorderLine struct {
	ReorderCandidate
	SuggestedQty int
}

// ReorderGroup holds the lines to order from one supplier. SupplierID is nil
// for the group of items without a habitual supplier.
type ReorderGroup struct {
	SupplierID   *int64
	SupplierName *string
	Lines        []ReorderLine
}

// SuggestedQty is the quantity that brings stock plus what is already on order
// up to TargetStockFactor times the minimum; never negative.
func SuggestedQty(current, min, pending int) int {
	if q := TargetStockFactor*min - current - pending; q > 0 {
		return q
	}
	return 0
}

// BuildReorderGroups drops candidates already covered by pending orders and
// groups the rest by supplier. Groups are ordered by supplier name with the
// unassigned group last; lines keep the input order.
func BuildReorderGroups(cands []ReorderCandidate) []ReorderGroup {
	groups := []ReorderGroup{}
	index := map[int64]int{}
	var unassigned *ReorderGroup
	for _, c := range cands {
		q := SuggestedQty(c.StockCurrent, c.StockMin, c.PendingOrdered)
		if q == 0 {
			continue
		}
		line := ReorderLine{ReorderCandidate: c, SuggestedQty: q}
		if c.SupplierID == nil {
			if unassigned == nil {
				unassigned = &ReorderGroup{}
			}
			unassigned.Lines = append(unassigned.Lines, line)
			continue
		}
		i, ok := index[*c.SupplierID]
		if !ok {
			i = len(groups)
			index[*c.SupplierID] = i
			groups = append(groups, ReorderGroup{SupplierID: c.SupplierID, SupplierName: c.SupplierName})
		}
		groups[i].Lines = append(groups[i].Lines, line)
	}
	sort.SliceStable(groups, func(a, b int) bool {
		return lowerName(groups[a].SupplierName) < lowerName(groups[b].SupplierName)
	})
	if unassigned != nil {
		groups = append(groups, *unassigned)
	}
	return groups
}

func lowerName(n *string) string {
	if n == nil {
		return ""
	}
	return strings.ToLower(*n)
}
