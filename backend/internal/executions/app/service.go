// Package app coordinates the creation and lifecycle of manufacturing executions.
package app

import (
	"context"
	"errors"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
)

var ErrProjectNotFound = errors.New("project template not found")

type TemplateReader interface {
	Get(context.Context, int64) (projectdomain.Project, error)
}

type Repository interface {
	Create(context.Context, domain.Execution) (domain.Execution, error)
	Get(context.Context, int64) (domain.Execution, error)
	List(context.Context, domain.Filter) ([]domain.Execution, int, error)
	Transition(context.Context, int64, domain.Status, domain.Status) (domain.Execution, error)
	ListAssignableUsers(context.Context) ([]domain.AssignableUser, error)
}

type Service struct {
	repo      Repository
	templates TemplateReader
}

func NewService(repo Repository, templates TemplateReader) *Service {
	return &Service{repo: repo, templates: templates}
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
