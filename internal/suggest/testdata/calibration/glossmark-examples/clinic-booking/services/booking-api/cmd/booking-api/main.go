// @gloss implements .BookingApiService scope=file conf=0.8
// Command booking-api serves the Larkspur Clinics booking HTTP API.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/larkspur-clinics/booking-api/internal/booking"
	"github.com/larkspur-clinics/booking-api/internal/httpapi"
	"github.com/larkspur-clinics/booking-api/internal/outbox"
	"github.com/larkspur-clinics/booking-api/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	db := store.New(pool)
	relay := outbox.NewRelay(db, os.Getenv("REMINDER_QUEUE_URL"), logger)
	go relay.Run(ctx)

	svc := booking.NewService(db, db, db, time.Now)
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           httpapi.NewRouter(svc, db, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	logger.Info("booking-api listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("serve", "err", err)
		os.Exit(1)
	}
}
