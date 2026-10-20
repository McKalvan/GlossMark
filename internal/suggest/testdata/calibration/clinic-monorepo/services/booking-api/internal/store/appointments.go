package store

type AppointmentStore struct{}

func (s *AppointmentStore) Insert(a string) error { return nil }

func (s *AppointmentStore) Update(a string) error { return nil }

func (s *AppointmentStore) Delete(id string) error { return nil }

func (s *AppointmentStore) ListByPractitioner(id string) ([]string, error) { return nil, nil }
