package booking

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

// CancellationWindow is how long before the start a patient may cancel without
// it counting as a late cancellation. Clinic policy, agreed with operations.
// @gloss implements .CancellationPolicy conf=0.95
const CancellationWindow = 24 * time.Hour

// Cancel releases the slot. Inside the window the appointment is flagged late,
// which the no-show report counts.
// @gloss implements .CancellationHandler conf=0.55
func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
	appt, err := s.appts.GetAppointment(ctx, id)
	if err != nil {
		return models.Appointment{}, ErrNotFound
	}
	if appt.Status != models.StatusBooked {
		return models.Appointment{}, ErrAlreadyResolved
	}
	now := s.now()
	appt.Status = models.StatusCancelled
	appt.CancelledAt = &now
	appt.LateCancel = appt.StartsAt.Sub(now) < CancellationWindow

	if err := s.appts.UpdateAppointment(ctx, appt); err != nil {
		return models.Appointment{}, fmt.Errorf("update appointment: %w", err)
	}
	if err := s.slots.MarkSlotBooked(ctx, appt.SlotID, false); err != nil {
		return models.Appointment{}, fmt.Errorf("release slot: %w", err)
	}
	if err := s.events.Enqueue(ctx, "appointment.cancelled", appt); err != nil {
		return models.Appointment{}, fmt.Errorf("enqueue cancellation: %w", err)
	}
	return appt, nil
}
