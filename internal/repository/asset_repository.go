package repository

import (
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AssetRepository interface {
	Create(asset *models.Asset) error
	Update(asset *models.Asset) error
	GetByID(id string) (*models.Asset, error)
	List(offset, limit int, ownerType, ownerID string) ([]models.Asset, int64, error)
	SetStatus(id string, status models.AssetStatus) error
}

type assetRepository struct {
	db *database.Database
}

func NewAssetRepository(db *database.Database) AssetRepository {
	return &assetRepository{db: db}
}

func (r *assetRepository) Create(asset *models.Asset) error {
	return r.db.Create(asset).Error
}

func (r *assetRepository) Update(asset *models.Asset) error {
	return r.db.Save(asset).Error
}

func (r *assetRepository) GetByID(id string) (*models.Asset, error) {
	var a models.Asset
	if err := r.db.First(&a, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *assetRepository) List(offset, limit int, ownerType, ownerID string) ([]models.Asset, int64, error) {
	var items []models.Asset
	var total int64

	q := r.db.Model(&models.Asset{})
	if ownerType != "" {
		q = q.Where("owner_type = ?", ownerType)
	}
	if ownerID != "" {
		q = q.Where("owner_id = ?", ownerID)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *assetRepository) SetStatus(id string, status models.AssetStatus) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	res := r.db.Model(&models.Asset{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
