package main

import (
	"context"
	"errors"
	"time"

	catalogapi "github.com/edgar/inventario/backend/internal/catalog/adapters/httpapi"
	catalogpg "github.com/edgar/inventario/backend/internal/catalog/adapters/postgres"
	catalogapp "github.com/edgar/inventario/backend/internal/catalog/app"
	catalogdomain "github.com/edgar/inventario/backend/internal/catalog/domain"
	custodyapi "github.com/edgar/inventario/backend/internal/custody/adapters/httpapi"
	custodypg "github.com/edgar/inventario/backend/internal/custody/adapters/postgres"
	custodyapp "github.com/edgar/inventario/backend/internal/custody/app"
	dashboardapi "github.com/edgar/inventario/backend/internal/dashboard/adapters/httpapi"
	dashboardpg "github.com/edgar/inventario/backend/internal/dashboard/adapters/postgres"
	dashboardapp "github.com/edgar/inventario/backend/internal/dashboard/app"
	"github.com/edgar/inventario/backend/internal/identity/adapters"
	"github.com/edgar/inventario/backend/internal/identity/adapters/httpapi"
	"github.com/edgar/inventario/backend/internal/identity/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	maintenanceapi "github.com/edgar/inventario/backend/internal/maintenance/adapters/httpapi"
	maintenancepg "github.com/edgar/inventario/backend/internal/maintenance/adapters/postgres"
	maintenanceapp "github.com/edgar/inventario/backend/internal/maintenance/app"
	"github.com/edgar/inventario/backend/internal/platform/config"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	purchasingapi "github.com/edgar/inventario/backend/internal/purchasing/adapters/httpapi"
	purchasingpg "github.com/edgar/inventario/backend/internal/purchasing/adapters/postgres"
	purchasingapp "github.com/edgar/inventario/backend/internal/purchasing/app"
	purchasingdomain "github.com/edgar/inventario/backend/internal/purchasing/domain"
	stockapi "github.com/edgar/inventario/backend/internal/stock/adapters/httpapi"
	stockpg "github.com/edgar/inventario/backend/internal/stock/adapters/postgres"
	stockapp "github.com/edgar/inventario/backend/internal/stock/app"
	stockdomain "github.com/edgar/inventario/backend/internal/stock/domain"
	suppliersapi "github.com/edgar/inventario/backend/internal/suppliers/adapters/httpapi"
	supplierspg "github.com/edgar/inventario/backend/internal/suppliers/adapters/postgres"
	suppliersapp "github.com/edgar/inventario/backend/internal/suppliers/app"
	suppliersdomain "github.com/edgar/inventario/backend/internal/suppliers/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Login throttling: 5 failures per client+email within 15 minutes.
const (
	loginMaxFails = 5
	loginWindow   = 15 * time.Minute
)

func wireIdentity(pool *pgxpool.Pool, cfg config.Config) (*httpapi.Handler, *app.Service, error) {
	issuer, err := adapters.NewJWTIssuer(cfg.JWTSecret, cfg.AccessTTL)
	if err != nil {
		return nil, nil, err
	}
	repo := postgres.New(pool)
	svc := app.NewService(app.Deps{
		Users:         repo,
		Tokens:        repo.Tokens(),
		Hasher:        adapters.DefaultArgon2idHasher(),
		Issuer:        issuer,
		Limiter:       app.NewMemoryLoginLimiter(loginMaxFails, loginWindow, nil),
		RefreshTTL:    cfg.RefreshTTL,
		RotationGrace: cfg.RefreshRotationGrace,
	})
	return httpapi.New(svc, cfg.CookieSecure), svc, nil
}

// openingBalanceHook is the only place catalog creation and stock meet: a
// consumable created with stock gets its opening movement inside the same
// transaction as the item. The acting user comes from the request principal.
func openingBalanceHook(ctx context.Context, tx pgx.Tx, it catalogdomain.Item) error {
	if it.Kind != catalogdomain.KindConsumable || len(it.Stocks) == 0 {
		return nil
	}
	userID := ""
	if p, ok := httpx.PrincipalFrom(ctx); ok {
		userID = p.UserID
	}
	stocks := make([]stockdomain.LocationStock, len(it.Stocks))
	for i, st := range it.Stocks {
		stocks[i] = stockdomain.LocationStock{LocationID: st.LocationID, LocationName: st.LocationName, Quantity: st.Quantity}
	}
	return stockpg.RecordOpenings(ctx, tx, it.ID, stocks, userID)
}

// newCatalogService builds the catalog use cases with the opening-balance hook.
func newCatalogService(pool *pgxpool.Pool) *catalogapp.Service {
	repo := catalogpg.New(pool, catalogpg.WithCreateHook(openingBalanceHook))
	return catalogapp.NewService(repo.Categories(), repo.Locations(), repo.Items())
}

// wireCatalog builds the catalog handler, reusing identity's bearer-token
// middleware and permission names.
func wireCatalog(pool *pgxpool.Pool, identity *httpapi.Handler) *catalogapi.Handler {
	svc := newCatalogService(pool)
	return catalogapi.New(svc, identity.Authenticate(), catalogapi.Permissions{
		Read:           string(domain.PermRead),
		WriteItems:     string(domain.PermWriteItems),
		ManageCatalogs: string(domain.PermManageCatalogs),
	})
}

// wireStock builds the stock handler with the same authentication and
// permission names as the catalog.
func wireStock(pool *pgxpool.Pool, identity *httpapi.Handler) *stockapi.Handler {
	svc := stockapp.NewService(stockpg.New(pool))
	return stockapi.New(svc, identity.Authenticate(), stockapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}

// wireCustody builds the custody handler with the same authentication and
// permission names as the catalog.
func wireCustody(pool *pgxpool.Pool, identity *httpapi.Handler) *custodyapi.Handler {
	svc := custodyapp.NewService(custodypg.New(pool), nil)
	return custodyapi.New(svc, identity.Authenticate(), custodyapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}

// wireMaintenance builds the maintenance handler with the same authentication
// and permission names as the catalog.
func wireMaintenance(pool *pgxpool.Pool, identity *httpapi.Handler) *maintenanceapi.Handler {
	svc := maintenanceapp.NewService(maintenancepg.New(pool), nil)
	return maintenanceapi.New(svc, identity.Authenticate(), maintenanceapi.Permissions{
		Read:       string(domain.PermRead),
		WriteItems: string(domain.PermWriteItems),
	})
}

// wireSuppliers builds the supplier handler; operators and admins write, every
// role reads.
func wireSuppliers(pool *pgxpool.Pool, identity *httpapi.Handler) *suppliersapi.Handler {
	svc := suppliersapp.NewService(supplierspg.New(pool))
	return suppliersapi.New(svc, identity.Authenticate(), suppliersapi.Permissions{
		Read:  string(domain.PermRead),
		Write: string(domain.PermWriteItems),
	})
}

// wireDashboard builds the read-only dashboard handler; every role may read it.
func wireDashboard(pool *pgxpool.Pool, identity *httpapi.Handler) *dashboardapi.Handler {
	svc := dashboardapp.NewService(dashboardpg.New(pool), nil)
	return dashboardapi.New(svc, identity.Authenticate(), dashboardapi.Permissions{
		Read: string(domain.PermRead),
	})
}

// wirePurchasing builds the purchase order handler; operators and admins write,
// every role reads.
func wirePurchasing(pool *pgxpool.Pool, identity *httpapi.Handler) *purchasingapi.Handler {
	return purchasingapi.New(newPurchasingService(pool), identity.Authenticate(), purchasingapi.Permissions{
		Read:  string(domain.PermRead),
		Write: string(domain.PermWriteItems),
	})
}

// newPurchasingService is the only place purchasing meets the catalog, stock
// and suppliers modules: it implements purchasing's ports on top of their
// services. Receiving runs in one transaction because the postgres repository
// hands its transaction to these ports through the context (see
// internal/platform/db), and catalog item creation and stock recording join it.
func newPurchasingService(pool *pgxpool.Pool) *purchasingapp.Service {
	return purchasingapp.NewService(
		purchasingpg.New(pool),
		purchasingCatalog{svc: newCatalogService(pool)},
		purchasingStock{svc: stockapp.NewService(stockpg.New(pool))},
		purchasingSuppliers{svc: suppliersapp.NewService(supplierspg.New(pool))},
		nil,
	)
}

// purchasingCatalog adapts the catalog service to purchasing's ItemCatalog.
type purchasingCatalog struct{ svc *catalogapp.Service }

func newItemInput(spec purchasingapp.NewItemSpec) catalogdomain.NewItemInput {
	in := catalogdomain.NewItemInput{
		Description: spec.Description, CategoryID: spec.CategoryID, Kind: catalogdomain.Kind(spec.Kind),
		StockMin: spec.StockMin, Attributes: spec.Attributes, Notes: spec.Notes,
	}
	if spec.Unit != "" {
		u := spec.Unit
		in.Unit = &u
	}
	return in
}

// asPurchasingValidation turns a catalog validation error into purchasing's,
// prefixing each field; any other error is returned unchanged.
func asPurchasingValidation(err error, prefix string) error {
	var ve *catalogdomain.ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	fields := make(map[string]string, len(ve.Fields))
	for k, v := range ve.Fields {
		fields[prefix+k] = v
	}
	return &purchasingdomain.ValidationError{Fields: fields}
}

func (c purchasingCatalog) ValidateNewItem(ctx context.Context, spec purchasingapp.NewItemSpec) error {
	if _, err := catalogdomain.NewItem(newItemInput(spec)); err != nil {
		return asPurchasingValidation(err, "")
	}
	cats, err := c.svc.ListCategories(ctx)
	if err != nil {
		return err
	}
	for _, cat := range cats {
		if cat.ID == spec.CategoryID {
			return nil
		}
	}
	return purchasingdomain.FieldError("category_id", "category does not exist")
}

func (c purchasingCatalog) CreateItem(ctx context.Context, spec purchasingapp.NewItemSpec, locationID int64) (purchasingapp.CreatedItem, error) {
	in := newItemInput(spec)
	if spec.Kind == purchasingdomain.KindAsset {
		in.LocationID = &locationID
	}
	it, err := c.svc.CreateItem(ctx, catalogapp.CreateItemInput{NewItemInput: in})
	if err != nil {
		return purchasingapp.CreatedItem{}, asPurchasingValidation(err, "new_item.")
	}
	return purchasingapp.CreatedItem{ID: it.ID, Code: it.Code}, nil
}

func (c purchasingCatalog) ItemInfo(ctx context.Context, itemID string) (purchasingapp.ItemInfo, error) {
	it, err := c.svc.GetItem(ctx, itemID)
	if errors.Is(err, catalogdomain.ErrItemNotFound) {
		return purchasingapp.ItemInfo{}, purchasingapp.ErrItemNotFound
	}
	if err != nil {
		return purchasingapp.ItemInfo{}, err
	}
	return purchasingapp.ItemInfo{Kind: string(it.Kind), Description: it.Description}, nil
}

func (c purchasingCatalog) LocationExists(ctx context.Context, id int64) (bool, error) {
	locs, err := c.svc.ListLocations(ctx)
	if err != nil {
		return false, err
	}
	for _, l := range locs {
		if l.ID == id {
			return true, nil
		}
	}
	return false, nil
}

// purchasingStock adapts the stock service to purchasing's StockReceiver.
type purchasingStock struct{ svc *stockapp.Service }

func (s purchasingStock) ReceiveIn(ctx context.Context, itemID, userID string, locationID int64, qty int, note string) error {
	_, err := s.svc.Record(ctx, itemID, userID, stockdomain.Request{
		Type: stockdomain.TypeIn, Qty: qty, LocationID: &locationID, Note: note,
	})
	var ve *stockdomain.ValidationError
	if errors.As(err, &ve) {
		fields := make(map[string]string, len(ve.Fields))
		for k, v := range ve.Fields {
			fields["stock."+k] = v
		}
		return &purchasingdomain.ValidationError{Fields: fields}
	}
	return err
}

// purchasingSuppliers adapts the suppliers service to purchasing's Suppliers.
type purchasingSuppliers struct{ svc *suppliersapp.Service }

func (s purchasingSuppliers) Get(ctx context.Context, id int64) (string, bool, error) {
	sup, err := s.svc.Get(ctx, id)
	if errors.Is(err, suppliersdomain.ErrNotFound) {
		return "", false, purchasingapp.ErrSupplierNotFound
	}
	if err != nil {
		return "", false, err
	}
	return sup.Name, sup.Active, nil
}
