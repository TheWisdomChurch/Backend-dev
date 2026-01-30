package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

type SecurityEventRepository interface {
	Create(event *models.SecurityEvent) error
	ListByUser(userID string, limit int) ([]models.SecurityEvent, error)
}

type securityEventRepository struct {
	db *gorm.DB
}

func NewSecurityEventRepository(db *database.Database) SecurityEventRepository {
	return &securityEventRepository{db: db.DB}
}

func (r *securityEventRepository) Create(event *models.SecurityEvent) error {
	return r.db.Create(event).Error
}

func (r *securityEventRepository) ListByUser(userID string, limit int) ([]models.SecurityEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	var events []models.SecurityEvent
	err := r.db.Where("user_id = ?", userID).
		Order("created_at desc").
		Limit(limit).
		Find(&events).Error
	return events, err
}
