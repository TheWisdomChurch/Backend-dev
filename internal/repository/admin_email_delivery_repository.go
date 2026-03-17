package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AdminEmailDeliveryRepository interface {
	Create(item *models.AdminEmailDelivery) error
	List(offset, limit int) ([]models.AdminEmailDelivery, int64, error)
}

type adminEmailDeliveryRepository struct {
	db *database.Database
}

func NewAdminEmailDeliveryRepository(db *database.Database) AdminEmailDeliveryRepository {
	return &adminEmailDeliveryRepository{db: db}
}

func (r *adminEmailDeliveryRepository) Create(item *models.AdminEmailDelivery) error {
	return r.db.Create(item).Error
}

func (r *adminEmailDeliveryRepository) List(offset, limit int) ([]models.AdminEmailDelivery, int64, error) {
	var items []models.AdminEmailDelivery
	var total int64

	q := r.db.Model(&models.AdminEmailDelivery{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}
