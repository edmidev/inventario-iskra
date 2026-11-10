// Package app coordinates the creation and lifecycle of manufacturing executions.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
)

var ErrProjectNotFound = errors.New("project template not found")

type TemplateReader interface {
	Get(context.Context, int64) (projectdomain.Project, error)
}

type Repository interface {
	Create(context.Context, domain.Execution) (domain.Execution, error)
	LinkPurchaseOrderLine(context.Context, int64, int64, quantity.Quantity) (domain.Execution, error)
	AddMaterialRequirement(context.Context, int64, string, string, quantity.Quantity, string) (domain.Execution, error)
	AddToolRequirement(context.Context, int64, string, string, string, string) (domain.Execution, error)
	Get(context.Context, int64) (domain.Execution, error)
	List(context.Context, domain.Filter) ([]domain.Execution, int, error)
	Transition(context.Context, int64, domain.Status, domain.Status) (domain.Execution, error)
	ListAssignableUsers(context.Context) ([]domain.AssignableUser, error)
}

func (s *Service) LinkPurchaseOrderLine(ctx context.Context, executionID, lineID int64, amount quantity.Quantity) (domain.Execution, error) {
	return s.repo.LinkPurchaseOrderLine(ctx, executionID, lineID, amount)
}

type Service struct {
	repo      Repository
	templates TemplateReader
	stock     interface {
		RecordExecutionMaterial(context.Context, string, string, int64, string, stockdomain.Request) (stockapp.Result, error)
	}
}

func NewService(repo Repository, templates TemplateReader, stock ...interface {
	RecordExecutionMaterial(context.Context, string, string, int64, string, stockdomain.Request) (stockapp.Result, error)
}) *Service {
	s := &Service{repo: repo, templates: templates}
	if len(stock) > 0 {
		s.stock = stock[0]
	}
	return s
}

func (s *Service) Create(ctx context.Context, input domain.CreateInput) (domain.Execution, error) {
	normalized, err := input.Normalize()
	if err != nil {
		return domain.Execution{}, err
	}
	template, err := s.templates.Get(ctx, normalized.ProjectID)
	if errors.Is(err, projectdomain.ErrNotFound) {
		return domain.Execution{}, ErrProjectNotFound
	}
	if err != nil {
		return domain.Execution{}, err
	}
	snapshot := domain.ProjectSnapshot{
		ProjectID:       template.ID,
		Name:            template.Name,
		Description:     template.Description,
		Characteristics: append([]string{}, template.Characteristics...),
		Tools:           append([]string{}, template.Tools...),
		EstimatedDays:   template.EstimatedDays,
	}
	for _, material := range template.Consumables {
		snapshot.Consumables = append(snapshot.Consumables, domain.MaterialSnapshot{ItemID: material.ItemID, QuantityPerUnit: material.Quantity})
	}
	execution, err := domain.NewExecution(normalized, snapshot)
	if err != nil {
		return domain.Execution{}, err
	}
	return s.repo.Create(ctx, execution)
}

func (s *Service) AddMaterialRequirement(ctx context.Context, executionID int64, userID string, input domain.AddMaterialInput) (domain.Execution, error) {
	if executionID < 1 || strings.TrimSpace(userID) == "" {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"execution_id": "execution and authenticated user are required"}}
	}
	normalized, err := input.Normalize()
	if err != nil {
		return domain.Execution{}, err
	}
	serialized, err := json.Marshal(struct {
		ExecutionID int64
		UserID      string
		ItemID      string
		Quantity    quantity.Quantity
	}{executionID, userID, normalized.ItemID, normalized.Quantity})
	if err != nil {
		return domain.Execution{}, err
	}
	fingerprint := sha256.Sum256(serialized)
	return s.repo.AddMaterialRequirement(ctx, executionID, normalized.RequestID, normalized.ItemID, normalized.Quantity, hex.EncodeToString(fingerprint[:]))
}

func (s *Service) AddToolRequirement(ctx context.Context, executionID int64, userID string, input domain.AddToolInput) (domain.Execution, error) {
	if executionID < 1 || strings.TrimSpace(userID) == "" {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"execution_id": "execution and authenticated user are required"}}
	}
	normalized, err := input.Normalize()
	if err != nil {
		return domain.Execution{}, err
	}
	serialized, err := json.Marshal(struct {
		ExecutionID int64
		UserID      string
		ItemID      string
	}{executionID, userID, normalized.ItemID})
	if err != nil {
		return domain.Execution{}, err
	}
	fingerprint := sha256.Sum256(serialized)
	return s.repo.AddToolRequirement(ctx, executionID, normalized.RequestID, userID, normalized.ItemID, hex.EncodeToString(fingerprint[:]))
}

func (s *Service) Get(ctx context.Context, id int64) (domain.Execution, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter domain.Filter) ([]domain.Execution, int, domain.Filter, error) {
	normalized, err := filter.Normalize()
	if err != nil {
		return nil, 0, normalized, err
	}
	rows, count, err := s.repo.List(ctx, normalized)
	return rows, count, normalized, err
}

func (s *Service) Start(ctx context.Context, id int64) (domain.Execution, error) {
	return s.repo.Transition(ctx, id, domain.StatusRegistered, domain.StatusInProgress)
}

func (s *Service) Pause(ctx context.Context, id int64) (domain.Execution, error) {
	return s.repo.Transition(ctx, id, domain.StatusInProgress, domain.StatusPaused)
}

func (s *Service) Resume(ctx context.Context, id int64) (domain.Execution, error) {
	return s.repo.Transition(ctx, id, domain.StatusPaused, domain.StatusInProgress)
}

func (s *Service) ListAssignableUsers(ctx context.Context) ([]domain.AssignableUser, error) {
	return s.repo.ListAssignableUsers(ctx)
}

func (s *Service) DeliverMaterial(ctx context.Context, executionID int64, itemID, userID, requestID string, qty quantity.Quantity, locationID int64, note string) (domain.Execution, error) {
	return s.moveMaterial(ctx, executionID, itemID, userID, requestID, qty, locationID, note, stockdomain.TypeOut)
}

func (s *Service) ReturnMaterial(ctx context.Context, executionID int64, itemID, userID, requestID string, qty quantity.Quantity, locationID int64, note string) (domain.Execution, error) {
	return s.moveMaterial(ctx, executionID, itemID, userID, requestID, qty, locationID, note, stockdomain.TypeIn)
}

func (s *Service) moveMaterial(ctx context.Context, executionID int64, itemID, userID, requestID string, qty quantity.Quantity, locationID int64, note string, movement stockdomain.MovementType) (domain.Execution, error) {
	if s.stock == nil {
		return domain.Execution{}, errors.New("execution material movement is not configured")
	}
	if executionID < 1 || strings.TrimSpace(itemID) == "" || strings.TrimSpace(userID) == "" || strings.TrimSpace(requestID) == "" || qty <= 0 || locationID < 1 {
		return domain.Execution{}, &domain.ValidationError{Fields: map[string]string{"body": "execution, item, request id, positive quantity and physical location are required"}}
	}
	_, err := s.stock.RecordExecutionMaterial(ctx, itemID, userID, executionID, requestID, stockdomain.Request{
		Type: movement, Qty: qty, LocationID: &locationID, Note: note,
	})
	if err != nil {
		return domain.Execution{}, err
	}
	return s.repo.Get(ctx, executionID)
}
