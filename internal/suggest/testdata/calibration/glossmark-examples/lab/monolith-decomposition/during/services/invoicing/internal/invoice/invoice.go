package invoice

import "time"

// @gloss represents hire:Invoice
type Invoice struct {
	ID       int64
	HireID   int64
	NetPence int64
	VATPence int64
	IssuedAt time.Time
}
