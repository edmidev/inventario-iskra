// Package httpapi exposes the supplier use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/edgar/inventario/backend/internal/suppliers/app"
	"github.com/edgar/inventario/backend/internal/suppliers/domain"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read  string // list and read
	Write string // create, update, delete
}

// Handler serves suppliers.
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
		r.Route("/suppliers", func(r chi.Router) {
			r.With(read).Get("/", h.list)
			r.With(read).Get("/{id}", h.get)
			r.With(write).Post("/", h.create)
			r.With(write).Patch("/{id}", h.update)
			r.With(write).Delete("/{id}", h.delete)
		})
	})
}

type supplierDTO struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	ContactName string    `json:"contact_name"`
	Email       string    `json:"email"`
	Phone       string    `json:"phone"`
	Notes       string    `json:"notes"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func toDTO(s domain.Supplier) supplierDTO {
	return supplierDTO{
		ID: s.ID, Name: s.Name, ContactName: s.ContactName, Email: s.Email, Phone: s.Phone,
		Notes: s.Notes, Active: s.Active, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
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
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "supplier not found")
		return 0, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteFieldErrors(w, ve.Fields)
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "supplier not found")
	case errors.Is(err, domain.ErrNameTaken):
		httpx.WriteError(w, http.StatusConflict, "supplier_name_taken", "a supplier with this name already exists")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := domain.Filter{Q: q.Get("q")}
	fields := map[string]string{}
	if v := q.Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			fields["active"] = "must be true or false"
		}
		f.Active = &b
	}
	for name, dst := range map[string]*int{"limit": &f.Limit, "offset": &f.Offset} {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				fields[name] = "must be an integer"
			}
			*dst = n
		}
	}
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	sups, total, nf, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]supplierDTO, len(sups))
	for i, s := range sups {
		out[i] = toDTO(s)
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
	s, err := h.svc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(s))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		ContactName string `json:"contact_name"`
		Email       string `json:"email"`
		Phone       string `json:"phone"`
		Notes       string `json:"notes"`
		Active      *bool  `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	s, err := h.svc.Create(r.Context(), domain.Input{
		Name: in.Name, ContactName: in.ContactName, Email: in.Email,
		Phone: in.Phone, Notes: in.Notes, Active: active,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(s))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name        *string `json:"name"`
		ContactName *string `json:"contact_name"`
		Email       *string `json:"email"`
		Phone       *string `json:"phone"`
		Notes       *string `json:"notes"`
		Active      *bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	s, err := h.svc.Update(r.Context(), id, app.Patch{
		Name: in.Name, ContactName: in.ContactName, Email: in.Email,
		Phone: in.Phone, Notes: in.Notes, Active: in.Active,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(s))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
