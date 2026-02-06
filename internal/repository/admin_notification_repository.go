package repository

import (
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AdminNotificationRepository struct {
	db *database.Database
}

func NewAdminNotificationRepository(db *database.Database) *AdminNotificationRepository {
	return &AdminNotificationRepository{db: db}
}

func (r *AdminNotificationRepository) CreateMany(items []models.AdminNotification) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

func (r *AdminNotificationRepository) ListByUser(userID string, limit int) ([]models.AdminNotification, error) {
	var items []models.AdminNotification
	q := r.db.Where("user_id = ?", userID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&items).Error
	return items, err
}

func (r *AdminNotificationRepository) CountUnread(userID string) (int64, error) {
	var count int64
	err := r.db.Model(&models.AdminNotification{}).
		Where("user_id = ? AND is_read = false", userID).
		Count(&count).Error
	return count, err
}

func (r *AdminNotificationRepository) MarkRead(userID, id string, readAt time.Time) error {
	return r.db.Model(&models.AdminNotification{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{
			"is_read": true,
			"read_at": readAt,
		}).Error
}

func (r *AdminNotificationRepository) MarkAllRead(userID string, readAt time.Time) error {
	return r.db.Model(&models.AdminNotification{}).
		Where("user_id = ? AND is_read = false", userID).
		Updates(map[string]any{
			"is_read": true,
			"read_at": readAt,
		}).Error
}
