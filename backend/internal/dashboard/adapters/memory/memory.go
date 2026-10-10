// Package memory is an in-memory dashboard repository for tests.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/edgar/inventario/backend/internal/dashboard/domain"
)

// Repo returns a canned summary and records the instant it was asked for.
type Repo struct {
	mu      sync.Mutex
	summary domain.Summary
	err     error
	lastNow time.Time
}

// New builds a Repo that returns the zero summary until Set is called.
func New() *Repo { return &Repo{} }

// Set replaces the summary to return.
func (r *Repo) Set(s domain.Summary) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.summary = s
}

// FailWith makes Summary return err (nil clears it).
func (r *Repo) FailWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

// LastNow returns the instant passed to the latest Summary call.
func (r *Repo) LastNow() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastNow
}

// Summary implements app.Repository.
func (r *Repo) Summary(_ context.Context, now time.Time) (domain.Summary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastNow = now
	return r.summary, r.err
}
