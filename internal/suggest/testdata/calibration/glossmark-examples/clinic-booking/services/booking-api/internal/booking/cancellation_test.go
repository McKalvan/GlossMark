package booking

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// @gloss verifies clinic:Cancellation conf=0.45
func TestCancelOutsideWindowIsNotLate(t *testing.T) {
	f := newFakeRepo()
	svc := newTestService(f)
	slot := f.addSlot(clock.Add(CancellationWindow + time.Hour))
	appt, _ := svc.Book(context.Background(), BookRequest{PatientID: uuid.New(), SlotID: slot.ID})
	got, err := svc.Cancel(context.Background(), appt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LateCancel {
		t.Error("cancellation outside the window flagged late")
	}
	if f.slots[slot.ID].Booked {
		t.Error("slot not released")
	}
}

// @gloss verifies clinic:CancellationWindow conf=0.45
func TestCancelInsideWindowIsLate(t *testing.T) {
	f := newFakeRepo()
	svc := newTestService(f)
	slot := f.addSlot(clock.Add(CancellationWindow - time.Hour))
	appt, _ := svc.Book(context.Background(), BookRequest{PatientID: uuid.New(), SlotID: slot.ID})
	got, _ := svc.Cancel(context.Background(), appt.ID)
	if !got.LateCancel {
		t.Error("cancellation inside the window not flagged late")
	}
}
