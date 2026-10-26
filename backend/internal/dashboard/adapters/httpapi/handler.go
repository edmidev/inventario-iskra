// Package httpapi exposes the dashboard summary over HTTP under /api/v1.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/app"
	"github.com/edgar/inventario/backend/internal/dashboard/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

// Permissions names the permissions the routes require. They are injected so
// this module does not depend on the identity module.
type Permissions struct {
	Read string // every role holds it
}

// Handler serves the dashboard summary.
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
	r.Group(func(r chi.Router) {
		r.Use(h.authenticate)
		r.With(read).Get("/dashboard/summary", h.summary)
		r.With(read).Get("/dashboard/reorder-suggestions", h.reorderSuggestions)
	})
}

type lowStockItemDTO struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Description  string `json:"description"`
	Unit         string `json:"unit"`
	StockCurrent int    `json:"stock_current"`
	StockMin     int    `json:"stock_min"`
	// PendingOrdered is the quantity still to arrive on open purchase orders.
	PendingOrdered int     `json:"pending_ordered"`
	LocationName   *string `json:"location_name"`
	// Stocks is the per-location breakdown behind StockCurrent.
	Stocks []stockLocationDTO `json:"stocks"`
}

type stockLocationDTO struct {
	LocationID   int64  `json:"location_id"`
	LocationName string `json:"location_name"`
	Quantity     int    `json:"quantity"`
}

type overdueLoanDTO struct {
	ItemID       string    `json:"item_id"`
	Code         string    `json:"code"`
	Description  string    `json:"description"`
	AssigneeName string    `json:"assignee_name"`
	DueAt        time.Time `json:"due_at"`
}

type overdueMaintenanceDTO struct {
	ItemID           string    `json:"item_id"`
	Code             string    `json:"code"`
	Description      string    `json:"description"`
	MaintenanceDueAt time.Time `json:"maintenance_due_at"`
}

type summaryDTO struct {
	GeneratedAt time.Time `json:"generated_at"`
	Items       struct {
		Total       int `json:"total"`
		Assets      int `json:"assets"`
		Consumables int `json:"consumables"`
	} `json:"items"`
	Assets struct {
		Available   int `json:"available"`
		Unavailable int `json:"unavailable"`
		OnLoan      int `json:"on_loan"`
		Inoperative int `json:"inoperative"`
		InRepair    int `json:"in_repair"`
	} `json:"assets"`
	LowStock struct {
		Count int               `json:"count"`
		Top   []lowStockItemDTO `json:"top"`
	} `json:"low_stock"`
	Loans struct {
		Active     int              `json:"active"`
		Overdue    int              `json:"overdue"`
		OverdueTop []overdueLoanDTO `json:"overdue_top"`
	} `json:"loans"`
	Maintenance struct {
		OpenFailures int                     `json:"open_failures"`
		Overdue      int                     `json:"overdue"`
		OverdueTop   []overdueMaintenanceDTO `json:"overdue_top"`
	} `json:"maintenance"`
}

func toDTO(s domain.Summary) summaryDTO {
	var d summaryDTO
	d.GeneratedAt = s.GeneratedAt
	d.Items.Total, d.Items.Assets, d.Items.Consumables = s.Items.Total, s.Items.Assets, s.Items.Consumables
	d.Assets.Available, d.Assets.Unavailable, d.Assets.OnLoan = s.Assets.Available, s.Assets.Unavailable, s.Assets.OnLoan
	d.Assets.Inoperative, d.Assets.InRepair = s.Assets.Inoperative, s.Assets.InRepair
	d.LowStock.Count = s.LowStock.Count
	d.LowStock.Top = make([]lowStockItemDTO, len(s.LowStock.Top)) // never null in JSON
	for i, it := range s.LowStock.Top {
		stocks := make([]stockLocationDTO, len(it.Stocks)) // never null in JSON
		for j, st := range it.Stocks {
			stocks[j] = stockLocationDTO{st.LocationID, st.LocationName, st.Quantity}
		}
		d.LowStock.Top[i] = lowStockItemDTO{it.ID, it.Code, it.Description, it.Unit, it.StockCurrent, it.StockMin, it.PendingOrdered, it.LocationName, stocks}
	}
	d.Loans.Active, d.Loans.Overdue = s.Loans.Active, s.Loans.Overdue
	d.Loans.OverdueTop = make([]overdueLoanDTO, len(s.Loans.OverdueTop))
	for i, o := range s.Loans.OverdueTop {
		d.Loans.OverdueTop[i] = overdueLoanDTO{o.ItemID, o.Code, o.Description, o.AssigneeName, o.DueAt}
	}
	d.Maintenance.OpenFailures, d.Maintenance.Overdue = s.Maintenance.OpenFailures, s.Maintenance.Overdue
	d.Maintenance.OverdueTop = make([]overdueMaintenanceDTO, len(s.Maintenance.OverdueTop))
	for i, o := range s.Maintenance.OverdueTop {
		d.Maintenance.OverdueTop[i] = overdueMaintenanceDTO{o.ItemID, o.Code, o.Description, o.MaintenanceDueAt}
	}
	return d
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Summary(r.Context())
	if err != nil {
		slog.Error("dashboard summary", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(s))
}

type reorderLineDTO struct {
	ItemID            string  `json:"item_id"`
	Code              string  `json:"code"`
	Description       string  `json:"description"`
	Unit              string  `json:"unit"`
	StockCurrent      int     `json:"stock_current"`
	StockMin          int     `json:"stock_min"`
	PendingOrdered    int     `json:"pending_ordered"`
	SuggestedQty      int     `json:"suggested_qty"`
	LastUnitCostMinor *int64  `json:"last_unit_cost_minor"`
	LastCurrency      *string `json:"last_currency"`
}

type reorderGroupDTO struct {
	SupplierID   *int64           `json:"supplier_id"`
	SupplierName *string          `json:"supplier_name"`
	Lines        []reorderLineDTO `json:"lines"`
}

func (h *Handler) reorderSuggestions(w http.ResponseWriter, r *http.Request) {
	groups, err := h.svc.ReorderSuggestions(r.Context())
	if err != nil {
		slog.Error("dashboard reorder suggestions", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
		return
	}
	out := make([]reorderGroupDTO, len(groups)) // never null in JSON
	for i, g := range groups {
		lines := make([]reorderLineDTO, len(g.Lines))
		for j, l := range g.Lines {
			lines[j] = reorderLineDTO{l.ItemID, l.Code, l.Description, l.Unit, l.StockCurrent, l.StockMin,
				l.PendingOrdered, l.SuggestedQty, l.LastUnitCostMinor, l.LastCurrency}
		}
		out[i] = reorderGroupDTO{g.SupplierID, g.SupplierName, lines}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"groups": out})
}
