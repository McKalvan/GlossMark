package models

import (
	"time"

	"github.com/google/uuid"
)

// Patient is a person who books appointments. Fields are PII.
// @gloss represents clinic:Patient conf=0.95
type Patient struct {
	ID          uuid.UUID `json:"id"`
	GivenName   string    `json:"givenName"`
	FamilyName  string    `json:"familyName"`
	DateOfBirth time.Time `json:"dateOfBirth"`
	Phone       string    `json:"phone"`
	Email       string    `json:"email"`
	SMSConsent  bool      `json:"smsConsent"`
	CreatedAt   time.Time `json:"createdAt"`
}
