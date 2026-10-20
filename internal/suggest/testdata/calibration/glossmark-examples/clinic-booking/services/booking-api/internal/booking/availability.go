package booking

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

// MaxSearchHorizon bounds how far ahead a patient can search for open slots.
const MaxSearchHorizon = 90 * 24 * time.Hour

// FindOpenSlots returns the practitioner's unbooked slots in [from, to), earliest first.
// @gloss implements .SlotSearch conf=0.36
func (s *Service) FindOpenSlots(ctx context.Context, practitionerID uuid.UUID, from, to time.Time) ([]models.Slot, error) {
	if from.Before(s.now()) {
		from = s.now()
	}
	if to.Sub(from) > MaxSearchHorizon {
		to = from.Add(MaxSearchHorizon)
	}
	slots, err := s.slots.ListOpenSlots(ctx, practitionerID, from, to)
	if err != nil {
		return nil, err
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].StartsAt.Before(slots[j].StartsAt) })
	return slots, nil
}
