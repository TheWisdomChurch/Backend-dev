package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// EngagementRepository handles persistence for pastoral care requests,
// giving intents, and contact messages.
type EngagementRepository interface {
	CreatePastoralCareRequest(ctx context.Context, req *models.PastoralCareRequest) error
	ListPastoralCareRequests(ctx context.Context, offset, limit int) ([]models.PastoralCareRequest, int64, error)

	CreateGivingIntent(ctx context.Context, intent *models.GivingIntent) error
	ListGivingIntents(ctx context.Context, offset, limit int) ([]models.GivingIntent, int64, error)

	CreateContactMessage(ctx context.Context, msg *models.ContactMessage) error
	ListContactMessages(ctx context.Context, offset, limit int) ([]models.ContactMessage, int64, error)
}

type engagementRepository struct {
	db *database.Database
}

func NewEngagementRepository(db *database.Database) EngagementRepository {
	return &engagementRepository{db: db}
}

func (r *engagementRepository) CreatePastoralCareRequest(ctx context.Context, req *models.PastoralCareRequest) error {
	return r.db.WithContext(ctx).Create(req).Error
}

func (r *engagementRepository) ListPastoralCareRequests(ctx context.Context, offset, limit int) ([]models.PastoralCareRequest, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.PastoralCareRequest{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.PastoralCareRequest
	if err := r.db.WithContext(ctx).Model(&models.PastoralCareRequest{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) CreateGivingIntent(ctx context.Context, intent *models.GivingIntent) error {
	return r.db.WithContext(ctx).Create(intent).Error
}

func (r *engagementRepository) ListGivingIntents(ctx context.Context, offset, limit int) ([]models.GivingIntent, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.GivingIntent{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.GivingIntent
	if err := r.db.WithContext(ctx).Model(&models.GivingIntent{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *engagementRepository) CreateContactMessage(ctx context.Context, msg *models.ContactMessage) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *engagementRepository) ListContactMessages(ctx context.Context, offset, limit int) ([]models.ContactMessage, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.ContactMessage{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []models.ContactMessage
	if err := r.db.WithContext(ctx).Model(&models.ContactMessage{}).
		Order("created_at DESC").Offset(offset).Limit(limit).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
