package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OutboxMessage struct {
	ID      uuid.UUID
	Kind    string
	Payload []byte
}

// Enqueue writes an event to the transactional outbox; the relay ships it.
func (db *DB) Enqueue(ctx context.Context, kind string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `INSERT INTO outbox (id, kind, payload) VALUES ($1, $2, $3)`, uuid.New(), kind, body)
	return err
}

func (db *DB) ClaimOutbox(ctx context.Context, limit int, fn func([]OutboxMessage) error) error {
	return db.inTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, kind, payload FROM outbox WHERE sent_at IS NULL
			ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		var batch []OutboxMessage
		for rows.Next() {
			var m OutboxMessage
			if err := rows.Scan(&m.ID, &m.Kind, &m.Payload); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, m)
		}
		rows.Close()
		if err := fn(batch); err != nil {
			return err
		}
		for _, m := range batch {
			if _, err := tx.Exec(ctx, `UPDATE outbox SET sent_at = now() WHERE id = $1`, m.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
