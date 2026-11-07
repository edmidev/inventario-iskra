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
		r.Route("/executions", func(r chi.Router) {
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/", h.list)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/assignable-users", h.assignableUsers)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}", h.get)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/", h.create)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/start", h.start)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/pause", h.pause)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/{id}/resume", h.resume)
		})
	})
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
	switch {
	case errors.As(err, &validation):
		httpx.WriteFieldErrors(w, validation.Fields)
	case errors.Is(err, app.ErrProjectNotFound), errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "execution or project not found")
	case errors.Is(err, domain.ErrInactiveAssignment):
		httpx.WriteError(w, http.StatusUnprocessableEntity, "invalid_assignment", "customers and assigned users must be active")
	case errors.Is(err, domain.ErrInvalidState):
		httpx.WriteError(w, http.StatusConflict, "invalid_execution_state", "execution cannot transition from its current state")
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
