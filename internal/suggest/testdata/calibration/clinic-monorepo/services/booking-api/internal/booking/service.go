package booking

import (
	"errors"
	"time"
)

// CancellationWindow is how late a patient may cancel for free.
const CancellationWindow = 24 * time.Hour

var ErrNoRows = errors.New("no rows")

// Service books and cancels appointments.
type Service struct {
	repo Repo
}

// Repo is the storage port.
type Repo interface {
	GetSlot(id string) (Slot, error)
}

// @gloss represents clinic:Slot
type Slot struct {
	ID   string
	From time.Time
}

// @gloss implements .BookingHandler
func (s *Service) Book(patient, slot string) error {
	if _, err := s.repo.GetSlot(slot); err != nil {
		return err
	}
	return nil
}

// @gloss implements .CancellationPolicy
func (s *Service) Cancel(id string, now time.Time) error {
	return nil
}

func NewService(r Repo) *Service {
	return &Service{repo: r}
}

func overlaps(a, b Slot) bool {
	return a.From.Equal(b.From)
}
