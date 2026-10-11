// Package importer reads the legacy Excel workbook into a reviewable import
// plan, renders that plan as a report and applies it through the catalog,
// stock and maintenance services. Nothing here writes to the database unless
// Apply is called.
package importer

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Canonical category and location names the importer maps the workbook to.
const (
	CatMachinery   = "Maquinaria"
	CatTools       = "Herramientas"
	CatElectrical  = "Electricidad"
	CatConsumables = "Consumibles"

	// LocationCase is the default location of the first block of sheet 2.
	LocationCase = "Maleta y caja de Alejandro"
	// DefaultRepeatLocation is the default location of the repeated block of
	// sheet 2 (the workshop); the CLI uses it for sheets 1 and 3 as well.
	DefaultRepeatLocation = "Taller"
)

// categoryPrefixes are the code prefixes of the categories the importer may
// create. They only matter for newly generated codes.
var categoryPrefixes = map[string]string{
	CatMachinery: "MAQ", CatTools: "HER", CatElectrical: "ELE", CatConsumables: "CON",
}

// categoryOrder keeps report output stable.
var categoryOrder = []string{CatMachinery, CatTools, CatElectrical, CatConsumables}

// Fold lowercases s, removes accents and collapses every run of whitespace
// into one space. It is the comparison form used for descriptions, names and
// header labels.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Clean trims s and collapses internal whitespace, keeping case and accents.
func Clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// foldKey is Fold without any non-alphanumeric character, so "maquinaria/ "
// and "Maquinaria" meet.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range Fold(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CanonicalCategory maps a raw workbook category to one of the known category
// names, tolerating case, accents, slashes and spaces. ok is false for empty
// or unknown values.
func CanonicalCategory(raw string) (name string, ok bool) {
	switch foldKey(raw) {
	case "maquinaria", "maquinarias":
		return CatMachinery, true
	case "herramientas", "herramienta":
		return CatTools, true
	case "electricidad":
		return CatElectrical, true
	case "consumibles", "consumible":
		return CatConsumables, true
	}
	return "", false
}

var codeRe = regexp.MustCompile(`(?i)^(ISK-[A-Z0-9]+(?:-[A-Z0-9]+)*)(?:\s+(.*))?$`)

// SplitCode recognizes a legacy code at the start of a cell. rest is whatever
// follows the code after whitespace (a description fused into the cell).
func SplitCode(cell string) (code, rest string, ok bool) {
	m := codeRe.FindStringSubmatch(strings.TrimSpace(cell))
	if m == nil {
		return "", "", false
	}
	return strings.ToUpper(m[1]), Clean(m[2]), true
}

// parseQuantity reads a whole non-negative number from a raw cell.
func parseQuantity(raw string) (n int, present bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	f, perr := strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
	if perr != nil || f != float64(int64(f)) || f < 0 || f > 1_000_000_000 {
		return 0, true, errBadQuantity(raw)
	}
	return int(f), true, nil
}

type errBadQuantity string

func (e errBadQuantity) Error() string {
	return "quantity " + strconv.Quote(string(e)) + " is not a whole number between 0 and 1000000000"
}

// excelEpoch is the day zero of the 1900 date system (after Excel's phantom
// 1900-02-29), in UTC.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// parseDate reads a date cell: an Excel serial number, ISO text or
// dd/mm/yyyy text. The result is midnight of that calendar day in loc.
// via says how it was read ("excel serial", "dd/mm/yyyy text", "iso text").
func parseDate(raw string, loc *time.Location) (t time.Time, via string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, "", false
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		if f < 1 || f > 2_958_465 { // 1900-01-01 .. 9999-12-31
			return time.Time{}, "", false
		}
		d := excelEpoch.AddDate(0, 0, int(f))
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), "excel serial " + strconv.Itoa(int(f)), true
	}
	for _, l := range []struct{ layout, via string }{
		{"2/1/2006", "dd/mm/yyyy text"}, {"02-01-2006", "dd-mm-yyyy text"}, {"2006-01-02", "iso text"},
	} {
		if p, err := time.Parse(l.layout, raw); err == nil {
			return time.Date(p.Year(), p.Month(), p.Day(), 0, 0, 0, 0, loc), l.via, true
		}
	}
	return time.Time{}, "", false
}
