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
