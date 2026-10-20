// Package store is the Postgres persistence for the booking API.
package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoRows = errors.New("not found")

type DB struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *DB { return &DB{pool: pool} }

func (db *DB) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoRows
	}
	return err
}
