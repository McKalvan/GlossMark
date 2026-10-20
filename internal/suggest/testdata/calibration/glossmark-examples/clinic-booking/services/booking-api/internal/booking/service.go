// Package booking holds the booking rules: who may book which slot, and when.
package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

var (
	ErrSlotTaken       = errors.New("slot already booked")
	ErrSlotInPast      = errors.New("slot starts in the past")
	ErrDoubleBooking   = errors.New("patient already has an appointment that overlaps this slot")
	ErrNotFound        = errors.New("appointment not found")
	ErrAlreadyResolved = errors.New("appointment is no longer booked")
)

type SlotRepo interface {
	GetSlot(ctx context.Context, id uuid.UUID) (models.Slot, error)
	ListOpenSlots(ctx context.Context, practitionerID uuid.UUID, from, to time.Time) ([]models.Slot, error)
	MarkSlotBooked(ctx context.Context, id uuid.UUID, booked bool) error
}

type AppointmentRepo interface {
	CreateAppointment(ctx context.Context, a models.Appointment) error
	GetAppointment(ctx context.Context, id uuid.UUID) (models.Appointment, error)
	UpdateAppointment(ctx context.Context, a models.Appointment) error
	ListPatientAppointments(ctx context.Context, patientID uuid.UUID, from, to time.Time) ([]models.Appointment, error)
}

type EventWriter interface {
	Enqueue(ctx context.Context, kind string, payload any) error
}

type Service struct {
	slots  SlotRepo
	appts  AppointmentRepo
	events EventWriter
	now    func() time.Time
}

func NewService(slots SlotRepo, appts AppointmentRepo, events EventWriter, now func() time.Time) *Service {
	return &Service{slots: slots, appts: appts, events: events, now: now}
}

type BookRequest struct {
	PatientID uuid.UUID `json:"patientId"`
	SlotID    uuid.UUID `json:"slotId"`
	Reason    string    `json:"reason"`
}

// Book reserves a slot for a patient and schedules the reminder through the outbox.
// @gloss implements .BookingHandler conf=0.91
func (s *Service) Book(ctx context.Context, req BookRequest) (models.Appointment, error) {
	slot, err := s.slots.GetSlot(ctx, req.SlotID)
	if err != nil {
		return models.Appointment{}, fmt.Errorf("load slot: %w", err)
	}
	if slot.Booked {
		return models.Appointment{}, ErrSlotTaken
	}
	if slot.StartsAt.Before(s.now()) {
		return models.Appointment{}, ErrSlotInPast
	}
	existing, err := s.appts.ListPatientAppointments(ctx, req.PatientID, slot.StartsAt.Add(-time.Hour), slot.EndsAt)
	if err != nil {
		return models.Appointment{}, fmt.Errorf("check overlaps: %w", err)
	}
	for _, a := range existing {
		if a.Status == models.StatusBooked {
			return models.Appointment{}, ErrDoubleBooking
		}
	}

	appt := models.Appointment{
		ID:             uuid.New(),
		PatientID:      req.PatientID,
		PractitionerID: slot.PractitionerID,
		SlotID:         slot.ID,
		StartsAt:       slot.StartsAt,
		Reason:         req.Reason,
		Status:         models.StatusBooked,
		CreatedAt:      s.now(),
	}
	if err := s.appts.CreateAppointment(ctx, appt); err != nil {
		return models.Appointment{}, fmt.Errorf("create appointment: %w", err)
	}
	if err := s.slots.MarkSlotBooked(ctx, slot.ID, true); err != nil {
		return models.Appointment{}, fmt.Errorf("mark slot: %w", err)
	}
	if err := s.events.Enqueue(ctx, "appointment.booked", appt); err != nil {
		return models.Appointment{}, fmt.Errorf("enqueue reminder: %w", err)
	}
	return appt, nil
}
