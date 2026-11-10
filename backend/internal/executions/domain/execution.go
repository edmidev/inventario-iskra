// Package domain defines immutable manufacturing execution snapshots.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/quantity"
)

var (
	ErrInvalidInput           = errors.New("invalid execution input")
	ErrInvalidState           = errors.New("invalid execution state transition")
	ErrNotFound               = errors.New("execution not found")
	ErrInactiveAssignment     = errors.New("execution assignment must reference active users and customers")
	ErrMaterialUnavailable    = errors.New("execution material must reference an active consumable item")
	ErrMaterialAdditionClosed = errors.New("execution no longer accepts material additions")
	ErrToolUnavailable        = errors.New("execution tool must reference an inventory asset")
	ErrToolAdditionClosed     = errors.New("execution no longer accepts tool additions")
	ErrProductionLimit        = errors.New("production exceeds planned units")
	ErrProductionReplay       = errors.New("production request id was reused with different terms")
	ErrProductionIncomplete   = errors.New("planned production is not complete")
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string        { return ErrInvalidInput.Error() }
func (e *ValidationError) Is(target error) bool { return target == ErrInvalidInput }

type Status string

const (
	StatusRegistered Status = "registered"
	StatusInProgress Status = "in_progress"
	StatusPaused     Status = "paused"
	StatusCompleted  Status = "completed"
	StatusCancelled  Status = "cancelled"
)

type MaterialSnapshot struct {
	ItemID          string            `json:"item_id"`
	QuantityPerUnit quantity.Quantity `json:"quantity_per_unit"`
}

// AdditionalMaterial is an execution-specific total, not a per-unit quantity.
type AdditionalMaterial struct {
	ItemID   string            `json:"item_id"`
	Quantity quantity.Quantity `json:"quantity"`
}

type AddMaterialInput struct {
	RequestID string            `json:"request_id"`
	ItemID    string            `json:"item_id"`
	Quantity  quantity.Quantity `json:"quantity"`
}

type AddToolInput struct {
	RequestID string `json:"request_id"`
	ItemID    string `json:"item_id"`
}

type ProduceInput struct {
	RequestID string `json:"request_id"`
	Quantity  int    `json:"quantity"`
}

func (in ProduceInput) Normalize() (ProduceInput, error) {
	in.RequestID = strings.ToLower(strings.TrimSpace(in.RequestID))
	fields := map[string]string{}
	if !validUUID(in.RequestID) {
		fields["request_id"] = "must be a valid UUID"
	}
	if in.Quantity < 1 {
		fields["quantity"] = "must be a positive whole number"
	}
	if len(fields) > 0 {
		return ProduceInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

type FinishedUnit struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	ExecutionID int64     `json:"execution_id"`
	ProducedAt  time.Time `json:"produced_at"`
}

func (in AddToolInput) Normalize() (AddToolInput, error) {
	in.RequestID = strings.ToLower(strings.TrimSpace(in.RequestID))
	in.ItemID = strings.ToLower(strings.TrimSpace(in.ItemID))
	fields := map[string]string{}
	if !validUUID(in.RequestID) {
		fields["request_id"] = "must be a valid UUID"
	}
	if !validUUID(in.ItemID) {
		fields["item_id"] = "must be a valid inventory item id"
	}
	if len(fields) > 0 {
		return AddToolInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

func (in AddMaterialInput) Normalize() (AddMaterialInput, error) {
	in.RequestID = strings.TrimSpace(in.RequestID)
	in.ItemID = strings.TrimSpace(in.ItemID)
	fields := map[string]string{}
	if !validUUID(in.RequestID) {
		fields["request_id"] = "must be a valid UUID"
	}
	if !validUUID(in.ItemID) {
		fields["item_id"] = "must be a valid inventory item id"
	}
	in.RequestID = strings.ToLower(in.RequestID)
	in.ItemID = strings.ToLower(in.ItemID)
	if in.Quantity <= 0 {
		fields["quantity"] = "must be greater than zero"
	} else if _, err := RequiredMaterialQuantity(in.Quantity, 1); err != nil {
		fields["quantity"] = "exceeds the supported stock limit"
	}
	if len(fields) > 0 {
		return AddMaterialInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

type ProjectSnapshot struct {
	ProjectID       int64              `json:"project_id"`
	Name            string             `json:"project_name"`
	Description     string             `json:"description"`
	Characteristics []string           `json:"characteristics"`
	Consumables     []MaterialSnapshot `json:"consumables"`
	Tools           []string           `json:"tools"`
	EstimatedDays   int                `json:"template_estimated_days"`
}

type CreateInput struct {
	ProjectID           int64                `json:"project_id"`
	Units               int                  `json:"units"`
	EstimatedDays       int                  `json:"estimated_days,omitempty"`
	CustomerIDs         []int64              `json:"customer_ids"`
	PrincipalUserID     string               `json:"principal_user_id"`
	WorkerIDs           []string             `json:"worker_ids"`
	AdditionalMaterials []AdditionalMaterial `json:"additional_materials,omitempty"`
	AdditionalTools     []string             `json:"additional_tools,omitempty"`
}

type Filter struct {
	Status        *Status
	Q             string
	Limit, Offset int
}

func (f Filter) Normalize() (Filter, error) {
	if f.Limit == 0 {
		f.Limit = 25
	}
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 {
		return f, &ValidationError{Fields: map[string]string{"limit": "must be 1 to 100 and offset nonnegative"}}
	}
	if f.Status != nil && *f.Status != StatusRegistered && *f.Status != StatusInProgress && *f.Status != StatusPaused && *f.Status != StatusCompleted && *f.Status != StatusCancelled {
		return f, &ValidationError{Fields: map[string]string{"status": "must be a supported execution status"}}
	}
	f.Q = strings.TrimSpace(f.Q)
	return f, nil
}

func (in CreateInput) Normalize() (CreateInput, error) {
	in.PrincipalUserID = strings.TrimSpace(in.PrincipalUserID)
	fields := map[string]string{}
	if in.ProjectID < 1 {
		fields["project_id"] = "must reference an existing project"
	}
	if in.Units < 1 {
		fields["units"] = "must be a positive whole number"
	}
	if in.EstimatedDays < 0 || in.EstimatedDays > 36500 {
		fields["estimated_days"] = "must be between 1 and 36500 days when provided"
	}
	if len(in.CustomerIDs) == 0 {
		fields["customer_ids"] = "at least one customer is required"
	}
	seenCustomers := map[int64]bool{}
	in.CustomerIDs = append([]int64{}, in.CustomerIDs...)
	for i, id := range in.CustomerIDs {
		if id < 1 || seenCustomers[id] {
			fields[fmt.Sprintf("customer_ids[%d]", i)] = "must be a distinct existing customer"
		}
		seenCustomers[id] = true
	}
	if in.PrincipalUserID == "" {
		fields["principal_user_id"] = "a principal responsible user is required"
	}
	in.WorkerIDs = append([]string{}, in.WorkerIDs...)
	seenUsers := map[string]bool{}
	if in.PrincipalUserID != "" {
		seenUsers[in.PrincipalUserID] = true
	}
	for i, id := range in.WorkerIDs {
		id = strings.TrimSpace(id)
		in.WorkerIDs[i] = id
		if id == "" || seenUsers[id] {
			fields[fmt.Sprintf("worker_ids[%d]", i)] = "must be a distinct assigned user"
		}
		seenUsers[id] = true
	}
	in.AdditionalMaterials = append([]AdditionalMaterial{}, in.AdditionalMaterials...)
	materialIndexes := map[string]int{}
	materials := make([]AdditionalMaterial, 0, len(in.AdditionalMaterials))
	for i, material := range in.AdditionalMaterials {
		material.ItemID = strings.ToLower(strings.TrimSpace(material.ItemID))
		field := fmt.Sprintf("additional_materials[%d]", i)
		if !validUUID(material.ItemID) {
			fields[field+".item_id"] = "must be a valid inventory item id"
		}
		if material.Quantity <= 0 {
			fields[field+".quantity"] = "must be greater than zero"
		} else if _, err := RequiredMaterialQuantity(material.Quantity, 1); err != nil {
			fields[field+".quantity"] = "exceeds the supported stock limit"
		}
		if index, exists := materialIndexes[material.ItemID]; exists {
			total := materials[index].Quantity + material.Quantity
			if _, err := RequiredMaterialQuantity(total, 1); err != nil {
				fields[field+".quantity"] = "combined quantity exceeds the supported stock limit"
			} else {
				materials[index].Quantity = total
			}
			continue
		}
		materialIndexes[material.ItemID] = len(materials)
		materials = append(materials, material)
	}
	in.AdditionalMaterials = materials
	in.AdditionalTools = append([]string{}, in.AdditionalTools...)
	seenTools := make(map[string]struct{}, len(in.AdditionalTools))
	tools := make([]string, 0, len(in.AdditionalTools))
	for i, id := range in.AdditionalTools {
		id = strings.ToLower(strings.TrimSpace(id))
		field := fmt.Sprintf("additional_tools[%d]", i)
		if !validUUID(id) {
			fields[field] = "must be a valid inventory item id"
		}
		if _, exists := seenTools[id]; exists {
			continue
		}
		seenTools[id] = struct{}{}
		tools = append(tools, id)
	}
	in.AdditionalTools = tools
	if len(fields) > 0 {
		return CreateInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if char != '-' {
				return false
			}
			continue
		}
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

type Execution struct {
	ID int64 `json:"id"`
	ProjectSnapshot
	ShortageRequestID   int64                `json:"shortage_request_id,omitempty"`
	Units               int                  `json:"units"`
	ProducedUnits       int                  `json:"produced_units"`
	EstimatedDays       int                  `json:"estimated_days"`
	Materials           []MaterialAllocation `json:"materials"`
	AdditionalMaterials []AdditionalMaterial `json:"-"`
	AdditionalTools     []string             `json:"-"`
	CustomerIDs         []int64              `json:"customer_ids"`
	PrincipalUserID     string               `json:"principal_user_id"`
	WorkerIDs           []string             `json:"worker_ids"`
	Status              Status               `json:"status"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

// MaterialAllocation is the current fulfillment state for a snapshotted
// consumable requirement. Reservations are informational quantities held
// globally for this execution; tools remain outside the stock reservation flow.
type MaterialAllocation struct {
	ItemID                   string            `json:"item_id"`
	RequiredQuantity         quantity.Quantity `json:"required_quantity"`
	ReservedQuantity         quantity.Quantity `json:"reserved_quantity"`
	DeliveredQuantity        quantity.Quantity `json:"delivered_quantity"`
	ReturnedQuantity         quantity.Quantity `json:"returned_quantity"`
	DeficitQuantity          quantity.Quantity `json:"deficit_quantity"`
	PurchasedPendingQuantity quantity.Quantity `json:"purchased_pending_quantity"`
	UnpurchasedQuantity      quantity.Quantity `json:"unpurchased_quantity"`
}

type AssignableUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewExecution(input CreateInput, snapshot ProjectSnapshot) (Execution, error) {
	in, err := input.Normalize()
	if err != nil {
		return Execution{}, err
	}
	if snapshot.ProjectID != in.ProjectID || snapshot.ProjectID < 1 || snapshot.Name == "" || snapshot.EstimatedDays < 1 {
		return Execution{}, &ValidationError{Fields: map[string]string{"project_id": "must reference an existing project"}}
	}
	days := in.EstimatedDays
	if days == 0 {
		days = snapshot.EstimatedDays
	}
	if days < 1 {
		return Execution{}, &ValidationError{Fields: map[string]string{"estimated_days": "must be between 1 and 36500 days"}}
	}
	for i, material := range snapshot.Consumables {
		if strings.TrimSpace(material.ItemID) == "" {
			return Execution{}, &ValidationError{Fields: map[string]string{fmt.Sprintf("consumables[%d].item_id", i): "required"}}
		}
		if _, err := RequiredMaterialQuantity(material.QuantityPerUnit, in.Units); err != nil {
			return Execution{}, &ValidationError{Fields: map[string]string{fmt.Sprintf("consumables[%d].quantity_per_unit", i): "scaled quantity exceeds the supported stock limit"}}
		}
	}
	snapshot.Characteristics = append([]string{}, snapshot.Characteristics...)
	snapshot.Consumables = append([]MaterialSnapshot{}, snapshot.Consumables...)
	snapshot.Tools = append([]string{}, snapshot.Tools...)
	return Execution{
		ProjectSnapshot: snapshot,
		Units:           in.Units, EstimatedDays: days,
		AdditionalMaterials: in.AdditionalMaterials,
		AdditionalTools:     in.AdditionalTools,
		CustomerIDs:         in.CustomerIDs, PrincipalUserID: in.PrincipalUserID,
		WorkerIDs: in.WorkerIDs, Status: StatusRegistered,
	}, nil
}

func (e *Execution) Start() error {
	if e.Status != StatusRegistered {
		return ErrInvalidState
	}
	e.Status = StatusInProgress
	return nil
}

func (e *Execution) Pause() error {
	if e.Status != StatusInProgress {
		return ErrInvalidState
	}
	e.Status = StatusPaused
	return nil
}

func (e *Execution) Resume() error {
	if e.Status != StatusPaused {
		return ErrInvalidState
	}
	e.Status = StatusInProgress
	return nil
}

func (e *Execution) Complete() error {
	if e.Status != StatusInProgress && e.Status != StatusPaused {
		return ErrInvalidState
	}
	if e.ProducedUnits != e.Units {
		return ErrProductionIncomplete
	}
	e.Status = StatusCompleted
	return nil
}

func (e *Execution) Cancel() error {
	if e.Status != StatusRegistered && e.Status != StatusInProgress && e.Status != StatusPaused {
		return ErrInvalidState
	}
	e.Status = StatusCancelled
	return nil
}

func (e Execution) Clone() Execution {
	e.Characteristics = append([]string{}, e.Characteristics...)
	e.Consumables = append([]MaterialSnapshot{}, e.Consumables...)
	e.Tools = append([]string{}, e.Tools...)
	e.CustomerIDs = append([]int64{}, e.CustomerIDs...)
	e.WorkerIDs = append([]string{}, e.WorkerIDs...)
	e.Materials = append([]MaterialAllocation{}, e.Materials...)
	e.AdditionalMaterials = append([]AdditionalMaterial{}, e.AdditionalMaterials...)
	e.AdditionalTools = append([]string{}, e.AdditionalTools...)
	return e
}
