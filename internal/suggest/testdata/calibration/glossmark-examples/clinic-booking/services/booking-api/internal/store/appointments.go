package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

const appointmentColumns = `id, patient_id, practitioner_id, slot_id, starts_at, reason, status, cancelled_at, late_cancel, created_at`

func scanAppointment(row interface{ Scan(...any) error }) (models.Appointment, error) {
	var a models.Appointment
	err := row.Scan(&a.ID, &a.PatientID, &a.PractitionerID, &a.SlotID, &a.StartsAt, &a.Reason,
		&a.Status, &a.CancelledAt, &a.LateCancel, &a.CreatedAt)
	return a, notFound(err)
}

func (db *DB) CreateAppointment(ctx context.Context, a models.Appointment) error {
	_, err := db.pool.Exec(ctx, `INSERT INTO appointments (`+appointmentColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		a.ID, a.PatientID, a.PractitionerID, a.SlotID, a.StartsAt, a.Reason, a.Status, a.CancelledAt, a.LateCancel, a.CreatedAt)
	return err
}

func (db *DB) GetAppointment(ctx context.Context, id uuid.UUID) (models.Appointment, error) {
	return scanAppointment(db.pool.QueryRow(ctx, `SELECT `+appointmentColumns+` FROM appointments WHERE id = $1`, id))
}

func (db *DB) UpdateAppointment(ctx context.Context, a models.Appointment) error {
	_, err := db.pool.Exec(ctx, `UPDATE appointments SET status = $2, cancelled_at = $3, late_cancel = $4 WHERE id = $1`,
		a.ID, a.Status, a.CancelledAt, a.LateCancel)
	return err
}

func (db *DB) ListPatientAppointments(ctx context.Context, patientID uuid.UUID, from, to time.Time) ([]models.Appointment, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+appointmentColumns+` FROM appointments
		WHERE patient_id = $1 AND starts_at >= $2 AND starts_at < $3 ORDER BY starts_at`, patientID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Appointment
	for rows.Next() {
		a, err := scanAppointment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
