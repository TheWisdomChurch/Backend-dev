package repository

import (
	"strings"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type SubscriberRepository struct {
	db *database.Database
}

func NewSubscriberRepository(db *database.Database) *SubscriberRepository {
	return &SubscriberRepository{db: db}
}

func (r *SubscriberRepository) Create(sub *models.Subscriber) error {
	return r.db.Create(sub).Error
}

func (r *SubscriberRepository) Update(sub *models.Subscriber) error {
	return r.db.Save(sub).Error
}

func (r *SubscriberRepository) GetByEmail(email string) (*models.Subscriber, error) {
	var sub models.Subscriber
	normalized := strings.ToLower(strings.TrimSpace(email))
	if err := r.db.First(&sub, "email = ?", normalized).Error; err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *SubscriberRepository) List(offset, limit int) ([]models.Subscriber, int64, error) {
	var items []models.Subscriber
	var total int64

	if err := r.db.Model(&models.Subscriber{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := r.db.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *SubscriberRepository) ListActive() ([]models.Subscriber, error) {
	var items []models.Subscriber
	if err := r.db.Where("status = ?", models.SubscriberStatusActive).
		Order("created_at desc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
