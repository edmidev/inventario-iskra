package httpapi

import (
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.ListRoles(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, 200, map[string]any{"data": roles})
}
func (h *Handler) permissionCatalog(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, 200, map[string]any{"data": domain.Permissions()})
}
func (h *Handler) setRolePermissions(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Permissions []domain.Permission `json:"permissions"`
	}
	if !decode(w, r, &in) {
		return
	}
	role, err := h.svc.SetRolePermissions(r.Context(), domain.Role(chi.URLParam(r, "role")), in.Permissions)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, role)
}
