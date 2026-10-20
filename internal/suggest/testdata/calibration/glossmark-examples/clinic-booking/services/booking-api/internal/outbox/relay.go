// Package outbox relays committed outbox rows to the reminder queue.
package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/larkspur-clinics/booking-api/internal/store"
)

// @gloss implements .OutboxRelay scope=symbol conf=0.64
type Relay struct {
	db       *store.DB
	queueURL string
	log      *slog.Logger
	send     func(ctx context.Context, queueURL string, msg store.OutboxMessage) error
}

func NewRelay(db *store.DB, queueURL string, log *slog.Logger) *Relay {
	return &Relay{db: db, queueURL: queueURL, log: log, send: sendSQS}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := r.db.ClaimOutbox(ctx, 50, func(batch []store.OutboxMessage) error {
				for _, m := range batch {
					if err := r.send(ctx, r.queueURL, m); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				r.log.Warn("outbox relay", "err", err)
			}
		}
	}
}
