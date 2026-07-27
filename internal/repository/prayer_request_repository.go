package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type PrayerRequestRepository interface {
	Create(ctx context.Context, r *models.PrayerRequest) error
	FindByID(ctx context.Context, id string) (*models.PrayerRequest, error)
	List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequestSummary, int64, error)
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

func (r *prayerRequestRepository) List(ctx context.Context, status, category string, limit, offset int) ([]models.PrayerRequestSummary, int64, error) {
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
	var rows []models.PrayerRequestSummary
	err := q.Select("id, category, is_anonymous, status, assigned_to, created_at, updated_at").
		Order("created_at DESC").Limit(limit).Offset(offset).Scan(&rows).Error
	return rows, total, err
}

func (r *prayerRequestRepository) UpdateStatus(ctx context.Context, id, status string) error {
	result := r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("status", status)
	return prayerMutationError(result)
}

func (r *prayerRequestRepository) AssignTo(ctx context.Context, id, userID string) error {
	result := r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("assigned_to", userID)
	return prayerMutationError(result)
}

func (r *prayerRequestRepository) AddNotes(ctx context.Context, id, notesEnc string) error {
	result := r.db.DB.WithContext(ctx).
		Model(&models.PrayerRequest{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("notes_enc", notesEnc)
	return prayerMutationError(result)
}

func (r *prayerRequestRepository) Delete(ctx context.Context, id string) error {
	return prayerMutationError(r.db.DB.WithContext(ctx).Delete(&models.PrayerRequest{}, "id = ?", id))
}

func prayerMutationError(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("prayer request not found")
	}
	return nil
}
