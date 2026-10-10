package domain

import (
	"regexp"
	"strings"
)

// Limits for names.
const (
	MaxNameLen = 120
)

var prefixRe = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)

// Category groups items and provides the prefix of generated item codes.
type Category struct {
	ID         int64
	Name       string
	CodePrefix string
}

// NormalizeCategory trims the name and uppercases the prefix, validating both.
func NormalizeCategory(name, prefix string) (string, string, error) {
	fe := fieldErrors{}
	n := strings.TrimSpace(name)
	if n == "" || len(n) > MaxNameLen {
		fe.add("name", "required, up to 120 characters")
	}
	p := strings.ToUpper(strings.TrimSpace(prefix))
	if !prefixRe.MatchString(p) {
		fe.add("code_prefix", "2 to 8 characters, letters A-Z and digits only")
	}
	if err := fe.err(); err != nil {
		return "", "", err
	}
	return n, p, nil
}
