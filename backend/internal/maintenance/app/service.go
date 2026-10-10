package app

import (
	"context"
	"strings"
	"time"

	"github.com/edgar/inventario/backend/internal/maintenance/domain"
)

// Service implements the maintenance use cases.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService wires the repository. now may be nil (defaults to time.Now).
func NewService(r Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: r, now: now}
}

// Create reports a failure or registers a preventive service on an asset,
// depending on req.Kind, on behalf of userID.
func (s *Service) Create(ctx context.Context, itemID, userID string, req domain.CreateRequest) (domain.Record, error) {
	report, preventive, err := req.Split()
	if err != nil {
		return domain.Record{}, err
	}
	now := s.now()
	if req.Kind == domain.KindCorrective {
		return s.repo.Report(ctx, itemID, userID, func(cur domain.ItemState) (domain.ReportOutcome, error) {
			return domain.DecideReport(cur, report, now)
		})
	}
	return s.repo.RegisterPreventive(ctx, itemID, userID, func(cur domain.ItemState) (domain.PreventiveOutcome, error) {
		return domain.DecidePreventive(cur, preventive, now)
	})
}

// Update edits an open record.
func (s *Service) Update(ctx context.Context, recordID int64, req domain.UpdateRequest) (domain.Record, error) {
	return s.repo.Update(ctx, recordID, func(cur domain.ItemState, rec domain.Record) (domain.UpdateOutcome, error) {
		return domain.DecideUpdate(cur, rec, req)
	})
}

// Close closes an open corrective record on behalf of userID.
func (s *Service) Close(ctx context.Context, recordID int64, userID string, req domain.CloseRequest) (domain.Record, error) {
	now := s.now()
	return s.repo.Close(ctx, recordID, userID, func(cur domain.ItemState, rec domain.Record) (domain.CloseOutcome, error) {
		return domain.DecideClose(cur, rec, req, now)
	})
}

// History returns one page of an item's records, newest first.
func (s *Service) History(ctx context.Context, itemID string, limit, offset int) ([]domain.Record, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	rs, total, err := s.repo.History(ctx, itemID, p)
	return rs, total, p, err
}

// List returns one page of records, open first, then newest first.
func (s *Service) List(ctx context.Context, f domain.ListFilter, limit, offset int) ([]domain.Record, int, domain.Page, error) {
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, err
	}
	fe := map[string]string{}
	if f.Status != "" && !f.Status.Valid() {
		fe["status"] = "must be open or closed"
	}
	if f.Kind != "" && !f.Kind.Valid() {
		fe["kind"] = "must be preventive or corrective"
	}
	if f.ItemID != "" && !validUUID(f.ItemID) {
		fe["item_id"] = "must be a UUID"
	}
	if len(fe) > 0 {
		return nil, 0, p, &domain.ValidationError{Fields: fe}
	}
	f.Destination = strings.TrimSpace(f.Destination)
	f.Q = strings.TrimSpace(f.Q)
	rs, total, err := s.repo.List(ctx, f, p)
	return rs, total, p, err
}

// Due returns one page of assets whose maintenance is overdue or due within
// f.WithinDays, most overdue first, plus the clock used to flag overdue rows.
func (s *Service) Due(ctx context.Context, f domain.DueFilter, limit, offset int) ([]domain.DueItem, int, domain.Page, time.Time, error) {
	now := s.now()
	p, err := domain.NormalizePage(limit, offset)
	if err != nil {
		return nil, 0, p, now, err
	}
	if f.WithinDays < 0 || f.WithinDays > domain.MaxDueWithinDays {
		return nil, 0, p, now, domain.FieldError("within_days", "must be between 0 and 365")
	}
	f.Q = strings.TrimSpace(f.Q)
	f.Now = now
	items, total, err := s.repo.Due(ctx, f, p)
	return items, total, p, now, err
}

func validUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}
