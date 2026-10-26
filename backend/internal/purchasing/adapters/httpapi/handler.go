// Package httpapi exposes the purchase order use cases over HTTP under /api/v1.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/purchasing/app"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
	"github.com/go-chi/chi/v5"
)

const (
	maxBodyBytes = 1 << 20
	dateLayout   = "2006-01-02"

	codeInvalidState = "invalid_state"
)

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read  string // list and read
	Write string // create, edit, place, cancel, receive
}

// Handler serves purchase orders.
type Handler struct {
	svc          *app.Service
	authenticate func(http.Handler) http.Handler
	perms        Permissions
}

// New builds a Handler. authenticate is the bearer-token middleware.
func New(svc *app.Service, authenticate func(http.Handler) http.Handler, perms Permissions) *Handler {
	return &Handler{svc: svc, authenticate: authenticate, perms: perms}
}

// Register mounts the routes on r (expected to be the /api/v1 router).
func (h *Handler) Register(r chi.Router) {
	read := httpx.RequirePermission(h.perms.Read)
	write := httpx.RequirePermission(h.perms.Write)

	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.Route("/purchase-orders", func(r chi.Router) {
			r.With(read).Get("/", h.list)
			r.With(read).Get("/{id}", h.get)
			r.With(write).Post("/", h.create)
			r.With(write).Patch("/{id}", h.update)
			r.With(write).Post("/{id}/order", h.place)
			r.With(write).Post("/{id}/cancel", h.cancel)
			r.With(write).Post("/{id}/receive", h.receive)
		})
	})
}

// ---- DTOs ----

type newItemDTO struct {
	Description string         `json:"description"`
	CategoryID  int64          `json:"category_id"`
	Kind        string         `json:"kind"`
	Unit        string         `json:"unit,omitempty"`
	StockMin    int            `json:"stock_min,omitempty"`
	Attributes  map[string]any `json:"attributes,omitempty"`
	Notes       string         `json:"notes,omitempty"`
}

type lineDTO struct {
	ID            int64       `json:"id"`
	ItemID        *string     `json:"item_id"`
	NewItem       *newItemDTO `json:"new_item"`
	Description   string      `json:"description"`
	Quantity      int         `json:"quantity"`
	ReceivedQty   int         `json:"received_qty"`
	UnitCostMinor int64       `json:"unit_cost_minor"`
}

type receiptItemDTO struct {
	ItemID   string `json:"item_id"`
	Code     string `json:"code,omitempty"`
	Quantity int    `json:"quantity"`
}

type receiptLineDTO struct {
	LineID   int64            `json:"line_id"`
	Quantity int              `json:"quantity"`
	Items    []receiptItemDTO `json:"items"`
}

type receiptDTO struct {
	ID         int64            `json:"id"`
	OrderID    int64            `json:"order_id"`
	LocationID int64            `json:"location_id"`
	ReceivedBy *string          `json:"received_by"`
	ReceivedAt time.Time        `json:"received_at"`
	Note       string           `json:"note"`
	ClientKey  string           `json:"client_key,omitempty"`
	Lines      []receiptLineDTO `json:"lines"`
}

type orderDTO struct {
	ID           int64        `json:"id"`
	SupplierID   int64        `json:"supplier_id"`
	SupplierName string       `json:"supplier_name"`
	Status       string       `json:"status"`
	Currency     string       `json:"currency"`
	ExchangeRate *float64     `json:"exchange_rate"`
	ExpectedAt   *string      `json:"expected_at"`
	Notes        string       `json:"notes"`
	TotalMinor   int64        `json:"total_minor"`
	Lines        []lineDTO    `json:"lines"`
	Receipts     []receiptDTO `json:"receipts,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	OrderedAt    *time.Time   `json:"ordered_at"`
	CancelledAt  *time.Time   `json:"cancelled_at"`
}

func toLineDTO(l domain.Line) lineDTO {
	d := lineDTO{
		ID: l.ID, Description: l.Description, Quantity: l.Quantity,
		ReceivedQty: l.ReceivedQty, UnitCostMinor: l.UnitCostMinor,
	}
	if l.ItemID != "" {
		id := l.ItemID
		d.ItemID = &id
	}
	if n := l.NewItem; n != nil {
		d.NewItem = &newItemDTO{
			Description: n.Description, CategoryID: n.CategoryID, Kind: n.Kind, Unit: n.Unit,
			StockMin: n.StockMin, Attributes: n.Attributes, Notes: n.Notes,
		}
	}
	return d
}

func toReceiptDTO(orderID int64, r domain.Receipt) receiptDTO {
	d := receiptDTO{
		ID: r.ID, OrderID: orderID, LocationID: r.LocationID, ReceivedAt: r.ReceivedAt,
		Note: r.Note, ClientKey: r.ClientKey, Lines: make([]receiptLineDTO, len(r.Lines)),
	}
	if r.ReceivedBy != "" {
		by := r.ReceivedBy
		d.ReceivedBy = &by
	}
	for i, l := range r.Lines {
		rl := receiptLineDTO{LineID: l.LineID, Quantity: l.Quantity, Items: make([]receiptItemDTO, len(l.Items))}
		for j, it := range l.Items {
			rl.Items[j] = receiptItemDTO{ItemID: it.ItemID, Code: it.Code, Quantity: it.Quantity}
		}
		d.Lines[i] = rl
	}
	return d
}

func toDTO(o domain.Order, withReceipts bool) orderDTO {
	d := orderDTO{
		ID: o.ID, SupplierID: o.SupplierID, SupplierName: o.SupplierName, Status: string(o.Status),
		Currency: string(o.Currency), ExchangeRate: o.ExchangeRate, Notes: o.Notes, TotalMinor: o.Total(),
		Lines: make([]lineDTO, len(o.Lines)), CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
		OrderedAt: o.OrderedAt, CancelledAt: o.CancelledAt,
	}
	if o.ExpectedAt != nil {
		s := o.ExpectedAt.Format(dateLayout)
		d.ExpectedAt = &s
	}
	for i, l := range o.Lines {
		d.Lines[i] = toLineDTO(l)
	}
	if withReceipts {
		d.Receipts = make([]receiptDTO, len(o.Receipts))
		for i, r := range o.Receipts {
			d.Receipts[i] = toReceiptDTO(o.ID, r)
		}
	}
	return d
}

// ---- request decoding ----

type lineReq struct {
	ItemID        string      `json:"item_id"`
	NewItem       *newItemDTO `json:"new_item"`
	Quantity      int         `json:"quantity"`
	UnitCostMinor int64       `json:"unit_cost_minor"`
	// Read-only fields of the line DTO, accepted and ignored so an order
	// fetched with GET can be edited and sent back.
	ID          json.RawMessage `json:"id"`
	Description json.RawMessage `json:"description"`
	ReceivedQty json.RawMessage `json:"received_qty"`
}

func (l lineReq) toDomain() domain.LineInput {
	in := domain.LineInput{ItemID: l.ItemID, Quantity: l.Quantity, UnitCostMinor: l.UnitCostMinor}
	if n := l.NewItem; n != nil {
		in.NewItem = &domain.NewItem{
			Description: n.Description, CategoryID: n.CategoryID, Kind: n.Kind, Unit: n.Unit,
			StockMin: n.StockMin, Attributes: n.Attributes, Notes: n.Notes,
		}
	}
	return in
}

func linesToDomain(in []lineReq) []domain.LineInput {
	out := make([]domain.LineInput, len(in))
	for i, l := range in {
		out[i] = l.toDomain()
	}
	return out
}

// nullable distinguishes an absent JSON field from an explicit null.
type nullable[T any] struct {
	Set bool
	Val *T
}

func (n *nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return nil
	}
	n.Val = new(T)
	return json.Unmarshal(b, n.Val)
}

// parseDate turns a nullable date string into the use case's optional value.
func parseDate(n nullable[string], fields map[string]string) app.Opt[time.Time] {
	out := app.Opt[time.Time]{Set: n.Set}
	if n.Val == nil {
		return out
	}
	t, err := time.Parse(dateLayout, *n.Val)
	if err != nil {
		fields["expected_at"] = "must be a date like 2026-12-31"
		return out
	}
	out.Val = &t
	return out
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest, "malformed JSON body")
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "purchase order not found")
		return 0, false
	}
	return id, true
}

func userID(r *http.Request) string {
	if p, ok := httpx.PrincipalFrom(r.Context()); ok {
		return p.UserID
	}
	return ""
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	var ise *domain.InvalidStateError
	switch {
	case errors.As(err, &ve):
		if ve.Code == "" || ve.Code == domain.CodeValidation {
			httpx.WriteFieldErrors(w, ve.Fields)
			return
		}
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: httpx.ErrorDetail{
			Code: ve.Code, Message: ve.Message, Fields: ve.Fields}})
	case errors.As(err, &ise):
		httpx.WriteError(w, http.StatusConflict, codeInvalidState, ise.Error())
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "purchase order not found")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

// ---- handlers ----

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.Filter{Q: q.Get("q"), Status: domain.Status(q.Get("status"))}
	fields := map[string]string{}
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				fields[name] = "must be an integer"
			}
			*dst = n
		}
	}
	if v := q.Get("supplier_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fields["supplier_id"] = "must be an integer"
		}
		f.SupplierID = n
	}
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	orders, total, nf, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]orderDTO, len(orders))
	for i, o := range orders {
		out[i] = toDTO(o, false)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out, "total": total, "limit": nf.Limit, "offset": nf.Offset,
	})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(o, true))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SupplierID   int64     `json:"supplier_id"`
		Currency     string    `json:"currency"`
		ExchangeRate *float64  `json:"exchange_rate"`
		ExpectedAt   *string   `json:"expected_at"`
		Notes        string    `json:"notes"`
		Lines        []lineReq `json:"lines"`
	}
	if !decode(w, r, &in) {
		return
	}
	fields := map[string]string{}
	exp := parseDate(nullable[string]{Set: true, Val: in.ExpectedAt}, fields)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	o, err := h.svc.Create(r.Context(), userID(r), domain.Input{
		SupplierID: in.SupplierID, Currency: domain.Currency(in.Currency), ExchangeRate: in.ExchangeRate,
		ExpectedAt: exp.Val, Notes: in.Notes, Lines: linesToDomain(in.Lines),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(o, true))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		SupplierID   *int64            `json:"supplier_id"`
		Currency     *string           `json:"currency"`
		ExchangeRate nullable[float64] `json:"exchange_rate"`
		ExpectedAt   nullable[string]  `json:"expected_at"`
		Notes        *string           `json:"notes"`
		Lines        *[]lineReq        `json:"lines"`
	}
	if !decode(w, r, &in) {
		return
	}
	fields := map[string]string{}
	p := app.Patch{
		SupplierID:   in.SupplierID,
		ExchangeRate: app.Opt[float64]{Set: in.ExchangeRate.Set, Val: in.ExchangeRate.Val},
		ExpectedAt:   parseDate(in.ExpectedAt, fields),
		Notes:        in.Notes,
	}
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	if in.Currency != nil {
		c := domain.Currency(*in.Currency)
		p.Currency = &c
	}
	if in.Lines != nil {
		p.Lines = linesToDomain(*in.Lines)
	}
	o, err := h.svc.Update(r.Context(), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(o, true))
}

func (h *Handler) place(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.Place(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(o, true))
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	o, err := h.svc.Cancel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(o, true))
}

func (h *Handler) receive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		LocationID int64  `json:"location_id"`
		Note       string `json:"note"`
		ClientKey  string `json:"client_key"`
		Lines      []struct {
			LineID   int64 `json:"line_id"`
			Quantity int   `json:"quantity"`
		} `json:"lines"`
	}
	if !decode(w, r, &in) {
		return
	}
	req := domain.ReceiveRequest{LocationID: in.LocationID, Note: in.Note, ClientKey: in.ClientKey}
	for _, l := range in.Lines {
		req.Lines = append(req.Lines, domain.ReceiveLine{LineID: l.LineID, Quantity: l.Quantity})
	}
	res, err := h.svc.Receive(r.Context(), id, userID(r), req)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	httpx.WriteJSON(w, status, map[string]any{
		"receipt": toReceiptDTO(res.Order.ID, res.Receipt),
		"order":   toDTO(res.Order, true),
	})
}
