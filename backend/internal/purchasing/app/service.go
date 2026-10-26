package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/edgar/inventario/backend/internal/purchasing/domain"
)

// Service implements the purchase order use cases.
type Service struct {
	repo      Repository
	items     ItemCatalog
	stock     StockReceiver
	suppliers Suppliers
	now       func() time.Time
}

// NewService wires the repository and the ports. now may be nil.
func NewService(r Repository, items ItemCatalog, stock StockReceiver, sups Suppliers, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: r, items: items, stock: stock, suppliers: sups, now: now}
}

// Create validates and stores a draft order.
func (s *Service) Create(ctx context.Context, userID string, in domain.Input) (domain.Order, error) {
	n, err := in.Normalize()
	if err != nil {
		return domain.Order{}, err
	}
	if err := s.checkSupplier(ctx, n.SupplierID, false); err != nil {
		return domain.Order{}, err
	}
	if n.Lines != nil {
		if n.Lines, err = s.checkLines(ctx, n.Lines); err != nil {
			return domain.Order{}, err
		}
	}
	o := domain.NewDraft(n, userID, s.now())
	if err := s.repo.Create(ctx, &o); err != nil {
		return domain.Order{}, err
	}
	return s.repo.Get(ctx, o.ID)
}

// Get returns one order.
func (s *Service) Get(ctx context.Context, id int64) (domain.Order, error) {
	return s.repo.Get(ctx, id)
}

// List returns a filtered page of orders, the total and the normalized filter.
func (s *Service) List(ctx context.Context, f domain.Filter) ([]domain.Order, int, domain.Filter, error) {
	nf, err := f.Normalize()
	if err != nil {
		return nil, 0, domain.Filter{}, err
	}
	out, total, err := s.repo.List(ctx, nf)
	if err != nil {
		return nil, 0, domain.Filter{}, err
	}
	return out, total, nf, nil
}

// Opt is an optional PATCH value: Set tells "present", Val nil means null.
type Opt[T any] struct {
	Set bool
	Val *T
}

// Patch is a partial draft update. Lines, when non-nil, replace the whole set.
type Patch struct {
	SupplierID   *int64
	Currency     *domain.Currency
	ExchangeRate Opt[float64]
	ExpectedAt   Opt[time.Time]
	Notes        *string
	Lines        []domain.LineInput
}

// Update edits a draft order: it applies the patch, revalidates the whole
// order and stores it.
func (s *Service) Update(ctx context.Context, id int64, p Patch) (domain.Order, error) {
	return s.repo.Mutate(ctx, id, func(ctx context.Context, o *domain.Order) error {
		if o.Status != domain.StatusDraft {
			return &domain.InvalidStateError{Status: o.Status, Action: "edit"}
		}
		in := domain.Input{
			SupplierID: o.SupplierID, Currency: o.Currency, ExchangeRate: o.ExchangeRate,
			ExpectedAt: o.ExpectedAt, Notes: o.Notes, Lines: p.Lines,
		}
		if p.SupplierID != nil {
			in.SupplierID = *p.SupplierID
		}
		if p.Currency != nil {
			in.Currency = *p.Currency
		}
		if p.ExchangeRate.Set {
			in.ExchangeRate = p.ExchangeRate.Val
		}
		if p.ExpectedAt.Set {
			in.ExpectedAt = p.ExpectedAt.Val
		}
		if p.Notes != nil {
			in.Notes = *p.Notes
		}
		n, err := in.Normalize()
		if err != nil {
			return err
		}
		if p.SupplierID != nil && *p.SupplierID != o.SupplierID {
			if err := s.checkSupplier(ctx, n.SupplierID, false); err != nil {
				return err
			}
		}
		if n.Lines != nil {
			if n.Lines, err = s.checkLines(ctx, n.Lines); err != nil {
				return err
			}
		}
		return o.Edit(n, s.now())
	})
}

// Place moves a draft to ordered; the supplier must still be active.
func (s *Service) Place(ctx context.Context, id int64) (domain.Order, error) {
	return s.repo.Mutate(ctx, id, func(ctx context.Context, o *domain.Order) error {
		if o.Status != domain.StatusDraft {
			return &domain.InvalidStateError{Status: o.Status, Action: "place"}
		}
		if len(o.Lines) == 0 {
			return o.Place(s.now()) // reports the missing lines
		}
		if err := s.checkSupplier(ctx, o.SupplierID, true); err != nil {
			return err
		}
		return o.Place(s.now())
	})
}

// Cancel cancels a draft or an ordered order with nothing received.
func (s *Service) Cancel(ctx context.Context, id int64) (domain.Order, error) {
	return s.repo.Mutate(ctx, id, func(_ context.Context, o *domain.Order) error {
		return o.Cancel(s.now())
	})
}

// ReceiveResult is the outcome of a receive request.
type ReceiveResult struct {
	Order   domain.Order
	Receipt domain.Receipt
	// Replayed is true when the client key matched an earlier receipt and
	// nothing was done.
	Replayed bool
}

// Receive registers a delivery. In one transaction it creates the catalog items
// of new_item lines, records stock for consumables, stores the receipt and
// advances the order; any failure rolls all of it back.
func (s *Service) Receive(ctx context.Context, id int64, userID string, req domain.ReceiveRequest) (ReceiveResult, error) {
	var replay *domain.Receipt
	o, err := s.repo.Mutate(ctx, id, func(ctx context.Context, o *domain.Order) error {
		if rc, ok := o.ReceiptByKey(strings.TrimSpace(req.ClientKey)); ok {
			replay = &rc
			return domain.ErrNoChange
		}
		plan, idx, err := o.PlanReceipt(req)
		if err != nil {
			return err
		}
		ok, err := s.items.LocationExists(ctx, plan.LocationID)
		if err != nil {
			return err
		}
		if !ok {
			return domain.FieldError("location_id", "location does not exist")
		}
		rc := domain.Receipt{
			LocationID: plan.LocationID, ReceivedBy: userID, ReceivedAt: s.now(),
			Note: plan.Note, ClientKey: plan.ClientKey,
		}
		note := fmt.Sprintf("Purchase order #%d", o.ID)
		if plan.Note != "" {
			note += ": " + plan.Note
		}
		for i, rl := range plan.Lines {
			line := &o.Lines[idx[i]]
			items, err := s.bringIn(ctx, line, rl.Quantity, plan.LocationID, userID, note)
			if err != nil {
				return fmt.Errorf("line %d: %w", line.ID, err)
			}
			rc.Lines = append(rc.Lines, domain.ReceiptLine{LineID: line.ID, Quantity: rl.Quantity, Items: items})
		}
		o.ApplyReceipt(rc, s.now())
		return nil
	})
	if err != nil {
		return ReceiveResult{}, err
	}
	if replay != nil {
		return ReceiveResult{Order: o, Receipt: *replay, Replayed: true}, nil
	}
	return ReceiveResult{Order: o, Receipt: o.Receipts[len(o.Receipts)-1]}, nil
}

// bringIn creates the items and the stock movement for qty units of line.
func (s *Service) bringIn(ctx context.Context, line *domain.Line, qty int, loc int64, userID, note string) ([]domain.ReceiptItem, error) {
	if line.IsAsset() {
		out := make([]domain.ReceiptItem, 0, qty)
		for n := 0; n < qty; n++ {
			it, err := s.items.CreateItem(ctx, *line.NewItem, loc)
			if err != nil {
				return nil, fmt.Errorf("create asset %d of %d: %w", n+1, qty, err)
			}
			if line.ItemID == "" {
				line.ItemID = it.ID
			}
			out = append(out, domain.ReceiptItem{ItemID: it.ID, Code: it.Code, Quantity: 1})
		}
		return out, nil
	}
	code := ""
	if line.ItemID == "" { // a new consumable is created once, on first receipt
		it, err := s.items.CreateItem(ctx, *line.NewItem, loc)
		if err != nil {
			return nil, fmt.Errorf("create consumable: %w", err)
		}
		line.ItemID, code = it.ID, it.Code
	}
	if err := s.stock.ReceiveIn(ctx, line.ItemID, userID, loc, qty, note); err != nil {
		return nil, fmt.Errorf("record stock: %w", err)
	}
	return []domain.ReceiptItem{{ItemID: line.ItemID, Code: code, Quantity: qty}}, nil
}

// checkSupplier verifies the supplier exists and, when requireActive is set or
// always for inactive ones at create, is active. At create and at place time
// the supplier must be active; a supplier removed since is reported as
// inactive when placing.
func (s *Service) checkSupplier(ctx context.Context, id int64, placing bool) error {
	_, active, err := s.suppliers.Get(ctx, id)
	if errors.Is(err, ErrSupplierNotFound) {
		if placing {
			return supplierInactive()
		}
		return domain.FieldError("supplier_id", "supplier does not exist")
	}
	if err != nil {
		return err
	}
	if !active {
		return supplierInactive()
	}
	return nil
}

func supplierInactive() error {
	return &domain.ValidationError{
		Code: domain.CodeSupplierInactive, Message: "the supplier is not active",
		Fields: map[string]string{"supplier_id": domain.CodeSupplierInactive},
	}
}

// checkLines validates every line against the catalog and fills the
// description snapshot of existing-item lines. All problems are reported
// together.
func (s *Service) checkLines(ctx context.Context, lines []domain.LineInput) ([]domain.LineInput, error) {
	out := make([]domain.LineInput, len(lines))
	copy(out, lines)
	fe := map[string]string{}
	for i, l := range out {
		p := fmt.Sprintf("lines[%d].", i)
		if l.NewItem != nil {
			if err := s.items.ValidateNewItem(ctx, *l.NewItem); err != nil {
				var ve *domain.ValidationError
				if !errors.As(err, &ve) {
					return nil, err
				}
				for k, v := range ve.Fields {
					fe[p+"new_item."+k] = v
				}
			}
			continue
		}
		info, err := s.items.ItemInfo(ctx, l.ItemID)
		switch {
		case errors.Is(err, ErrItemNotFound):
			fe[p+"item_id"] = "item does not exist"
		case err != nil:
			return nil, err
		case info.Kind != domain.KindConsumable:
			fe[p+"item_id"] = "existing_item_must_be_consumable"
		default:
			out[i].Description = info.Description
		}
	}
	if len(fe) > 0 {
		return nil, &domain.ValidationError{Fields: fe}
	}
	return out, nil
}
