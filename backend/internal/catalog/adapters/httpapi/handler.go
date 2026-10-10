// Package httpapi exposes the catalog use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/edgar/inventario/backend/internal/catalog/app"
	"github.com/edgar/inventario/backend/internal/catalog/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

const maxBodyBytes = 1 << 20

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read           string // list and read
	WriteItems     string // create, update, delete items
	ManageCatalogs string // categories, locations, and supplying item codes
}

// Handler serves categories, locations and items.
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
	writeItems := httpx.RequirePermission(h.perms.WriteItems)
	admin := httpx.RequirePermission(h.perms.ManageCatalogs)

	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.Route("/categories", func(r chi.Router) {
			r.With(read).Get("/", h.listCategories)
			r.With(admin).Post("/", h.createCategory)
			r.With(admin).Patch("/{id}", h.updateCategory)
			r.With(admin).Delete("/{id}", h.deleteCategory)
		})
		r.Route("/locations", func(r chi.Router) {
			r.With(read).Get("/", h.listLocations)
			r.With(admin).Post("/", h.createLocation)
			r.With(admin).Patch("/{id}", h.updateLocation)
			r.With(admin).Delete("/{id}", h.deleteLocation)
		})
		r.Route("/items", func(r chi.Router) {
			r.With(read).Get("/", h.listItems)
			r.With(read).Get("/by-code/{code}", h.getItemByCode)
			r.With(read).Get("/{id}", h.getItem)
			r.With(writeItems).Post("/", h.createItem)
			r.With(writeItems).Patch("/{id}", h.updateItem)
			r.With(writeItems).Delete("/{id}", h.deleteItem)
		})
	})
}

// ---- helpers ----

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

func pathID(w http.ResponseWriter, r *http.Request, what string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, what+" not found")
		return 0, false
	}
	return id, true
}

func writeError(w http.ResponseWriter, err error) {
	var ve *domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteFieldErrors(w, ve.Fields)
	case errors.Is(err, domain.ErrCategoryNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "category not found")
	case errors.Is(err, domain.ErrLocationNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "location not found")
	case errors.Is(err, domain.ErrItemNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "item not found")
	case errors.Is(err, domain.ErrCategoryNameTaken):
		httpx.WriteError(w, http.StatusConflict, "category_name_taken", "a category with this name already exists")
	case errors.Is(err, domain.ErrCategoryPrefixTaken):
		httpx.WriteError(w, http.StatusConflict, "category_prefix_taken", "a category with this code prefix already exists")
	case errors.Is(err, domain.ErrCategoryInUse):
		httpx.WriteError(w, http.StatusConflict, "category_in_use", "category still has items")
	case errors.Is(err, domain.ErrLocationInUse):
		httpx.WriteError(w, http.StatusConflict, "location_in_use", "location still has child locations or items")
	case errors.Is(err, domain.ErrLocationCycle):
		httpx.WriteError(w, http.StatusConflict, "location_cycle", "the new parent would create a cycle")
	case errors.Is(err, domain.ErrItemOnLoan):
		httpx.WriteError(w, http.StatusConflict, "item_on_loan", "item is on loan; return it first")
	case errors.Is(err, domain.ErrCodeTaken):
		httpx.WriteError(w, http.StatusConflict, "code_taken", "an item with this code already exists")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}

// ---- categories ----

type categoryDTO struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	CodePrefix string `json:"code_prefix"`
}

func toCategoryDTO(c domain.Category) categoryDTO {
	return categoryDTO{ID: c.ID, Name: c.Name, CodePrefix: c.CodePrefix}
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.svc.ListCategories(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]categoryDTO, len(cats))
	for i, c := range cats {
		out[i] = toCategoryDTO(c)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		CodePrefix string `json:"code_prefix"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), in.Name, in.CodePrefix)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toCategoryDTO(c))
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "category")
	if !ok {
		return
	}
	var in struct {
		Name       *string `json:"name"`
		CodePrefix *string `json:"code_prefix"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateCategory(r.Context(), id, in.Name, in.CodePrefix)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toCategoryDTO(c))
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "category")
	if !ok {
		return
	}
	if err := h.svc.DeleteCategory(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- locations ----

type locationDTO struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	ParentID *int64 `json:"parent_id"`
}

func toLocationDTO(l domain.Location) locationDTO {
	return locationDTO{ID: l.ID, Name: l.Name, Kind: string(l.Kind), ParentID: l.ParentID}
}

func (h *Handler) listLocations(w http.ResponseWriter, r *http.Request) {
	locs, err := h.svc.ListLocations(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]locationDTO, len(locs))
	for i, l := range locs {
		out[i] = toLocationDTO(l)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) createLocation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		ParentID *int64 `json:"parent_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	l, err := h.svc.CreateLocation(r.Context(), in.Name, domain.LocationKind(in.Kind), in.ParentID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toLocationDTO(l))
}

func (h *Handler) updateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "location")
	if !ok {
		return
	}
	var in struct {
		Name     *string                `json:"name"`
		Kind     *string                `json:"kind"`
		ParentID domain.Nullable[int64] `json:"parent_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := app.LocationPatch{Name: in.Name, ParentID: in.ParentID}
	if in.Kind != nil {
		k := domain.LocationKind(*in.Kind)
		p.Kind = &k
	}
	l, err := h.svc.UpdateLocation(r.Context(), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toLocationDTO(l))
}

func (h *Handler) deleteLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "location")
	if !ok {
		return
	}
	if err := h.svc.DeleteLocation(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- items ----

type itemDTO struct {
	ID                      string         `json:"id"`
	Code                    string         `json:"code"`
	Description             string         `json:"description"`
	CategoryID              int64          `json:"category_id"`
	Kind                    string         `json:"kind"`
	Status                  string         `json:"status"`
	Availability            string         `json:"availability"`
	Unit                    *string        `json:"unit"`
	StockCurrent            int            `json:"stock_current"`
	StockMin                int            `json:"stock_min"`
	LowStock                bool           `json:"low_stock"`
	OnLoan                  bool           `json:"on_loan"`
	Custody                 *custodyDTO    `json:"custody,omitempty"`
	LocationID              *int64         `json:"location_id"`
	LastMaintenanceAt       *time.Time     `json:"last_maintenance_at"`
	MaintenanceIntervalDays *int           `json:"maintenance_interval_days"`
	MaintenanceDueAt        *time.Time     `json:"maintenance_due_at"`
	MaintenanceOverdue      bool           `json:"maintenance_overdue"`
	Attributes              map[string]any `json:"attributes"`
	Notes                   string         `json:"notes"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

// custodyDTO is the active loan embedded in an item response.
type custodyDTO struct {
	AssigneeLocationID int64      `json:"assignee_location_id"`
	AssigneeName       string     `json:"assignee_name"`
	AssignedAt         time.Time  `json:"assigned_at"`
	DueAt              *time.Time `json:"due_at"`
	Overdue            bool       `json:"overdue"`
}

func toItemDTO(it domain.Item) itemDTO {
	var loan *custodyDTO
	if l := it.Loan; l != nil {
		loan = &custodyDTO{AssigneeLocationID: l.AssigneeLocationID, AssigneeName: l.AssigneeName,
			AssignedAt: l.AssignedAt, DueAt: l.DueAt, Overdue: l.Overdue(time.Now())}
	}
	attrs := it.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}
	return itemDTO{
		ID: it.ID, Code: it.Code, Description: it.Description, CategoryID: it.CategoryID,
		Kind: string(it.Kind), Status: string(it.Status), Availability: string(it.Availability()),
		Unit: it.Unit, StockCurrent: it.StockCurrent, StockMin: it.StockMin, LowStock: it.LowStock(),
		OnLoan: it.OnLoan(), Custody: loan,
		LocationID: it.LocationID, LastMaintenanceAt: it.LastMaintenanceAt,
		MaintenanceIntervalDays: it.MaintenanceIntervalDays, MaintenanceDueAt: it.MaintenanceDueAt(),
		MaintenanceOverdue: it.MaintenanceOverdue(time.Now()), Attributes: attrs, Notes: it.Notes,
		CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt,
	}
}

func validItemID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

func itemID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !validItemID(id) {
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "item not found")
		return "", false
	}
	return id, true
}

func parseFilter(r *http.Request) (domain.ItemFilter, map[string]string) {
	q := r.URL.Query()
	fields := map[string]string{}
	f := domain.ItemFilter{
		Q: q.Get("q"), Status: domain.Status(q.Get("status")), Kind: domain.Kind(q.Get("kind")),
		Availability: domain.Availability(q.Get("availability")),
	}
	intParam := func(name string, dst *int) {
		if v := q.Get(name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				fields[name] = "must be an integer"
				return
			}
			*dst = n
		}
	}
	idParam := func(name string) *int64 {
		v := q.Get(name)
		if v == "" {
			return nil
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fields[name] = "must be an integer"
			return nil
		}
		return &n
	}
	f.CategoryID = idParam("category_id")
	f.LocationID = idParam("location_id")
	intParam("limit", &f.Limit)
	intParam("offset", &f.Offset)
	switch v := q.Get("low_stock"); v {
	case "", "false":
	case "true":
		f.LowStock = true
	default:
		fields["low_stock"] = "must be true or false"
	}
	f.Sort = q.Get("sort")
	if strings.HasPrefix(f.Sort, "-") {
		f.Desc, f.Sort = true, f.Sort[1:]
	}
	switch q.Get("order") {
	case "", "asc":
	case "desc":
		f.Desc = true
	default:
		fields["order"] = "must be asc or desc"
	}
	return f, fields
}

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	f, fields := parseFilter(r)
	if len(fields) > 0 {
		httpx.WriteFieldErrors(w, fields)
		return
	}
	items, total, nf, err := h.svc.ListItems(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]itemDTO, len(items))
	for i, it := range items {
		out[i] = toItemDTO(it)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": out, "total": total, "limit": nf.Limit, "offset": nf.Offset,
	})
}

func (h *Handler) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(w, r)
	if !ok {
		return
	}
	it, err := h.svc.GetItem(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toItemDTO(it))
}

func (h *Handler) getItemByCode(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "code")))
	it, err := h.svc.GetItemByCode(r.Context(), code)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toItemDTO(it))
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code                    string         `json:"code"`
		Description             string         `json:"description"`
		CategoryID              int64          `json:"category_id"`
		Kind                    string         `json:"kind"`
		Status                  string         `json:"status"`
		Unit                    *string        `json:"unit"`
		StockCurrent            *int           `json:"stock_current"`
		StockMin                int            `json:"stock_min"`
		LocationID              *int64         `json:"location_id"`
		LastMaintenanceAt       *time.Time     `json:"last_maintenance_at"`
		MaintenanceIntervalDays *int           `json:"maintenance_interval_days"`
		Attributes              map[string]any `json:"attributes"`
		Notes                   string         `json:"notes"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Code != "" {
		// Only catalog administrators (and the importer) may pick codes.
		if p, _ := httpx.PrincipalFrom(r.Context()); !p.Has(h.perms.ManageCatalogs) {
			httpx.WriteError(w, http.StatusForbidden, httpx.CodeForbidden, "only administrators can set an item code")
			return
		}
	}
	it, err := h.svc.CreateItem(r.Context(), app.CreateItemInput{
		Code: in.Code,
		NewItemInput: domain.NewItemInput{
			Description: in.Description, CategoryID: in.CategoryID, Kind: domain.Kind(in.Kind),
			Status: domain.Status(in.Status), Unit: in.Unit, StockCurrent: in.StockCurrent,
			StockMin: in.StockMin, LocationID: in.LocationID, LastMaintenanceAt: in.LastMaintenanceAt,
			MaintenanceIntervalDays: in.MaintenanceIntervalDays, Attributes: in.Attributes, Notes: in.Notes,
		},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toItemDTO(it))
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(w, r)
	if !ok {
		return
	}
	var in struct {
		Description             *string                `json:"description"`
		CategoryID              *int64                 `json:"category_id"`
		Status                  *string                `json:"status"`
		Unit                    *string                `json:"unit"`
		StockMin                *int                   `json:"stock_min"`
		LocationID              domain.Nullable[int64] `json:"location_id"`
		MaintenanceIntervalDays domain.Nullable[int]   `json:"maintenance_interval_days"`
		Attributes              map[string]any         `json:"attributes"`
		Notes                   *string                `json:"notes"`
		// Accepted only to reject changes with a precise message.
		Code         *string `json:"code"`
		Kind         *string `json:"kind"`
		StockCurrent *int    `json:"stock_current"`
	}
	if !decode(w, r, &in) {
		return
	}
	p := domain.ItemPatch{
		Description: in.Description, CategoryID: in.CategoryID, Unit: in.Unit, StockMin: in.StockMin,
		LocationID: in.LocationID, MaintenanceIntervalDays: in.MaintenanceIntervalDays,
		Attributes: in.Attributes, Notes: in.Notes, Code: in.Code, StockCurrent: in.StockCurrent,
	}
	if in.Status != nil {
		s := domain.Status(*in.Status)
		p.Status = &s
	}
	if in.Kind != nil {
		k := domain.Kind(*in.Kind)
		p.Kind = &k
	}
	it, err := h.svc.UpdateItem(r.Context(), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toItemDTO(it))
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := itemID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DeleteItem(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
