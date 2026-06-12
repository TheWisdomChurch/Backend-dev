package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type PrayerRequestRepository interface {
	Create(ctx context.Context, r *models.PrayerRequest) error
	FindByID(ctx context.Context, id string) (*models.PrayerRequest, error)
	List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequest, int64, error)
	UpdateStatus(ctx context.Context, id, status string) error
	AssignTo(ctx context.Context, id, userID string) error
	AddNotes(ctx context.Context, id string, notesEnc string) error
	Delete(ctx context.Context, id string) error
}

type prayerRequestRepository struct {
	db *database.Database
}

func NewPrayerRequestRepository(db *database.Database) PrayerRequestRepository {
	return &prayerRequestRepository{db: db}
}

func (r *prayerRequestRepository) Create(ctx context.Context, req *models.PrayerRequest) error {
	return r.db.DB.WithContext(ctx).Create(req).Error
}

func (r *prayerRequestRepository) FindByID(ctx context.Context, id string) (*models.PrayerRequest, error) {
	var req models.PrayerRequest
	err := r.db.DB.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&req).Error
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *prayerRequestRepository) List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequest, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.PrayerRequest{}).Where("deleted_at IS NULL")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if category != "" {
		q = q.Where("category = ?", category)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.PrayerRequest
	err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *prayerRequestRepository) UpdateStatus(ctx context.Context, id, status string) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *prayerRequestRepository) AssignTo(ctx context.Context, id, userID string) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ?", id).
		Update("assigned_to", userID).Error
}

func (r *prayerRequestRepository) AddNotes(ctx context.Context, id, notesEnc string) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ?", id).
		Update("notes_enc", notesEnc).Error
}

func (r *prayerRequestRepository) Delete(ctx context.Context, id string) error {
	return r.db.DB.WithContext(ctx).Delete(&models.PrayerRequest{}, "id = ?", id).Error
}
