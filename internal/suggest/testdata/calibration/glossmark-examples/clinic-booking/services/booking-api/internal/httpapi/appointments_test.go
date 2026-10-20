// @gloss verifies .BookingApiService scope=file conf=0.75
package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/booking"
	"github.com/larkspur-clinics/booking-api/internal/models"
)

type stubBooker struct{ err error }

func (s stubBooker) Book(context.Context, booking.BookRequest) (models.Appointment, error) {
	return models.Appointment{ID: uuid.New(), Status: models.StatusBooked}, s.err
}
func (s stubBooker) Cancel(context.Context, uuid.UUID) (models.Appointment, error) {
	return models.Appointment{}, s.err
}
func (s stubBooker) FindOpenSlots(context.Context, uuid.UUID, time.Time, time.Time) ([]models.Slot, error) {
	return nil, s.err
}

type noPractitioners struct{}

func (noPractitioners) ListPractitioners(context.Context, string) ([]models.Practitioner, error) {
	return nil, nil
}

func TestCreateAppointmentMapsConflict(t *testing.T) {
	router := NewRouter(stubBooker{err: booking.ErrSlotTaken}, noPractitioners{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/v1/appointments", strings.NewReader(`{"slotId":"`+uuid.NewString()+`"}`))
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}

func TestRequiresSession(t *testing.T) {
	router := NewRouter(stubBooker{}, noPractitioners{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/practitioners", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
