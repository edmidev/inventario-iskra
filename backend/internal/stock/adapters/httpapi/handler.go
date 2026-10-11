// Package httpapi exposes the stock use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/stock/app"
	"github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read       string // movement history and alerts
	WriteItems string // record movements
}

// Handler serves stock movements and alerts.
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
	write := httpx.RequirePermission(h.perms.WriteItems)
	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.With(write).Post("/items/{id}/movements", h.record)
		r.With(read).Get("/items/{id}/movements", h.history)
		r.With(read).Get("/stock/alerts", h.alerts)
	})
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	var ie *domain.InsufficientStockError
	var lr *domain.LocationRequiredError
	switch {
	case errors.As(err, &ve):
		httpx.WriteFieldErrors(w, ve.Fields)
	case errors.As(err, &lr):
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{Error: httpx.ErrorDetail{
			Code: "location_required", Fields: lr.Fields,
			Message: "this consumable is stocked in several locations; say which one"}})
	case errors.Is(err, domain.ErrItemNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "item not found")
	case errors.Is(err, domain.ErrNotApplicable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "not_applicable",
			"this movement only applies to consumables; assets can only be transferred")
	case errors.Is(err, domain.ErrItemOnLoan):
		httpx.WriteError(w, http.StatusConflict, "item_on_loan",
			"item is on loan; return it before changing its location")
	case errors.As(err, &ie):
		msg := fmt.Sprintf("only %d in stock, %d requested", ie.Available, ie.Requested)
		if ie.LocationName != "" {
			msg = fmt.Sprintf("only %d available in %q, %d requested", ie.Available, ie.LocationName, ie.Requested)
		}
		httpx.WriteError(w, http.StatusConflict, "insufficient_stock", msg)
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

type movementDTO struct {
	ID             int64     `json:"id"`
	ItemID         string    `json:"item_id"`
	Type           string    `json:"type"`
	Qty            int       `json:"qty"`
	StockBefore    int       `json:"stock_before"`
	StockAfter     int       `json:"stock_after"`
	LocationID     *int64    `json:"location_id"`
	FromLocationID *int64    `json:"from_location_id"`
	ToLocationID   *int64    `json:"to_location_id"`
	MovedQty       int       `json:"moved_qty"`
	UserID         *string   `json:"user_id"`
	UserName       *string   `json:"user_name"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
}

func toMovementDTO(m domain.Movement) movementDTO {
	return movementDTO{ID: m.ID, ItemID: m.ItemID, Type: string(m.Type), Qty: m.Qty, StockBefore: m.StockBefore,
		StockAfter: m.StockAfter, LocationID: m.LocationID, FromLocationID: m.FromLocationID, ToLocationID: m.ToLocationID,
		MovedQty: m.MovedQty, UserID: m.UserID,
		UserName: m.UserName, Note: m.Note, CreatedAt: m.CreatedAt}
}

func (h *Handler) record(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var in struct {
		Type           string `json:"type"`
		Qty            int    `json:"qty"`
		LocationID     *int64 `json:"location_id"`
		FromLocationID *int64 `json:"from_location_id"`
		ToLocationID   *int64 `json:"to_location_id"`
		Note           string `json:"note"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest, "malformed JSON body")
		return
	}
	res, err := h.svc.Record(r.Context(), chi.URLParam(r, "id"), p.UserID, domain.Request{
		Type: domain.MovementType(in.Type), Qty: in.Qty, LocationID: in.LocationID,
		FromLocationID: in.FromLocationID, ToLocationID: in.ToLocationID, Note: in.Note})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"movement":      toMovementDTO(res.Movement),
		"stock_current": res.StockCurrent,
		"location_id":   res.LocationID,
		"stocks":        toStockDTOs(res.Stocks),
	})
}

// pageParams reads limit/offset; unparsable values become 422 field errors.
func pageParams(r *http.Request) (limit, offset int, fields map[string]string) {
	fields = map[string]string{}
	parse := func(name string) int {
		v := r.URL.Query().Get(name)
		if v == "" {
			return 0
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			fields[name] = "must be an integer"
		}
		return n
	}
	return parse("limit"), parse("offset"), fields
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	limit, offset, fields := pageParams(r)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	ms, total, page, err := h.svc.History(r.Context(), chi.URLParam(r, "id"),
		domain.MovementType(r.URL.Query().Get("type")), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]movementDTO, len(ms))
	for i, m := range ms {
		out[i] = toMovementDTO(m)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "limit": page.Limit, "offset": page.Offset})
}

// stockDTO is one location's quantity of a consumable.
type stockDTO struct {
	LocationID   int64  `json:"location_id"`
	LocationName string `json:"location_name"`
	Quantity     int    `json:"quantity"`
}

func toStockDTOs(in []domain.LocationStock) []stockDTO {
	out := make([]stockDTO, len(in))
	for i, s := range in {
		out[i] = stockDTO{LocationID: s.LocationID, LocationName: s.LocationName, Quantity: s.Quantity}
	}
	return out
}

type alertDTO struct {
	ItemID       string  `json:"item_id"`
	Code         string  `json:"code"`
	Description  string  `json:"description"`
	Unit         string  `json:"unit"`
	StockCurrent int     `json:"stock_current"`
	StockMin     int     `json:"stock_min"`
	Shortage     int     `json:"shortage"`
	LocationID   *int64  `json:"location_id"`
	LocationName *string `json:"location_name"`
	// Stocks is the per-location breakdown behind StockCurrent.
	Stocks []stockDTO `json:"stocks"`
}

func (h *Handler) alerts(w http.ResponseWriter, r *http.Request) {
	limit, offset, fields := pageParams(r)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	as, total, page, err := h.svc.Alerts(r.Context(), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]alertDTO, len(as))
	for i, a := range as {
		out[i] = alertDTO{ItemID: a.ItemID, Code: a.Code, Description: a.Description, Unit: a.Unit,
			StockCurrent: a.StockCurrent, StockMin: a.StockMin, Shortage: a.Shortage(),
			LocationID: a.LocationID, LocationName: a.LocationName, Stocks: toStockDTOs(a.Stocks)}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "limit": page.Limit, "offset": page.Offset})
}
