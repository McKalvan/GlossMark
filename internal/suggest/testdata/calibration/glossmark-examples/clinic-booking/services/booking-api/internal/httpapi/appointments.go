package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/booking"
	"github.com/larkspur-clinics/booking-api/internal/models"
)

type Booker interface {
	Book(ctx context.Context, req booking.BookRequest) (models.Appointment, error)
	Cancel(ctx context.Context, id uuid.UUID) (models.Appointment, error)
	FindOpenSlots(ctx context.Context, practitionerID uuid.UUID, from, to time.Time) ([]models.Slot, error)
}

type handlers struct {
	svc           Booker
	practitioners PractitionerLister
}

func (h *handlers) createAppointment(w http.ResponseWriter, r *http.Request) {
	var req booking.BookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	appt, err := h.svc.Book(r.Context(), req)
	switch {
	case errors.Is(err, booking.ErrSlotTaken), errors.Is(err, booking.ErrDoubleBooking):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, booking.ErrSlotInPast):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not book")
	default:
		writeJSON(w, http.StatusCreated, appt)
	}
}

func (h *handlers) getAppointment(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "served by the read model")
}

func (h *handlers) cancelAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	appt, err := h.svc.Cancel(r.Context(), id)
	switch {
	case errors.Is(err, booking.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, booking.ErrAlreadyResolved):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not cancel")
	default:
		writeJSON(w, http.StatusOK, appt)
	}
}
