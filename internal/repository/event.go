package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type EventRepository struct {
	db *database.Database
}

func NewEventRepository(db *database.Database) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(e *models.Event) error {
	return r.db.Create(e).Error
}

func (r *EventRepository) GetByID(id string) (*models.Event, error) {
	var e models.Event
	if err := r.db.First(&e, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *EventRepository) Update(e *models.Event) error {
	return r.db.Save(e).Error
}

func (r *EventRepository) Delete(id string) error {
	return r.db.Delete(&models.Event{}, "id = ?", id).Error
}

func (r *EventRepository) List(offset, limit int) ([]models.Event, int64, error) {
	var items []models.Event
	var total int64

	if err := r.db.Model(&models.Event{}).Count(&total).Error; err != nil {
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

// ✅ NEW: save CDN url + object key for an event image
func (r *EventRepository) UpdateImage(id string, imageURL string, imageKey string) error {
	return r.db.Model(&models.Event{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"image_url": imageURL,
			"image_key": imageKey,
		}).Error
}
