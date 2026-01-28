package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type ReelRepository struct {
	db *database.Database
}

func NewReelRepository(db *database.Database) *ReelRepository {
	return &ReelRepository{db: db}
}

func (r *ReelRepository) Create(item *models.Reel) error {
	return r.db.Create(item).Error
}

func (r *ReelRepository) Delete(id string) error {
	return r.db.Delete(&models.Reel{}, "id = ?", id).Error
}

func (r *ReelRepository) List(offset, limit int) ([]models.Reel, int64, error) {
	var items []models.Reel
	var total int64

	if err := r.db.Model(&models.Reel{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.
		Order("created_at desc").
		Offset(offset).
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
