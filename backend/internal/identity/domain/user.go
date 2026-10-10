// Package domain holds the identity entities and business rules.
package domain

import (
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

// Role is a user's authorization level.
type Role string

// Supported roles.
const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

// Permission is a coarse capability checked by HTTP middleware.
type Permission string

// Supported permissions.
const (
	// PermRead allows reading inventory data.
	PermRead Permission = "inventory:read"
	// PermWrite allows operational writes (movements, custody, maintenance).
	PermWrite Permission = "operations:write"
	// PermWriteItems allows creating, editing and deleting inventory items.
	PermWriteItems Permission = "items:write"
	// PermManageCatalogs allows editing categories and locations, and supplying item codes.
	PermManageCatalogs Permission = "catalogs:manage"
	// PermManageUsers allows user administration.
	PermManageUsers Permission = "users:manage"
)

var rolePermissions = map[Role][]Permission{
	RoleViewer:   {PermRead},
	RoleOperator: {PermRead, PermWrite, PermWriteItems},
	RoleAdmin:    {PermRead, PermWrite, PermWriteItems, PermManageCatalogs, PermManageUsers},
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	_, ok := rolePermissions[r]
	return ok
}

// Has reports whether the role grants permission p.
func (r Role) Has(p Permission) bool {
	for _, granted := range rolePermissions[r] {
		if granted == p {
			return true
		}
	}
	return false
}

// Permissions returns a copy of the permissions granted to r.
func (r Role) Permissions() []Permission {
	return append([]Permission(nil), rolePermissions[r]...)
}

// Validation limits.
const (
	MinPasswordLen = 10
	MaxPasswordLen = 128
	MaxNameLen     = 120
)

// Domain errors. The HTTP adapter maps each to a status and stable code.
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid or expired token")
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailTaken         = errors.New("email already in use")
	ErrLastAdmin          = errors.New("cannot deactivate or demote the last active admin")
	ErrTooManyAttempts    = errors.New("too many login attempts")
)

// User is an account that can sign in.
type User struct {
	ID           string
	Name         string
	Email        string
	PasswordHash string
	Role         Role
	Active       bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// removesActiveAdmin reports whether applying the new role/active values
// to u would remove an active admin, given the current count of active admins.
func (u User) removesActiveAdmin(newRole Role, newActive bool) bool {
	return u.Role == RoleAdmin && u.Active && (newRole != RoleAdmin || !newActive)
}

// CanChangeTo checks the "last active admin" invariant for a role/active
// change. activeAdmins is the current number of active admins (including u).
func (u User) CanChangeTo(newRole Role, newActive bool, activeAdmins int) error {
	if u.removesActiveAdmin(newRole, newActive) && activeAdmins <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// NormalizeEmail trims and lowercases an email, returning ErrInvalidInput if
// it is not a plain address.
func NormalizeEmail(raw string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(raw))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e || len(e) > 254 {
		return "", ErrInvalidInput
	}
	return e, nil
}

// NormalizeName trims and validates a display name.
func NormalizeName(raw string) (string, error) {
	n := strings.TrimSpace(raw)
	if n == "" || len(n) > MaxNameLen {
		return "", ErrInvalidInput
	}
	return n, nil
}

// ValidatePassword enforces the password policy.
func ValidatePassword(p string) error {
	if len(p) < MinPasswordLen || len(p) > MaxPasswordLen {
		return ErrInvalidInput
	}
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidID reports whether id is a canonical UUID string.
func ValidID(id string) bool { return uuidRe.MatchString(id) }
