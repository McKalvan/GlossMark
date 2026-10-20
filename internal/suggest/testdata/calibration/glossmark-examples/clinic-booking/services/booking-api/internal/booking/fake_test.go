package booking

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

type fakeRepo struct {
	slots  map[uuid.UUID]models.Slot
	appts  map[uuid.UUID]models.Appointment
	events []string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{slots: map[uuid.UUID]models.Slot{}, appts: map[uuid.UUID]models.Appointment{}}
}

func (f *fakeRepo) addSlot(start time.Time) models.Slot {
	s := models.Slot{ID: uuid.New(), PractitionerID: uuid.New(), StartsAt: start, EndsAt: start.Add(30 * time.Minute)}
	f.slots[s.ID] = s
	return s
}

func (f *fakeRepo) GetSlot(_ context.Context, id uuid.UUID) (models.Slot, error) {
	return f.slots[id], nil
}

func (f *fakeRepo) ListOpenSlots(_ context.Context, _ uuid.UUID, from, to time.Time) ([]models.Slot, error) {
	var out []models.Slot
	for _, s := range f.slots {
		if !s.Booked && !s.StartsAt.Before(from) && s.StartsAt.Before(to) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeRepo) MarkSlotBooked(_ context.Context, id uuid.UUID, booked bool) error {
	s := f.slots[id]
	s.Booked = booked
	f.slots[id] = s
	return nil
}

func (f *fakeRepo) CreateAppointment(_ context.Context, a models.Appointment) error {
	f.appts[a.ID] = a
	return nil
}

func (f *fakeRepo) GetAppointment(_ context.Context, id uuid.UUID) (models.Appointment, error) {
	return f.appts[id], nil
}

func (f *fakeRepo) UpdateAppointment(_ context.Context, a models.Appointment) error {
	f.appts[a.ID] = a
	return nil
}

func (f *fakeRepo) ListPatientAppointments(_ context.Context, patientID uuid.UUID, from, to time.Time) ([]models.Appointment, error) {
	var out []models.Appointment
	for _, a := range f.appts {
		if a.PatientID == patientID && !a.StartsAt.Before(from) && a.StartsAt.Before(to) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepo) Enqueue(_ context.Context, kind string, _ any) error {
	f.events = append(f.events, kind)
	return nil
}
