package models

import "github.com/google/uuid"

// Practitioner is a clinician who sees patients at one or more clinics.
// @gloss represents clinic:Practitioner conf=0.95
type Practitioner struct {
	ID          uuid.UUID   `json:"id"`
	DisplayName string      `json:"displayName"`
	Specialty   string      `json:"specialty"`
	ClinicIDs   []uuid.UUID `json:"clinicIds"`
	Accepting   bool        `json:"acceptingNewPatients"`
}
