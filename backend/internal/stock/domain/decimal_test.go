package domain

import (
	"encoding/json"
	"testing"
)

func TestFractionalMovementInput(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(`{"Type":"in","Qty":0.125}`), &req); err != nil {
		t.Fatal(err)
	}
	out, err := Decide(ItemState{Kind: KindConsumable, Locations: map[int64]string{1: "Store"}}, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out.StockCurrent)
	if err != nil || string(b) != "0.125" {
		t.Fatalf("stock %s err %v", b, err)
	}
}
