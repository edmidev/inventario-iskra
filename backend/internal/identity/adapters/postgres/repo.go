// Package postgres implements the identity repositories on PostgreSQL via pgx.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/edgar/inventario/backend/internal/identity/app"
	"github.com/edgar/inventario/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// adminLockKey serializes changes that can affect the active-admin count.
const adminLockKey = 0x1d3e70001

const userCols = `id::text, name, email, password_hash, role, active, created_at, updated_at`

// Repo implements app.UserRepository and app.RefreshTokenRepository.
type Repo struct{ pool *pgxpool.Pool }

// New builds a Repo on an open pool.
func New(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Tokens returns the refresh-token repository view.
func (r *Repo) Tokens() app.RefreshTokenRepository { return tokenRepo{r.pool} }

func scanUser(row pgx.Row) (domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &role, &u.Active, &u.CreatedAt, &u.UpdatedAt)
	u.Role = domain.Role(role)
	return u, err
}

// Create implements app.UserRepository.
func (r *Repo) Create(ctx context.Context, u *domain.User) error {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO users (name, email, password_hash, role, active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+userCols, u.Name, u.Email, u.PasswordHash, string(u.Role), u.Active)
	created, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrEmailTaken
		}
		return fmt.Errorf("create user: %w", err)
	}
	*u = created
	return nil
}

// GetByID implements app.UserRepository.
func (r *Repo) GetByID(ctx context.Context, id string) (domain.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1::uuid`, id))
	return u, mapNotFound(err)
}

// GetByEmail implements app.UserRepository.
func (r *Repo) GetByEmail(ctx context.Context, email string) (domain.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
	return u, mapNotFound(err)
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("query user: %w", err)
	}
	return nil
}

// List implements app.UserRepository.
func (r *Repo) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []domain.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Count implements app.UserRepository.
func (r *Repo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// Mutate implements app.UserRepository. A transaction-scoped advisory lock
// serializes concurrent mutations so the last-admin rule cannot be raced.
func (r *Repo) Mutate(ctx context.Context, id string, fn func(*domain.User, int) error) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(adminLockKey)); err != nil {
		return domain.User{}, fmt.Errorf("lock: %w", err)
	}
	u, err := scanUser(tx.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1::uuid FOR UPDATE`, id))
	if err != nil {
		return domain.User{}, mapNotFound(err)
	}
	var admins int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role = 'admin' AND active`).Scan(&admins); err != nil {
		return domain.User{}, fmt.Errorf("count admins: %w", err)
	}
	if err := fn(&u, admins); err != nil {
		return domain.User{}, err
	}
	updated, err := scanUser(tx.QueryRow(ctx, `
		UPDATE users SET name = $2, role = $3, active = $4, password_hash = $5, updated_at = now()
		WHERE id = $1::uuid RETURNING `+userCols, id, u.Name, string(u.Role), u.Active, u.PasswordHash))
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("commit: %w", err)
	}
	return updated, nil
}

const tokenCols = `id::text, user_id::text, token_hash, expires_at, revoked_at, created_at, rotated_at, replaced_by::text`

func tokenDest(rt *app.RefreshToken) []any {
	return []any{&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.CreatedAt, &rt.RotatedAt, &rt.ReplacedBy}
}

type tokenRepo struct{ pool *pgxpool.Pool }

func (t tokenRepo) Create(ctx context.Context, rt app.RefreshToken) error {
	_, err := t.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1::uuid, $2, $3)`,
		rt.UserID, rt.TokenHash, rt.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (t tokenRepo) GetByHash(ctx context.Context, hash string) (app.RefreshToken, error) {
	var rt app.RefreshToken
	err := t.pool.QueryRow(ctx, `
		SELECT `+tokenCols+` FROM refresh_tokens WHERE token_hash = $1`, hash).
		Scan(tokenDest(&rt)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefreshToken{}, domain.ErrInvalidToken
	}
	if err != nil {
		return app.RefreshToken{}, fmt.Errorf("get refresh token: %w", err)
	}
	return rt, nil
}

func (t tokenRepo) GetByID(ctx context.Context, id string) (app.RefreshToken, error) {
	var rt app.RefreshToken
	err := t.pool.QueryRow(ctx, `
		SELECT `+tokenCols+` FROM refresh_tokens WHERE id = $1::uuid`, id).
		Scan(tokenDest(&rt)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.RefreshToken{}, domain.ErrInvalidToken
	}
	if err != nil {
		return app.RefreshToken{}, fmt.Errorf("get refresh token: %w", err)
	}
	return rt, nil
}

// Rotate revokes the token and inserts its successor in one transaction. The
// UPDATE takes the row lock, so concurrent rotations of the same token are
// serialized and exactly one of them sees the token still active.
func (t tokenRepo) Rotate(ctx context.Context, id string, next app.RefreshToken, at time.Time) (bool, error) {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2, rotated_at = $2 WHERE id = $1::uuid AND revoked_at IS NULL`, id, at)
	if err != nil {
		return false, fmt.Errorf("rotate refresh token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	var nextID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1::uuid, $2, $3) RETURNING id::text`,
		next.UserID, next.TokenHash, next.ExpiresAt).Scan(&nextID); err != nil {
		return false, fmt.Errorf("create refresh token: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE refresh_tokens SET replaced_by = $2::uuid WHERE id = $1::uuid`, id, nextID); err != nil {
		return false, fmt.Errorf("link refresh token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

func (t tokenRepo) Revoke(ctx context.Context, id string, at time.Time) (bool, error) {
	tag, err := t.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE id = $1::uuid AND revoked_at IS NULL`, id, at)
	if err != nil {
		return false, fmt.Errorf("revoke refresh token: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (t tokenRepo) RevokeAllForUser(ctx context.Context, userID string, at time.Time) error {
	_, err := t.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID, at)
	if err != nil {
		return fmt.Errorf("revoke user tokens: %w", err)
	}
	return nil
}
