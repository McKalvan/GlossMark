// @gloss verifies clinic:Booking scope=file conf=0.85
package booking

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

var clock = time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)

func newTestService(f *fakeRepo) *Service {
	return NewService(f, f, f, func() time.Time { return clock })
}

func TestBookReservesSlotAndEnqueuesReminder(t *testing.T) {
	f := newFakeRepo()
	slot := f.addSlot(clock.Add(48 * time.Hour))
	appt, err := newTestService(f).Book(context.Background(), BookRequest{PatientID: uuid.New(), SlotID: slot.ID})
	if err != nil {
		t.Fatalf("Book: %v", err)
	}
	if appt.Status != models.StatusBooked {
		t.Errorf("status = %s, want booked", appt.Status)
	}
	if !f.slots[slot.ID].Booked {
		t.Error("slot not marked booked")
	}
	if len(f.events) != 1 || f.events[0] != "appointment.booked" {
		t.Errorf("events = %v", f.events)
	}
}

func TestBookRejectsTakenSlot(t *testing.T) {
	f := newFakeRepo()
	slot := f.addSlot(clock.Add(time.Hour))
	f.slots[slot.ID] = models.Slot{ID: slot.ID, StartsAt: slot.StartsAt, Booked: true}
	_, err := newTestService(f).Book(context.Background(), BookRequest{PatientID: uuid.New(), SlotID: slot.ID})
	if !errors.Is(err, ErrSlotTaken) {
		t.Fatalf("err = %v, want ErrSlotTaken", err)
	}
}

func TestBookRejectsDoubleBooking(t *testing.T) {
	f := newFakeRepo()
	patient := uuid.New()
	first := f.addSlot(clock.Add(4 * time.Hour))
	second := f.addSlot(clock.Add(4*time.Hour + 30*time.Minute))
	svc := newTestService(f)
	if _, err := svc.Book(context.Background(), BookRequest{PatientID: patient, SlotID: first.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Book(context.Background(), BookRequest{PatientID: patient, SlotID: second.ID})
	if !errors.Is(err, ErrDoubleBooking) {
		t.Fatalf("err = %v, want ErrDoubleBooking", err)
	}
}
