// Package httpapi exposes the custody use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/edgar/inventario/backend/internal/custody/app"
	"github.com/edgar/inventario/backend/internal/custody/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read       string // history and active loans
	WriteItems string // assign and return
}

// Handler serves loans and returns.
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
		r.With(write).Post("/items/{id}/assign", h.assign)
		r.With(write).Post("/items/{id}/return", h.returnItem)
		r.With(read).Get("/items/{id}/custody", h.history)
		r.With(read).Get("/custody/active", h.active)
	})
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteFieldErrors(w, ve.Fields)
	case errors.Is(err, domain.ErrItemNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "item not found")
	case errors.Is(err, domain.ErrNotApplicable):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "not_applicable",
			"only assets can be lent; consumables move with stock transfers")
	case errors.Is(err, domain.ErrItemNotAvailable):
		httpx.WriteError(w, http.StatusConflict, "item_not_available", "only functional items can be lent")
	case errors.Is(err, domain.ErrAlreadyAssigned):
		httpx.WriteError(w, http.StatusConflict, "already_assigned", "item is already on loan")
	case errors.Is(err, domain.ErrNotAssigned):
		httpx.WriteError(w, http.StatusConflict, "not_assigned", "item is not on loan")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

type assignmentDTO struct {
	ID                 int64      `json:"id"`
	ItemID             string     `json:"item_id"`
	ItemCode           string     `json:"item_code"`
	ItemDescription    string     `json:"item_description"`
	AssigneeLocationID int64      `json:"assignee_location_id"`
	AssigneeName       string     `json:"assignee_name"`
	AssigneeKind       string     `json:"assignee_kind"`
	OriginLocationID   *int64     `json:"origin_location_id"`
	OriginName         *string    `json:"origin_name"`
	AssignedAt         time.Time  `json:"assigned_at"`
	DueAt              *time.Time `json:"due_at"`
	Overdue            bool       `json:"overdue"`
	ReturnedAt         *time.Time `json:"returned_at"`
	ReturnedToID       *int64     `json:"returned_to_location_id"`
	ReturnedToName     *string    `json:"returned_to_name"`
	AssignedBy         string     `json:"assigned_by"`
	AssignedByName     string     `json:"assigned_by_name"`
	ReturnedBy         *string    `json:"returned_by"`
	ReturnedByName     *string    `json:"returned_by_name"`
	AssignNote         string     `json:"assign_note"`
	ReturnNote         string     `json:"return_note"`
	CreatedAt          time.Time  `json:"created_at"`
}

func (h *Handler) toDTO(a domain.Assignment) assignmentDTO {
	return assignmentDTO{
		ID: a.ID, ItemID: a.ItemID, ItemCode: a.ItemCode, ItemDescription: a.ItemDescription,
		AssigneeLocationID: a.AssigneeLocationID, AssigneeName: a.AssigneeName, AssigneeKind: string(a.AssigneeKind),
		OriginLocationID: a.OriginLocationID, OriginName: a.OriginName, AssignedAt: a.AssignedAt,
		DueAt: a.DueAt, Overdue: a.Overdue(h.svc.Now()), ReturnedAt: a.ReturnedAt,
		ReturnedToID: a.ReturnedToID, ReturnedToName: a.ReturnedToName,
		AssignedBy: a.AssignedBy, AssignedByName: a.AssignedByName, ReturnedBy: a.ReturnedBy, ReturnedByName: a.ReturnedByName,
		AssignNote: a.AssignNote, ReturnNote: a.ReturnNote, CreatedAt: a.CreatedAt,
	}
}

func (h *Handler) list(w http.ResponseWriter, as []domain.Assignment, total int, p domain.Page) {
	out := make([]assignmentDTO, len(as))
	for i, a := range as {
		out[i] = h.toDTO(a)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out, "total": total, "limit": p.Limit, "offset": p.Offset})
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

func (h *Handler) assign(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var in struct {
		AssigneeLocationID int64      `json:"assignee_location_id"`
		DueAt              *time.Time `json:"due_at"`
		Note               string     `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	a, err := h.svc.Assign(r.Context(), chi.URLParam(r, "id"), p.UserID, domain.AssignRequest{
		AssigneeLocationID: in.AssigneeLocationID, DueAt: in.DueAt, Note: in.Note})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, h.toDTO(a))
}

func (h *Handler) returnItem(w http.ResponseWriter, r *http.Request) {
	p, ok := httpx.PrincipalFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "authentication required")
		return
	}
	var in struct {
		ReturnLocationID *int64 `json:"return_location_id"`
		Note             string `json:"note"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) { // the body is optional
		return
	}
	a, err := h.svc.Return(r.Context(), chi.URLParam(r, "id"), p.UserID, domain.ReturnRequest{
		ReturnLocationID: in.ReturnLocationID, Note: in.Note})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, h.toDTO(a))
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
	as, total, page, err := h.svc.History(r.Context(), chi.URLParam(r, "id"), limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	h.list(w, as, total, page)
}

func (h *Handler) active(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fields := map[string]string{}
	limit, offset := queryInt(r, "limit", fields), queryInt(r, "offset", fields)
	f := domain.ActiveFilter{Q: q.Get("q")}
	if v := q.Get("assignee_location_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fields["assignee_location_id"] = "must be an integer"
		}
		f.AssigneeLocationID = &n
	}
	switch q.Get("overdue") {
	case "", "false":
	case "true":
		f.OverdueOnly = true
	default:
		fields["overdue"] = "must be true or false"
	}
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	as, total, page, err := h.svc.Active(r.Context(), f, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	h.list(w, as, total, page)
}
