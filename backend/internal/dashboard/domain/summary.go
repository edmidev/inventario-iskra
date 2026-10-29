// Package domain holds the read model behind the home dashboard. It is a pure
// projection: it owns no data and enforces no business rules.
package domain

import "time"

// TopLimit is the maximum number of rows in each "top" list.
const TopLimit = 5

// Summary is the home-screen snapshot of the whole inventory.
type Summary struct {
	GeneratedAt time.Time
	Items       ItemCounts
	Assets      AssetCounts
	LowStock    LowStock
	Loans       Loans
	Maintenance Maintenance
}

// ItemCounts counts non-deleted items by kind.
type ItemCounts struct {
	Total       int
	Assets      int
	Consumables int
}

// AssetCounts counts non-deleted assets. Available and Unavailable partition
// the assets (available = functional and not on loan); the rest overlap them.
type AssetCounts struct {
	Available   int
	Unavailable int
	OnLoan      int
	Inoperative int
	InRepair    int
}

// StockLocation is the quantity of a consumable held at one location.
type StockLocation struct {
	LocationID   int64
	LocationName string
	Quantity     int
}

// LowStockItem is a consumable at or below its configured minimum. The
// comparison is on the total; Stocks is the per-location breakdown and
// LocationName is set only when all the stock sits in one location.
type LowStockItem struct {
	ID           string
	Code         string
	Description  string
	Unit         string
	StockCurrent int
	StockMin     int
	// PendingOrdered is the quantity still to arrive on open purchase orders.
	PendingOrdered int
	LocationName   *string
	Stocks         []StockLocation
}

// LowStock counts low-stock consumables and lists the most severe.
type LowStock struct {
	Count int
	Top   []LowStockItem
}

// OverdueLoan is an active loan past its due date.
type OverdueLoan struct {
	ItemID       string
	Code         string
	Description  string
	AssigneeName string
	DueAt        time.Time
}

// Loans counts active and overdue loans and lists the oldest overdue.
type Loans struct {
	Active     int
	Overdue    int
	OverdueTop []OverdueLoan
}

// OverdueMaintenance is an asset whose maintenance date has passed.
type OverdueMaintenance struct {
	ItemID           string
	Code             string
	Description      string
	MaintenanceDueAt time.Time
}

// Maintenance counts open failures and overdue maintenance.
type Maintenance struct {
	OpenFailures int
	Overdue      int
	OverdueTop   []OverdueMaintenance
}
