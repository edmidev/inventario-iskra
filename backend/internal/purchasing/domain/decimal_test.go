package domain_test

import (
	qtyval "github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
	"testing"
)

func TestFractionalConsumableAndWholeAssetRules(t *testing.T) {
	q, err := qtyval.Parse("0.125")
	if err != nil {
		t.Fatal(err)
	}
	c := consumableLine(1)
	c.Quantity = q
	c.UnitCostMinor = 100
	o := draft(t, c)
	if o.Total() != 13 {
		t.Fatalf("fractional line rounds half-up: %d", o.Total())
	}
	a := assetLine(1)
	a.Quantity = q
	if _, err := (domain.Input{SupplierID: 1, Currency: domain.USD, Lines: []domain.LineInput{a}}).Normalize(); err == nil {
		t.Fatal("fractional asset order accepted")
	}
	asset := draft(t, assetLine(1))
	if err := asset.Place(now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := asset.PlanReceipt(domain.ReceiveRequest{LocationID: 1, Lines: []domain.ReceiveLine{{LineID: 100, Quantity: q}}}); err == nil {
		t.Fatal("fractional asset receipt accepted")
	}
}
