// Package httpapi exposes the identity use cases over HTTP under /api/v1.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/edgar/inventario/backend/internal/platform/httpx"
	"github.com/go-chi/chi/v5"
)

const (
	refreshCookieName = "refresh_token"
	refreshCookiePath = "/api/v1/auth"
	maxBodyBytes      = 1 << 20
)

// Handler serves the auth and user endpoints.
type Handler struct {
	svc          *app.Service
	cookieSecure bool
}

// New builds a Handler. cookieSecure sets the Secure attribute of the refresh
// cookie (disable only for plain-HTTP local development).
func New(svc *app.Service, cookieSecure bool) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure}
}

// Verifier adapts the access-token check to httpx.TokenVerifier, mapping the
// role to its permissions.
func (h *Handler) Verifier() httpx.TokenVerifier {
	return func(token string) (httpx.Principal, error) {
		c, err := h.svc.VerifyAccess(token)
		if err != nil {
			return httpx.Principal{}, err
		}
		perms := c.Role.Permissions()
		names := make([]string, len(perms))
		for i, p := range perms {
			names[i] = string(p)
		}
		return httpx.Principal{UserID: c.UserID, Role: string(c.Role), Permissions: names}, nil
	}
}

// Authenticate returns the bearer-token middleware for other modules to reuse.
func (h *Handler) Authenticate() func(http.Handler) http.Handler {
	return httpx.Authenticate(h.Verifier())
}

// Register mounts the routes on r (expected to be the /api/v1 router).
func (h *Handler) Register(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", h.login)
		r.Post("/refresh", h.refresh)
		r.Post("/logout", h.logout)
		r.Group(func(r chi.Router) {
			r.Use(h.Authenticate())
			r.Get("/me", h.me)
			r.Post("/password", h.changeOwnPassword)
		})
	})
	r.Route("/users", func(r chi.Router) {
		r.Use(h.Authenticate(), httpx.RequirePermission(string(domain.PermManageUsers)))
		r.Get("/", h.listUsers)
		r.Post("/", h.createUser)
		r.Patch("/{id}", h.updateUser)
		r.Post("/{id}/password", h.resetPassword)
	})
}

type userDTO struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toDTO(u domain.User) userDTO {
	return userDTO{ID: u.ID, Name: u.Name, Email: u.Email, Role: string(u.Role), Active: u.Active,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt}
}

type sessionDTO struct {
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	ExpiresIn   int     `json:"expires_in"`
	User        userDTO `json:"user"`
}

func (h *Handler) writeSession(w http.ResponseWriter, s app.Session) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: s.RefreshToken, Path: refreshCookiePath,
		Expires: s.RefreshExpires, HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, sessionDTO{
		AccessToken: s.AccessToken, TokenType: "Bearer",
		ExpiresIn: int(time.Until(s.AccessExpires).Seconds()), User: toDTO(s.User),
	})
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookieName, Value: "", Path: refreshCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest, "malformed JSON body")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	s, err := h.svc.Login(r.Context(), in.Email, in.Password, clientIP(r))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	h.writeSession(w, s)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var raw string
	if c, err := r.Cookie(refreshCookieName); err == nil {
		raw = c.Value
	}
	s, err := h.svc.Refresh(r.Context(), raw)
	if err != nil {
		h.clearCookie(w)
		writeDomainError(w, err)
		return
	}
	h.writeSession(w, s)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookieName); err == nil {
		if err := h.svc.Logout(r.Context(), c.Value); err != nil {
			slog.Error("logout failed", "err", err)
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	u, err := h.svc.Me(r.Context(), p.UserID)
	if err != nil || !u.Active {
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "account unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(u))
}

func (h *Handler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, _ := httpx.PrincipalFrom(r.Context())
	err := h.svc.ChangeOwnPassword(r.Context(), p.UserID, in.CurrentPassword, in.NewPassword)
	if errors.Is(err, domain.ErrInvalidCredentials) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_current_password", "current password is incorrect")
		return
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.ListUsers(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out := make([]userDTO, len(users))
	for i, u := range users {
		out[i] = toDTO(u)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := h.svc.CreateUser(r.Context(), app.CreateUserInput{
		Name: in.Name, Email: in.Email, Password: in.Password, Role: domain.Role(in.Role),
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDTO(u))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name   *string `json:"name"`
		Role   *string `json:"role"`
		Active *bool   `json:"active"`
	}
	if !decode(w, r, &in) {
		return
	}
	upd := app.UpdateUserInput{Name: in.Name, Active: in.Active}
	if in.Role != nil {
		role := domain.Role(*in.Role)
		upd.Role = &role
	}
	u, err := h.svc.UpdateUser(r.Context(), chi.URLParam(r, "id"), upd)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toDTO(u))
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		NewPassword string `json:"new_password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := h.svc.ResetPassword(r.Context(), chi.URLParam(r, "id"), in.NewPassword); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, httpx.CodeInvalidRequest,
			"invalid input: check name, email, role and password (min 10 characters)")
	case errors.Is(err, domain.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, domain.ErrInvalidToken):
		httpx.WriteError(w, http.StatusUnauthorized, httpx.CodeUnauthorized, "invalid or expired session")
	case errors.Is(err, domain.ErrUserNotFound):
		httpx.WriteError(w, http.StatusNotFound, httpx.CodeNotFound, "user not found")
	case errors.Is(err, domain.ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "email already in use")
	case errors.Is(err, domain.ErrLastAdmin):
		httpx.WriteError(w, http.StatusConflict, "last_admin", err.Error())
	case errors.Is(err, domain.ErrTooManyAttempts):
		w.Header().Set("Retry-After", "900")
		httpx.WriteError(w, http.StatusTooManyRequests, httpx.CodeRateLimited, "too many attempts, try again later")
	default:
		slog.Error("internal error", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	}
}
