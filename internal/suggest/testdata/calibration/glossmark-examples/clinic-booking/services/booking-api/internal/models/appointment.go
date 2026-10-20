package models

import (
	"time"

	"github.com/google/uuid"
)

type AppointmentStatus string

const (
	StatusBooked    AppointmentStatus = "booked"
	StatusCancelled AppointmentStatus = "cancelled"
	StatusAttended  AppointmentStatus = "attended"
	StatusNoShow    AppointmentStatus = "no_show"
)

// Appointment is a patient's reservation of one slot.
// @gloss represents clinic:Appointment conf=0.95
type Appointment struct {
	ID             uuid.UUID         `json:"id"`
	PatientID      uuid.UUID         `json:"patientId"`
	PractitionerID uuid.UUID         `json:"practitionerId"`
	SlotID         uuid.UUID         `json:"slotId"`
	StartsAt       time.Time         `json:"startsAt"`
	Reason         string            `json:"reason,omitempty"`
	Status         AppointmentStatus `json:"status"`
	CancelledAt    *time.Time        `json:"cancelledAt,omitempty"`
	LateCancel     bool              `json:"lateCancel"`
	CreatedAt      time.Time         `json:"createdAt"`
}
