// Package httpapi exposes the booking service over HTTP.
package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

type PractitionerLister interface {
	ListPractitioners(ctx context.Context, specialty string) ([]models.Practitioner, error)
}

func NewRouter(svc Booker, practitioners PractitionerLister, log *slog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, requestLogger(log), middleware.Recoverer)
	r.Use(requirePatientSession)

	h := &handlers{svc: svc, practitioners: practitioners}
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Route("/v1", func(r chi.Router) {
		r.Get("/practitioners", h.listPractitioners)
		r.Get("/practitioners/{id}/slots", h.listSlots)
		r.Post("/appointments", h.createAppointment)
		r.Get("/appointments/{id}", h.getAppointment)
		r.Post("/appointments/{id}/cancel", h.cancelAppointment)
	})
	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
