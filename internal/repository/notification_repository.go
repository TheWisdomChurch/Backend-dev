package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type NotificationRepository struct {
	db *database.Database
}

func NewNotificationRepository(db *database.Database) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Create(notification *models.Notification) error {
	return r.db.Create(notification).Error
}

func (r *NotificationRepository) CreateDelivery(delivery *models.NotificationDelivery) error {
	return r.db.Create(delivery).Error
}

func (r *NotificationRepository) UpdateDelivery(delivery *models.NotificationDelivery) error {
	return r.db.Save(delivery).Error
}
