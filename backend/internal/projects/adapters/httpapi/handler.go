// Package httpapi exposes reusable project templates over HTTP.
package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/projects/app"
	"github.com/edgar/inventario/backend/internal/projects/domain"
	"github.com/go-chi/chi/v5"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

type Permissions struct{ Read, Write string }
type Handler struct {
	svc          *app.Service
	authenticate func(http.Handler) http.Handler
	perms        Permissions
}

func New(s *app.Service, a func(http.Handler) http.Handler, p Permissions) *Handler {
	return &Handler{s, a, p}
}
func (h *Handler) Register(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.Route("/projects", func(r chi.Router) {
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/", h.list)
			r.With(httpx.RequirePermission(h.perms.Read)).Get("/{id}", h.get)
			r.With(httpx.RequirePermission(h.perms.Write)).Post("/", h.create)
			r.With(httpx.RequirePermission(h.perms.Write)).Put("/{id}", h.update)
			r.With(httpx.RequirePermission(h.perms.Write)).Delete("/{id}", h.delete)
		})
	})
}
func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(dst); e != nil {
		httpx.WriteError(w, 400, httpx.CodeInvalidRequest, "malformed JSON body")
		return false
	}
	if e := d.Decode(&struct{}{}); e != io.EOF {
		httpx.WriteError(w, 400, httpx.CodeInvalidRequest, "expected one JSON value")
		return false
	}
	return true
}
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if e != nil || id < 1 {
		httpx.WriteError(w, 404, httpx.CodeNotFound, "project not found")
		return 0, false
	}
	return id, true
}
func writeError(w http.ResponseWriter, e error) {
	var v *domain.ValidationError
	switch {
	case errors.As(e, &v):
		httpx.WriteFieldErrors(w, v.Fields)
	case errors.Is(e, domain.ErrNotFound):
		httpx.WriteError(w, 404, httpx.CodeNotFound, "project not found")
	case errors.Is(e, domain.ErrNameTaken):
		httpx.WriteError(w, 409, "project_name_taken", "a project with this name already exists")
	default:
		slog.Error("project error", "err", e)
		httpx.WriteError(w, 500, httpx.CodeInternal, "internal error")
	}
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	f := domain.Filter{Q: r.URL.Query().Get("q")}
	for k, p := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := r.URL.Query().Get(k); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				httpx.WriteFieldErrors(w, map[string]string{k: "must be an integer"})
				return
			}
			*p = n
		}
	}
	rows, n, nf, e := h.svc.List(r.Context(), f)
	if e != nil {
		writeError(w, e)
		return
	}
	httpx.WriteJSON(w, 200, map[string]any{"data": rows, "total": n, "limit": nf.Limit, "offset": nf.Offset})
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, e := h.svc.Get(r.Context(), id)
	if e != nil {
		writeError(w, e)
		return
	}
	httpx.WriteJSON(w, 200, p)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in domain.Input
	if !decode(w, r, &in) {
		return
	}
	p, e := h.svc.Create(r.Context(), in)
	if e != nil {
		writeError(w, e)
		return
	}
	httpx.WriteJSON(w, 201, p)
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in domain.Input
	if !decode(w, r, &in) {
		return
	}
	p, e := h.svc.Update(r.Context(), id, in)
	if e != nil {
		writeError(w, e)
		return
	}
	httpx.WriteJSON(w, 200, p)
}
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if e := h.svc.Delete(r.Context(), id); e != nil {
		writeError(w, e)
		return
	}
	w.WriteHeader(204)
}
