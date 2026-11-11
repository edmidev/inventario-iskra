// Package quantity provides exact quantities at one-thousandth-unit precision.
// Quantities travel over JSON and SQL in human units; their Go representation
// is integer thousandths. Never construct them by casting human-unit integers.
package quantity

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Quantity stores thousandths of a unit. Its zero value represents zero units.
type Quantity int64

const Scale int64 = 1000

var ErrInvalid = errors.New("quantity must be finite, fit int64 thousandths and have at most three decimals")
var number = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// Parse reads an exact human-unit number, including bounded JSON exponents.
func Parse(s string) (Quantity, error) {
	if len(s) > 64 || !number.MatchString(s) {
		return 0, ErrInvalid
	}
	if dot := strings.IndexByte(s, '.'); dot >= 0 && !strings.ContainsAny(s, "eE") && len(s)-dot-1 > 3 {
		return 0, ErrInvalid
	}
	if p := strings.IndexAny(s, "eE"); p >= 0 {
		e, err := strconv.Atoi(s[p+1:])
		if err != nil || e > 18 || e < -18 {
			return 0, ErrInvalid
		}
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, ErrInvalid
	}
	r.Mul(r, big.NewRat(Scale, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return 0, ErrInvalid
	}
	return Quantity(r.Num().Int64()), nil
}

// Units converts whole human units to an exact quantity, checking overflow.
func Units(n int64) (Quantity, error) { return Parse(strconv.FormatInt(n, 10)) }

// Whole reports whether this quantity represents a whole number of units.
func (q Quantity) Whole() bool { return int64(q)%Scale == 0 }

// String returns the canonical human-unit decimal without trailing zeros.
func (q Quantity) String() string {
	n := int64(q)
	whole, fraction := n/Scale, n%Scale
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	sign := ""
	if n < 0 && whole == 0 {
		sign = "-"
	}
	if fraction < 0 {
		fraction = -fraction
	}
	return sign + strconv.FormatInt(whole, 10) + "." + strings.TrimRight(fmt.Sprintf("%03d", fraction), "0")
}

// MarshalJSON writes a JSON number in human units.
func (q Quantity) MarshalJSON() ([]byte, error) { return []byte(q.String()), nil }

// UnmarshalJSON rejects null, quoted numbers and unrepresentable precision.
func (q *Quantity) UnmarshalJSON(b []byte) error {
	v, err := Parse(string(b))
	if err == nil {
		*q = v
	}
	return err
}

// Value stores the human-unit decimal without conversion through float64.
func (q Quantity) Value() (driver.Value, error) { return q.String(), nil }

// NumericValue implements pgx's exact NUMERIC encoder.
func (q Quantity) NumericValue() (pgtype.Numeric, error) {
	return pgtype.Numeric{Int: big.NewInt(int64(q)), Exp: -3, Valid: true}, nil
}

// ScanNumeric implements pgx's exact NUMERIC decoder.
func (q *Quantity) ScanNumeric(n pgtype.Numeric) error {
	if !n.Valid || n.NaN || n.InfinityModifier != pgtype.Finite || n.Int == nil {
		return ErrInvalid
	}
	v, err := Parse(n.Int.String() + "e" + strconv.Itoa(int(n.Exp)))
	if err == nil {
		*q = v
	}
	return err
}

// Scan supports database/sql readers without lossy numeric conversions.
func (q *Quantity) Scan(v any) error {
	var s string
	switch n := v.(type) {
	case string:
		s = n
	case []byte:
		s = string(n)
	case int64:
		s = strconv.FormatInt(n, 10)
	default:
		return ErrInvalid
	}
	value, err := Parse(s)
	if err == nil {
		*q = value
	}
	return err
}

// Add adds two quantities with overflow checking.
func (q Quantity) Add(other Quantity) (Quantity, error) {
	n := new(big.Int).Add(big.NewInt(int64(q)), big.NewInt(int64(other)))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return Quantity(n.Int64()), nil
}

// Multiply scales a quantity by an integer count with overflow checking.
func (q Quantity) Multiply(count int64) (Quantity, error) {
	n := new(big.Int).Mul(big.NewInt(int64(q)), big.NewInt(count))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return Quantity(n.Int64()), nil
}

// Cost returns quantity times a nonnegative minor-unit price, rounded half up.
func (q Quantity) Cost(unitCostMinor int64) (int64, error) {
	if q < 0 || unitCostMinor < 0 {
		return 0, ErrInvalid
	}
	n := new(big.Int).Mul(big.NewInt(int64(q)), big.NewInt(unitCostMinor))
	n.Add(n, big.NewInt(Scale/2))
	n.Quo(n, big.NewInt(Scale))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}

// Format renders quantities in human units, including legacy integer-format
// report columns. The internal thousandths representation is never user copy.
func (q Quantity) Format(s fmt.State, verb rune) {
	text := q.String()
	if verb == 'q' {
		text = strconv.Quote(text)
	}
	_, _ = s.Write([]byte(text))
}
