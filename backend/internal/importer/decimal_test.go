package importer

import "testing"

func TestFractionalConsumableCell(t *testing.T) {
	q, present, err := parseQuantity("0,125")
	if err != nil || !present || q.String() != "0.125" {
		t.Fatalf("cell %s %v %v", q, present, err)
	}
	if _, _, err = parseQuantity("0.0001"); err == nil {
		t.Fatal("fourth decimal accepted")
	}
}
