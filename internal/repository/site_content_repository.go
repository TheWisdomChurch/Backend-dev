package repository

import (
	"context"

	"gorm.io/gorm/clause"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// SiteContentRepository handles key-value CMS content persistence.
type SiteContentRepository interface {
	GetByKey(ctx context.Context, key string) (*models.SiteContent, error)
	Upsert(ctx context.Context, row *models.SiteContent) error
}

type siteContentRepository struct {
	db *database.Database
}

func NewSiteContentRepository(db *database.Database) SiteContentRepository {
	return &siteContentRepository{db: db}
}

func (r *siteContentRepository) GetByKey(ctx context.Context, key string) (*models.SiteContent, error) {
	var row models.SiteContent
	if err := r.db.WithContext(ctx).Where("key = ?", key).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *siteContentRepository) Upsert(ctx context.Context, row *models.SiteContent) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_by", "updated_at"}),
	}).Create(row).Error
}
