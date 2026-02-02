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

// ✅ NEW: Set event image CDN URL (stores into the existing `image` column)
func (r *EventRepository) SetImage(id string, imageURL string, imageKey *string) error {
	updates := map[string]any{
		"image": imageURL, // matches models.Event.Image
	}

	// Optional: only if you added `image_key` column in DB
	if imageKey != nil {
		updates["image_key"] = *imageKey
	}

	return r.db.Model(&models.Event{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// ✅ OPTIONAL: Set event banner CDN URL (stores into the existing `banner_image` column)
func (r *EventRepository) SetBannerImage(id string, bannerURL string, bannerKey *string) error {
	updates := map[string]any{
		"banner_image": bannerURL, // matches models.Event.BannerImage
	}

	// Optional: only if you added `banner_image_key` column in DB
	if bannerKey != nil {
		updates["banner_image_key"] = *bannerKey
	}

	return r.db.Model(&models.Event{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// SetStatus persists a status update; best-effort for auto-rollover.
func (r *EventRepository) SetStatus(id string, status models.EventStatus) error {
	return r.db.Model(&models.Event{}).
		Where("id = ?", id).
		Update("status", status).Error
}
