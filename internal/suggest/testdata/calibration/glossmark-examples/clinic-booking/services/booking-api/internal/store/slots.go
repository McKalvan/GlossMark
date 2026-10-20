package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

func (db *DB) GetSlot(ctx context.Context, id uuid.UUID) (models.Slot, error) {
	var s models.Slot
	err := db.pool.QueryRow(ctx, `SELECT id, practitioner_id, clinic_id, starts_at, ends_at, booked FROM slots WHERE id = $1`, id).
		Scan(&s.ID, &s.PractitionerID, &s.ClinicID, &s.StartsAt, &s.EndsAt, &s.Booked)
	return s, notFound(err)
}

func (db *DB) ListOpenSlots(ctx context.Context, practitionerID uuid.UUID, from, to time.Time) ([]models.Slot, error) {
	rows, err := db.pool.Query(ctx, `SELECT id, practitioner_id, clinic_id, starts_at, ends_at, booked FROM slots
		WHERE practitioner_id = $1 AND NOT booked AND starts_at >= $2 AND starts_at < $3`, practitionerID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Slot
	for rows.Next() {
		var s models.Slot
		if err := rows.Scan(&s.ID, &s.PractitionerID, &s.ClinicID, &s.StartsAt, &s.EndsAt, &s.Booked); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (db *DB) MarkSlotBooked(ctx context.Context, id uuid.UUID, booked bool) error {
	_, err := db.pool.Exec(ctx, `UPDATE slots SET booked = $2 WHERE id = $1`, id, booked)
	return err
}
