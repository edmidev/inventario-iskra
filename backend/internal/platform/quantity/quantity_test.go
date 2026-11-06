package quantity

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestParseAndJSON(t *testing.T) {
	for _, tt := range []struct{ in, want string }{{"1", "1"}, {"0.125", "0.125"}, {"2.500", "2.5"}, {"-0.001", "-0.001"}, {"1e-3", "0.001"}} {
		t.Run(tt.in, func(t *testing.T) {
			var q Quantity
			if err := json.Unmarshal([]byte(tt.in), &q); err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(q)
			if err != nil || string(got) != tt.want {
				t.Fatalf("%s %v", got, err)
			}
		})
	}
	for _, in := range []string{"0.0001", "1.0000", "null", "\"1\"", "9223372036854776", "1e1000000"} {
		t.Run(in, func(t *testing.T) {
			var q Quantity
			if json.Unmarshal([]byte(in), &q) == nil {
				t.Fatal("accepted invalid quantity")
			}
		})
	}
}
func TestLineCost(t *testing.T) {
	q, err := Parse("0.125")
	if err != nil {
		t.Fatal(err)
	}
	cost, err := q.Cost(100)
	if err != nil || cost != 13 {
		t.Fatalf("%d %v", cost, err)
	}
	q, _ = Parse("1")
	cost, err = q.Cost(250)
	if err != nil || cost != 250 {
		t.Fatalf("integer regression %d %v", cost, err)
	}
}

func TestBoundsAndArithmetic(t *testing.T) {
	for _, s := range []string{"9223372036854775.807", "-9223372036854775.808", "0", "1000000000"} {
		q, err := Parse(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if q.String() != s {
			t.Fatalf("got %s, want %s", q, s)
		}
	}
	max, _ := Parse("9223372036854775.807")
	if _, err := max.Add(1); err == nil {
		t.Fatal("overflow accepted")
	}
	if _, err := max.Multiply(2); err == nil {
		t.Fatal("multiply overflow accepted")
	}
	if _, err := max.Cost(9223372036854775807); err == nil {
		t.Fatal("cost overflow accepted")
	}
	if _, err := Units(9223372036854775807); err == nil {
		t.Fatal("unit conversion overflow accepted")
	}
	q, err := Units(3)
	if err != nil || q.String() != "3" || !q.Whole() {
		t.Fatalf("whole units %s %v", q, err)
	}
	q, _ = Parse("0.125")
	if q.Whole() {
		t.Fatal("fractional quantity marked whole")
	}
	q, err = q.Multiply(3)
	if err != nil || q.String() != "0.375" {
		t.Fatalf("multiply %s %v", q, err)
	}
}

func TestInvalidDecodeDoesNotMutateReceiver(t *testing.T) {
	q, _ := Parse("2.5")
	if err := json.Unmarshal([]byte("0.0001"), &q); err == nil {
		t.Fatal("accepted excess precision")
	}
	if q.String() != "2.5" {
		t.Fatal("failed decoding changed receiver")
	}
}

func TestHumanUnitReportFormatting(t *testing.T) {
	q, _ := Parse("2.125")
	if got := fmt.Sprintf("%d", q); got != "2.125" {
		t.Fatalf("report quantity %s", got)
	}
}
