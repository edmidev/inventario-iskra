package postgres_test

import (
	qtyval "github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/internal/purchasing/domain"
	"testing"
)

func TestFractionalPurchaseReceiptIsExactAndIdempotent(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	q, err := qtyval.Parse("0.125")
	if err != nil {
		t.Fatal(err)
	}
	line := f.paper(1)
	line.Quantity = q
	line.UnitCostMinor = 100
	o := f.placed(t, line)
	if o.Total() != 13 {
		t.Fatalf("half-up total %d", o.Total())
	}
	req := domain.ReceiveRequest{LocationID: f.loc, ClientKey: "decimal-receipt", Lines: []domain.ReceiveLine{{LineID: o.Lines[0].ID, Quantity: q}}}
	first, err := f.svc.Receive(ctx, o.ID, f.userID, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Order.Status != domain.StatusReceived || first.Order.Lines[0].ReceivedQty != q {
		t.Fatalf("receipt %+v", first)
	}
	second, err := f.svc.Receive(ctx, o.ID, f.userID, req)
	if err != nil || !second.Replayed {
		t.Fatalf("replay %+v %v", second, err)
	}
	var sum qtyval.Quantity
	if err = f.pool.QueryRow(ctx, `SELECT sum(qty) FROM stock_movements WHERE item_id=$1 AND type='in'`, f.consID).Scan(&sum); err != nil || sum != q {
		t.Fatalf("ledger %s %v", sum, err)
	}
}
