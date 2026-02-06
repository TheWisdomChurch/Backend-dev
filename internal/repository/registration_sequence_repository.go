package repository

import "wisdomHouse-backend/internal/database"

type RegistrationSequenceRepository struct {
	db *database.Database
}

func NewRegistrationSequenceRepository(db *database.Database) *RegistrationSequenceRepository {
	return &RegistrationSequenceRepository{db: db}
}

func (r *RegistrationSequenceRepository) Next(prefix string) (int, error) {
	var next int
	err := r.db.Raw(`
		INSERT INTO registration_sequences (prefix, last_number, updated_at)
		VALUES (?, 1, now())
		ON CONFLICT (prefix)
		DO UPDATE SET last_number = registration_sequences.last_number + 1, updated_at = now()
		RETURNING last_number
	`, prefix).Scan(&next).Error
	return next, err
}
