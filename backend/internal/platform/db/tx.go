package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type txKey struct{}

// WithTx returns ctx carrying tx. A repository that begins its own
// transaction through Begin joins this one instead, so a use case in another
// module (purchasing receiving goods) can make catalog and stock writes
// atomic with its own without those modules importing each other.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// Begin starts a transaction on pool, or joins the one carried by ctx. It
// returns the transaction plus commit and rollback functions: on a joined
// transaction both are no-ops, because the owner decides its outcome. Safe to
// defer rollback after a successful commit.
func Begin(ctx context.Context, pool *pgxpool.Pool) (tx pgx.Tx, commit func() error, rollback func(), err error) {
	if outer, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return outer, func() error { return nil }, func() {}, nil
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	return tx,
		func() error { return tx.Commit(ctx) },
		func() { _ = tx.Rollback(ctx) },
		nil
}
