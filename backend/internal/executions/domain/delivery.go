package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrCustomerNotAssigned = errors.New("delivery customer is not assigned to this execution")
	ErrUnitUnavailable     = errors.New("finished unit does not belong to this execution")
	ErrUnitDelivered       = errors.New("finished unit was already delivered")
	ErrDeliveryReplay      = errors.New("delivery request id was reused with different terms")
)

type DeliveryInput struct {
	RequestID  string  `json:"request_id"`
	CustomerID int64   `json:"customer_id"`
	UnitIDs    []int64 `json:"unit_ids"`
}

func (in DeliveryInput) Normalize() (DeliveryInput, error) {
	in.RequestID = strings.ToLower(strings.TrimSpace(in.RequestID))
	in.UnitIDs = append([]int64{}, in.UnitIDs...)
	fields := map[string]string{}
	if !validUUID(in.RequestID) {
		fields["request_id"] = "must be a valid UUID"
	}
	if in.CustomerID < 1 {
		fields["customer_id"] = "must reference an assigned customer"
	}
	if len(in.UnitIDs) == 0 {
		fields["unit_ids"] = "at least one finished unit is required"
	}
	sort.Slice(in.UnitIDs, func(i, j int) bool { return in.UnitIDs[i] < in.UnitIDs[j] })
	for i, id := range in.UnitIDs {
		if id < 1 || (i > 0 && id == in.UnitIDs[i-1]) {
			fields[fmt.Sprintf("unit_ids[%d]", i)] = "must be a distinct finished unit id"
		}
	}
	if len(fields) > 0 {
		return DeliveryInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

type DeliveryReceipt struct {
	ID           int64     `json:"id"`
	Number       string    `json:"number"`
	ExecutionID  int64     `json:"execution_id"`
	CustomerID   int64     `json:"customer_id"`
	CustomerName string    `json:"customer_name"`
	ActorID      string    `json:"actor_id"`
	UnitCodes    []string  `json:"unit_codes"`
	DeliveredAt  time.Time `json:"delivered_at"`
}

type FinishedStockFilter struct {
	Status        string
	Q             string
	Limit, Offset int
}

func (f FinishedStockFilter) Normalize() (FinishedStockFilter, error) {
	f.Status = strings.TrimSpace(f.Status)
	f.Q = strings.TrimSpace(f.Q)
	if f.Limit == 0 {
		f.Limit = 25
	}
	fields := map[string]string{}
	if f.Status != "" && f.Status != "available" && f.Status != "delivered" {
		fields["status"] = "must be available or delivered"
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		fields["limit"] = "must be 1 to 100 and offset nonnegative"
	}
	if len(fields) > 0 {
		return f, &ValidationError{Fields: fields}
	}
	return f, nil
}

type FinishedStockRow struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	ExecutionID  int64      `json:"execution_id"`
	ProjectName  string     `json:"project_name"`
	ProducedAt   time.Time  `json:"produced_at"`
	Status       string     `json:"status"`
	Receipt      string     `json:"receipt,omitempty"`
	CustomerName string     `json:"customer_name,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
}
