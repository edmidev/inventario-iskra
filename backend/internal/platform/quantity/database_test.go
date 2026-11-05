package quantity

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNumericCodecRoundTrip(t *testing.T) {
	m := pgtype.NewMap()
	for _, s := range []string{"0", "1", "0.125", "-2.5", "1000000000"} {
		t.Run(s, func(t *testing.T) {
			q, err := Parse(s)
			if err != nil {
				t.Fatal(err)
			}
			for _, format := range []int16{pgtype.TextFormatCode, pgtype.BinaryFormatCode} {
				encoded, err := m.Encode(pgtype.NumericOID, format, q, nil)
				if err != nil {
					t.Fatal(err)
				}
				var decoded Quantity
				if err = m.Scan(pgtype.NumericOID, format, encoded, &decoded); err != nil {
					t.Fatal(err)
				}
				if decoded != q {
					t.Fatalf("roundtrip %s != %s", decoded, q)
				}
			}
		})
	}
}
func TestNumericScanRejectsInvalid(t *testing.T) {
	for _, n := range []pgtype.Numeric{{}, {NaN: true, Valid: true}, {InfinityModifier: pgtype.Infinity, Valid: true}, {Int: big.NewInt(1), Exp: -4, Valid: true}} {
		var q Quantity
		if q.ScanNumeric(n) == nil {
			t.Fatal("accepted invalid numeric")
		}
	}
}
func TestSQLScan(t *testing.T) {
	for _, v := range []any{int64(2), "2", []byte("2.000")} {
		var q Quantity
		if err := q.Scan(v); err != nil || q.String() != "2" {
			t.Fatalf("%v: %s %v", v, q, err)
		}
		stored, err := q.Value()
		if err != nil || stored != "2" {
			t.Fatalf("value %v %v", stored, err)
		}
	}
	for _, v := range []any{nil, float64(2), "NaN", "0.0001"} {
		var q Quantity
		if q.Scan(v) == nil {
			t.Fatalf("accepted %v", v)
		}
	}
}
