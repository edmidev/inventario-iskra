// Package httpapi exposes the maintenance use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/maintenance/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read       string // history, listings and due maintenance
	WriteItems string // report failures, register services, edit and close
}

// Handler serves maintenance records and due maintenance.
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
		r.With(write).Post("/items/{id}/maintenance", h.create)
		r.With(read).Get("/items/{id}/maintenance", h.history)
		r.With(write).Patch("/maintenance/{id}", h.update)
		r.With(write).Post("/maintenance/{id}/close", h.close)
		r.With(read).Get("/maintenance", h.list)
		r.With(read).Get("/maintenance/due", h.due)
	})
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteFieldErrors(w, ve.Fields)
	case errors.Is(err, domain.ErrItemNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "item not found")
	case errors.Is(err, domain.ErrRecordNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "maintenance record not found")
	case errors.Is(err, domain.ErrNotApplicable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "not_applicable",
			"maintenance only applies to assets, not consumables")
	case errors.Is(err, domain.ErrOpenFailureExists):
		httpx.WriteError(w, http.StatusConflict, "open_failure_exists", "item already has an open failure; close it first")
	case errors.Is(err, domain.ErrRecordClosed):
		httpx.WriteError(w, http.StatusConflict, "record_closed", "maintenance record is already closed")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

type recordDTO struct {
	ID              int64      `json:"id"`
	ItemID          string     `json:"item_id"`
	ItemCode        string     `json:"item_code"`
	ItemDescription string     `json:"item_description"`
	ItemStatus      string     `json:"item_status"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	ReportedAt      time.Time  `json:"reported_at"`
	Description     string     `json:"description"`
	Destination     *string    `json:"destination"`
	Cost            *float64   `json:"cost"`
	NextDueAt       *time.Time `json:"next_due_at"`
	ClosedAt        *time.Time `json:"closed_at"`
	ResolutionNote  *string    `json:"resolution_note"`
	ReportedBy      string     `json:"reported_by"`
	ReportedByName  string     `json:"reported_by_name"`
	ClosedBy        *string    `json:"closed_by"`
	ClosedByName    *string    `json:"closed_by_name"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func toRecordDTO(r domain.Record) recordDTO {
	return recordDTO{
		ID: r.ID, ItemID: r.ItemID, ItemCode: r.ItemCode, ItemDescription: r.ItemDescription, ItemStatus: string(r.ItemStatus),
		Kind: string(r.Kind), Status: string(r.Status), ReportedAt: r.ReportedAt, Description: r.Description,
		Destination: r.Destination, Cost: r.Cost, NextDueAt: r.NextDueAt, ClosedAt: r.ClosedAt, ResolutionNote: r.ResolutionNote,
		ReportedBy: r.ReportedBy, ReportedByName: r.ReportedByName, ClosedBy: r.ClosedBy, ClosedByName: r.ClosedByName,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func writeRecords(w http.ResponseWriter, rs []domain.Record, total int, p domain.Page) {
	out := make([]recordDTO, len(rs))
	for i, r := range rs {
		out[i] = toRecordDTO(r)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "limit": p.Limit, "offset": p.Offset})
}

type dueDTO struct {
	ItemID                  string     `json:"item_id"`
	Code                    string     `json:"code"`
	Description             string     `json:"description"`
	CategoryID              int64      `json:"category_id"`
	CategoryName            string     `json:"category_name"`
	Status                  string     `json:"status"`
	LocationID              *int64     `json:"location_id"`
	LocationName            *string    `json:"location_name"`
	LastMaintenanceAt       *time.Time `json:"last_maintenance_at"`
	MaintenanceIntervalDays int        `json:"maintenance_interval_days"`
	MaintenanceDueAt        time.Time  `json:"maintenance_due_at"`
	MaintenanceOverdue      bool       `json:"maintenance_overdue"`
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

// optString tells an absent JSON field from an explicit null or string.
type optString struct {
	Set   bool
	Value *string
}

// UnmarshalJSON implements json.Unmarshaler; it only runs when the field is present.
func (o *optString) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

func principal(w http.ResponseWriter, r *http.Request) (httpx.Principal, bool) {
	p, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
	}
	return p, ok
}

// recordID parses the {id} path parameter; non-numeric ids are simply unknown.
func recordID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id, err == nil && id > 0
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	var in struct {
		Kind        string     `json:"kind"`
		Description string     `json:"description"`
		ReportedAt  *time.Time `json:"reported_at"`
		Destination *string    `json:"destination"`
		ItemStatus  string     `json:"item_status"`
		PerformedAt *time.Time `json:"performed_at"`
		Cost        *float64   `json:"cost"`
		NextDueAt   *time.Time `json:"next_due_at"`
	}
	if !decode(w, r, &in) {
		return
	}
	rec, err := h.svc.Create(r.Context(), chi.URLParam(r, "id"), p.UserID, domain.CreateRequest{
		Kind: domain.Kind(in.Kind), Description: in.Description, ReportedAt: in.ReportedAt, Destination: in.Destination,
		ItemStatus: domain.ItemStatus(in.ItemStatus), PerformedAt: in.PerformedAt, Cost: in.Cost, NextDueAt: in.NextDueAt})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toRecordDTO(rec))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := recordID(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "maintenance record not found")
		return
	}
	var in struct {
		Description *string   `json:"description"`
		Destination optString `json:"destination"`
		ItemStatus  string    `json:"item_status"`
	}
	if !decode(w, r, &in) {
		return
	}
	rec, err := h.svc.Update(r.Context(), id, domain.UpdateRequest{
		Description: in.Description, SetDestination: in.Destination.Set, Destination: in.Destination.Value,
		ItemStatus: domain.ItemStatus(in.ItemStatus)})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRecordDTO(rec))
}

func (h *Handler) close(w http.ResponseWriter, r *http.Request) {
	p, ok := principal(w, r)
	if !ok {
		return
	}
	id, ok := recordID(r)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "maintenance record not found")
		return
	}
	var in struct {
		ClosedAt        *time.Time `json:"closed_at"`
		Cost            *float64   `json:"cost"`
		ResolutionNote  string     `json:"resolution_note"`
		ItemStatusAfter string     `json:"item_status_after"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) { // the body is optional
		return
	}
	rec, err := h.svc.Close(r.Context(), id, p.UserID, domain.CloseRequest{
		ClosedAt: in.ClosedAt, Cost: in.Cost, ResolutionNote: in.ResolutionNote, ItemStatusAfter: domain.ItemStatus(in.ItemStatusAfter)})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toRecordDTO(rec))
}

// queryInt parses an optional integer query parameter, recording a field error.
func queryInt(r *http.Request, name string, fields map[string]string) int {
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

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	fields := map[string]string{}
	limit, offset := queryInt(r, "limit", fields), queryInt(r, "offset", fields)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	rs, total, page, err := h.svc.History(r.Context(), chi.URLParam(r, "id"), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	writeRecords(w, rs, total, page)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fields := map[string]string{}
	limit, offset := queryInt(r, "limit", fields), queryInt(r, "offset", fields)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	rs, total, page, err := h.svc.List(r.Context(), domain.ListFilter{
		Status: domain.Status(q.Get("status")), Kind: domain.Kind(q.Get("kind")), Destination: q.Get("destination"),
		ItemID: q.Get("item_id"), Q: q.Get("q")}, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	writeRecords(w, rs, total, page)
}

func (h *Handler) due(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fields := map[string]string{}
	limit, offset := queryInt(r, "limit", fields), queryInt(r, "offset", fields)
	f := domain.DueFilter{Q: q.Get("q"), WithinDays: queryInt(r, "within_days", fields)}
	if v := q.Get("category_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fields["category_id"] = "must be an integer"
		}
		f.CategoryID = &n
	}
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	items, total, page, now, err := h.svc.Due(r.Context(), f, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]dueDTO, len(items))
	for i, d := range items {
		out[i] = dueDTO{ItemID: d.ItemID, Code: d.Code, Description: d.Description, CategoryID: d.CategoryID,
			CategoryName: d.CategoryName, Status: string(d.Status), LocationID: d.LocationID, LocationName: d.LocationName,
			LastMaintenanceAt: d.LastMaintenanceAt, MaintenanceIntervalDays: d.IntervalDays, MaintenanceDueAt: d.DueAt,
			MaintenanceOverdue: d.Overdue(now)}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "limit": page.Limit, "offset": page.Offset})
}
