package domain

import (
	"errors"
	"sort"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/quantity"
)

var ErrInvalidReservationAllocation = errors.New("invalid reservation allocation")

const MaxMaterialRequirement quantity.Quantity = 1_000_000_000_000

// RequiredMaterialQuantity scales a template's per-unit quantity by the
// requested production count without converting through floating point.
func RequiredMaterialQuantity(perUnit quantity.Quantity, units int) (quantity.Quantity, error) {
	if perUnit <= 0 || units < 1 {
		return 0, ErrInvalidReservationAllocation
	}
	total, err := perUnit.Multiply(int64(units))
	if err != nil || total > MaxMaterialRequirement {
		return 0, ErrInvalidReservationAllocation
	}
	return total, nil
}

// ReservationNeed is the currently unallocated requirement for one execution
// and one inventory item. CreatedAt and ExecutionID provide a stable FIFO order.
type ReservationNeed struct {
	ExecutionID int64
	CreatedAt   time.Time
	Required    quantity.Quantity
}

// ReservationAllocation reports both the held quantity and any shortage.
type ReservationAllocation struct {
	ExecutionID int64
	Required    quantity.Quantity
	Reserved    quantity.Quantity
	Deficit     quantity.Quantity
}

// AllocateReservations distributes physical stock to the oldest execution
// needs first. The returned slice is in allocation order and never mutates
// the caller's input.
func AllocateReservations(available quantity.Quantity, needs []ReservationNeed) ([]ReservationAllocation, error) {
	if available < 0 {
		return nil, ErrInvalidReservationAllocation
	}
	ordered := append([]ReservationNeed(nil), needs...)
	seen := make(map[int64]struct{}, len(ordered))
	for _, need := range ordered {
		if need.ExecutionID < 1 || need.Required < 0 {
			return nil, ErrInvalidReservationAllocation
		}
		if _, exists := seen[need.ExecutionID]; exists {
			return nil, ErrInvalidReservationAllocation
		}
		seen[need.ExecutionID] = struct{}{}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ExecutionID < ordered[j].ExecutionID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	allocations := make([]ReservationAllocation, 0, len(ordered))
	remaining := available
	for _, need := range ordered {
		reserved := need.Required
		if reserved > remaining {
			reserved = remaining
		}
		allocations = append(allocations, ReservationAllocation{
			ExecutionID: need.ExecutionID,
			Required:    need.Required,
			Reserved:    reserved,
			Deficit:     need.Required - reserved,
		})
		remaining -= reserved
	}
	return allocations, nil
}
