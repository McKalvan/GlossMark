package models

import (
	"time"

	"github.com/google/uuid"
)

// Slot is a bookable period in a practitioner's schedule.
// @gloss represents clinic:Slot conf=0.95
type Slot struct {
	ID             uuid.UUID `json:"id"`
	PractitionerID uuid.UUID `json:"practitionerId"`
	ClinicID       uuid.UUID `json:"clinicId"`
	StartsAt       time.Time `json:"startsAt"`
	EndsAt         time.Time `json:"endsAt"`
	Booked         bool      `json:"booked"`
}

func (s Slot) Duration() time.Duration { return s.EndsAt.Sub(s.StartsAt) }
