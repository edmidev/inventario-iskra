// Package httpapi exposes manufacturing executions over HTTP.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/edgar/inventario/backend/internal/executions/app"
	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
	"github.com/go-chi/chi/v5"
)

type Permissions struct{ Read, Write string }

type Handler struct {
	svc          *app.Service
	authenticate func(http.Handler) http.Handler
	perms        Permissions
}

func New(service *app.Service, authenticate func(http.Handler) http.Handler, permissions Permissions) *Handler {
	return &Handler{svc: service, authenticate: authenticate, perms: permissions}
}

func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.With(httpx.RequirePermission(h.perms.Read)).Get("/finished-products", h.listFinishedStock)
		r.Route("/executions", func(r chi.Router) {
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/", h.list)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/assignable-users", h.assignableUsers)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}", h.get)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}/units", h.listUnits)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}/deliveries", h.listDeliveries)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}/events", h.listEvents)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/", h.create)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/start", h.start)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/pause", h.pause)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/resume", h.resume)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/complete", h.complete)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/cancel", h.cancel)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/produce", h.produce)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/deliveries", h.deliverUnits)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/events", h.recordIncident)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/materials", h.addMaterial)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/tools", h.addTool)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/purchase-lines", h.linkPurchaseLine)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/materials/{itemID}/deliver", h.deliverMaterial)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/materials/{itemID}/return", h.returnMaterial)
		})
	})
}

func (h *Handler) listFinishedStock(w http.ResponseWriter, r *http.Request) {
	filter := domain.FinishedStockFilter{Status: r.URL.Query().Get("status"), Q: r.URL.Query().Get("q")}
	for key, target := range map[string]*int{"limit": &filter.Limit, "offset": &filter.Offset} {
		if value := r.URL.Query().Get(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				httpx.WriteFieldErrors(w, map[string]string{key: "must be an integer"})
				return
			}
			*target = n
		}
	}
	rows, total, normalized, err := h.svc.ListFinishedStock(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rows, "total": total, "limit": normalized.Limit, "offset": normalized.Offset})
}

func (h *Handler) linkPurchaseLine(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	var input struct {
		PurchaseOrderLineID int64             `json:"purchase_order_line_id"`
		Quantity            quantity.Quantity `json:"quantity"`
	}
	if !decode(w, r, &input) {
		return
	}
	execution, err := h.svc.LinkPurchaseOrderLine(r.Context(), id, input.PurchaseOrderLineID, input.Quantity)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest, "malformed JSON body")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest, "expected one JSON value")
		return false
	}
	return true
}

func executionID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "execution not found")
		return 0, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	var validation *domain.ValidationError
	var stockValidation *stockdomain.ValidationError
	switch {
	case errors.As(err, &validation):
		httpx.WriteFieldErrors(w, validation.Fields)
	case errors.As(err, &stockValidation):
		httpx.WriteFieldErrors(w, stockValidation.Fields)
	case errors.Is(err, app.ErrProjectNotFound), errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "execution or project not found")
	case errors.Is(err, domain.ErrInactiveAssignment):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_assignment", "customers and assigned users must be active")
	case errors.Is(err, domain.ErrMaterialUnavailable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_material", "supplemental material must reference an active consumable")
	case errors.Is(err, domain.ErrToolUnavailable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_tool", "supplemental tool must reference an inventory asset")
	case errors.Is(err, domain.ErrInvalidState):
		httpx.WriteError(w, http.StatusConflict, "invalid_execution_state", "execution cannot transition from its current state")
	case errors.Is(err, domain.ErrProductionLimit):
		httpx.WriteError(w, http.StatusConflict, "production_limit", "production exceeds the planned unit count")
	case errors.Is(err, domain.ErrProductionIncomplete):
		httpx.WriteError(w, http.StatusConflict, "production_incomplete", "all planned units must be produced before completion")
	case errors.Is(err, domain.ErrProductionReplay):
		httpx.WriteError(w, http.StatusConflict, "request_id_reused", "request_id was already used for a different production batch")
	case errors.Is(err, domain.ErrDeliveryReplay):
		httpx.WriteError(w, http.StatusConflict, "request_id_reused", "request_id was already used for a different finished-product delivery")
	case errors.Is(err, domain.ErrIncidentReplay):
		httpx.WriteError(w, http.StatusConflict, "request_id_reused", "request_id was already used for a different incident")
	case errors.Is(err, domain.ErrCustomerNotAssigned):
		httpx.WriteFieldErrors(w, map[string]string{"customer_id": "customer is not assigned to this execution"})
	case errors.Is(err, domain.ErrUnitUnavailable):
		httpx.WriteFieldErrors(w, map[string]string{"unit_ids": "finished unit does not belong to this execution"})
	case errors.Is(err, domain.ErrUnitDelivered):
		httpx.WriteError(w, http.StatusConflict, "unit_already_delivered", "finished unit was already delivered")
	case errors.Is(err, domain.ErrMaterialAdditionClosed):
		httpx.WriteError(w, http.StatusConflict, "execution_not_active", "materials can only be added while the execution is active")
	case errors.Is(err, domain.ErrToolAdditionClosed):
		httpx.WriteError(w, http.StatusConflict, "execution_not_active", "tools can only be added while the execution is active")
	case errors.Is(err, stockdomain.ErrInsufficientStock), errors.Is(err, stockdomain.ErrReservationUnavailable):
		httpx.WriteError(w, http.StatusConflict, "material_unavailable", "the execution does not have enough reserved material")
	case errors.Is(err, stockdomain.ErrExecutionInactive):
		httpx.WriteError(w, http.StatusConflict, "execution_not_active", "materials can only move for an active execution")
	case errors.Is(err, stockdomain.ErrIdempotencyConflict):
		httpx.WriteError(w, http.StatusConflict, "request_id_reused", "request_id was already used for a different material movement")
	case errors.Is(err, stockdomain.ErrLocationNotFound):
		httpx.WriteFieldErrors(w, map[string]string{"location_id": "location does not exist"})
	case errors.Is(err, stockdomain.ErrInvalidInput), errors.Is(err, stockdomain.ErrNotApplicable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_material_movement", "invalid execution material movement")
	default:
		slog.Error("execution error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateInput
	if !decode(w, r, &input) {
		return
	}
	created, err := h.svc.Create(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	execution, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}

func (h *Handler) listUnits(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	units, err := h.svc.ListUnits(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": units})
}

func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	receipts, err := h.svc.ListDeliveries(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": receipts})
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	events, err := h.svc.ListEvents(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": events})
}

func (h *Handler) recordIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input domain.IncidentInput
	if !decode(w, r, &input) {
		return
	}
	event, err := h.svc.RecordIncident(r.Context(), id, principal.UserID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, event)
}

func (h *Handler) deliverUnits(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input domain.DeliveryInput
	if !decode(w, r, &input) {
		return
	}
	receipt, err := h.svc.DeliverFinishedUnits(r.Context(), id, principal.UserID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, receipt)
}

func (h *Handler) produce(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input domain.ProduceInput
	if !decode(w, r, &input) {
		return
	}
	units, err := h.svc.Produce(r.Context(), id, principal.UserID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": units})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	filter := domain.Filter{Q: r.URL.Query().Get("q")}
	for key, target := range map[string]*int{"limit": &filter.Limit, "offset": &filter.Offset} {
		if value := r.URL.Query().Get(key); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				httpx.WriteFieldErrors(w, map[string]string{key: "must be an integer"})
				return
			}
			*target = n
		}
	}
	if value := r.URL.Query().Get("status"); value != "" {
		status := domain.Status(value)
		filter.Status = &status
	}
	rows, count, normalized, err := h.svc.List(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": rows, "total": count, "limit": normalized.Limit, "offset": normalized.Offset})
}

func (h *Handler) assignableUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListAssignableUsers(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": users})
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request)  { h.transition(w, r, h.svc.Start) }
func (h *Handler) pause(w http.ResponseWriter, r *http.Request)  { h.transition(w, r, h.svc.Pause) }
func (h *Handler) resume(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.svc.Resume) }
func (h *Handler) complete(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.svc.Complete)
}
func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) { h.transition(w, r, h.svc.Cancel) }

func (h *Handler) addMaterial(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input domain.AddMaterialInput
	if !decode(w, r, &input) {
		return
	}
	execution, err := h.svc.AddMaterialRequirement(r.Context(), id, principal.UserID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}

func (h *Handler) addTool(w http.ResponseWriter, r *http.Request) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input domain.AddToolInput
	if !decode(w, r, &input) {
		return
	}
	execution, err := h.svc.AddToolRequirement(r.Context(), id, principal.UserID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, transition func(context.Context, int64) (domain.Execution, error)) {
	id, ok := executionID(w, r)
	if !ok {
		return
	}
	execution, err := transition(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}

type materialMovementInput struct {
	RequestID  string            `json:"request_id"`
	Quantity   quantity.Quantity `json:"quantity"`
	LocationID int64             `json:"location_id"`
	Note       string            `json:"note"`
}

func (h *Handler) deliverMaterial(w http.ResponseWriter, r *http.Request) {
	h.materialMovement(w, r, h.svc.DeliverMaterial)
}

func (h *Handler) returnMaterial(w http.ResponseWriter, r *http.Request) {
	h.materialMovement(w, r, h.svc.ReturnMaterial)
}

func (h *Handler) materialMovement(w http.ResponseWriter, r *http.Request, move func(context.Context, int64, string, string, string, quantity.Quantity, int64, string) (domain.Execution, error)) {
	executionID, ok := executionID(w, r)
	if !ok {
		return
	}
	principal, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var input materialMovementInput
	if !decode(w, r, &input) {
		return
	}
	execution, err := move(r.Context(), executionID, chi.URLParam(r, "itemID"), principal.UserID, input.RequestID, input.Quantity, input.LocationID, input.Note)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, execution)
}
