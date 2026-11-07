package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/quantity"
)

func TestAllocateReservationsOldestFirstAndExposeDeficit(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := old.Add(time.Hour)
	needs := []ReservationNeed{
		{ExecutionID: 2, CreatedAt: newer, Required: quantity.Quantity(5000)},
		{ExecutionID: 1, CreatedAt: old, Required: quantity.Quantity(4000)},
	}

	got, err := AllocateReservations(quantity.Quantity(6000), needs)
	if err != nil {
		t.Fatalf("AllocateReservations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d allocations, want 2", len(got))
	}
	if got[0].ExecutionID != 1 || got[0].Reserved != quantity.Quantity(4000) || got[0].Deficit != 0 {
		t.Fatalf("oldest allocation = %+v, want full 4 units", got[0])
	}
	if got[1].ExecutionID != 2 || got[1].Reserved != quantity.Quantity(2000) || got[1].Deficit != quantity.Quantity(3000) {
		t.Fatalf("newer allocation = %+v, want 2 reserved and 3 deficit", got[1])
	}
}

func TestRequiredMaterialQuantityScalesPerUnitSnapshot(t *testing.T) {
	got, err := RequiredMaterialQuantity(quantity.Quantity(1250), 3)
	if err != nil {
		t.Fatalf("RequiredMaterialQuantity() error = %v", err)
	}
	if got != quantity.Quantity(3750) {
		t.Fatalf("required quantity = %s, want 3.75", got)
	}
}

func TestRequiredMaterialQuantityRejectsStockOverflow(t *testing.T) {
	if _, err := RequiredMaterialQuantity(quantity.Quantity(MaxMaterialRequirement), 2); !errors.Is(err, ErrInvalidReservationAllocation) {
		t.Fatalf("oversized material requirement error = %v", err)
	}
}

func TestAllocateReservationsUsesExecutionIDToBreakTimestampTies(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	needs := []ReservationNeed{
		{ExecutionID: 9, CreatedAt: created, Required: quantity.Quantity(2000)},
		{ExecutionID: 3, CreatedAt: created, Required: quantity.Quantity(2000)},
	}

	got, err := AllocateReservations(quantity.Quantity(2000), needs)
	if err != nil {
		t.Fatalf("AllocateReservations() error = %v", err)
	}
	if got[0].ExecutionID != 3 || got[0].Reserved != quantity.Quantity(2000) || got[1].Reserved != 0 {
		t.Fatalf("tie allocation = %+v, want execution 3 first", got)
	}
}
