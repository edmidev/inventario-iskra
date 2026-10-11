package importer

// CreatedItem records one item written by Apply.
type CreatedItem struct {
	Sheet     string `json:"sheet"`
	Row       int    `json:"row"`
	Code      string `json:"code"`
	Generated bool   `json:"generated"`
}

// Result is what Apply did.
type Result struct {
	CategoriesCreated  int           `json:"categories_created"`
	LocationsCreated   int           `json:"locations_created"`
	ItemsCreated       int           `json:"items_created"`
	AssetsCreated      int           `json:"assets_created"`
	ConsumablesCreated int           `json:"consumables_created"`
	OpeningMovements   int           `json:"opening_movements"`
	MaintenanceOpened  int           `json:"maintenance_opened"`
	AlreadyImported    int           `json:"already_imported"`
	Conflicts          int           `json:"conflicts"`
	Created            []CreatedItem `json:"created,omitempty"`
	Errors             []string      `json:"errors,omitempty"`
}
