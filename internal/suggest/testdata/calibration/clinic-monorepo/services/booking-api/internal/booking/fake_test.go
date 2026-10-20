package booking

type fakeRepo struct{ slots map[string]Slot }

func (f *fakeRepo) GetSlot(id string) (Slot, error) {
	return f.slots[id], nil
}

func booked() Slot {
	return Slot{ID: "s1"}
}

type stubBooker struct{}

func (stubBooker) Book(patient, slot string) error { return nil }
