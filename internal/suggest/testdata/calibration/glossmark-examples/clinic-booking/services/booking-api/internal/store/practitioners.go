package store

import (
	"context"

	"github.com/larkspur-clinics/booking-api/internal/models"
)

func (db *DB) ListPractitioners(ctx context.Context, specialty string) ([]models.Practitioner, error) {
	rows, err := db.pool.Query(ctx, `SELECT p.id, p.display_name, p.specialty, p.accepting_new_patients,
			array_agg(pc.clinic_id) FROM practitioners p JOIN practitioner_clinics pc ON pc.practitioner_id = p.id
		WHERE ($1 = '' OR p.specialty = $1) GROUP BY p.id ORDER BY p.display_name`, specialty)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Practitioner
	for rows.Next() {
		var p models.Practitioner
		if err := rows.Scan(&p.ID, &p.DisplayName, &p.Specialty, &p.Accepting, &p.ClinicIDs); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
