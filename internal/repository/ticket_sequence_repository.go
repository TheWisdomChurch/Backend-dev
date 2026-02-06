package repository

import "wisdomHouse-backend/internal/database"

type TicketSequenceRepository struct {
	db *database.Database
}

func NewTicketSequenceRepository(db *database.Database) *TicketSequenceRepository {
	return &TicketSequenceRepository{db: db}
}

func (r *TicketSequenceRepository) Next(prefix string) (int, error) {
	var next int
	err := r.db.Raw(`
		INSERT INTO ticket_sequences (prefix, last_number, updated_at)
		VALUES (?, 1, now())
		ON CONFLICT (prefix)
		DO UPDATE SET last_number = ticket_sequences.last_number + 1, updated_at = now()
		RETURNING last_number
	`, prefix).Scan(&next).Error
	return next, err
}
