package repository

import (
	"context"
	"time"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type ApprovalRequestRepository struct {
	db *database.Database
}

func (r *ApprovalRequestRepository) Count(ctx context.Context, statuses []models.ApprovalRequestStatus) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).Model(&models.ApprovalRequest{})
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	err := q.Count(&count).Error
	return count, err
}

func NewApprovalRequestRepository(db *database.Database) *ApprovalRequestRepository {
	return &ApprovalRequestRepository{db: db}
}

func (r *ApprovalRequestRepository) Create(req *models.ApprovalRequest) error {
	return r.db.Create(req).Error
}

func (r *ApprovalRequestRepository) FindByID(id string) (*models.ApprovalRequest, error) {
	var item models.ApprovalRequest
	if err := r.db.First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ApprovalRequestRepository) Update(req *models.ApprovalRequest) error {
	return r.db.Save(req).Error
}

func (r *ApprovalRequestRepository) FindByEntity(t models.ApprovalRequestType, entityID string) (*models.ApprovalRequest, error) {
	var item models.ApprovalRequest
	err := r.db.Where("type = ? AND entity_id = ?", t, entityID).Order("created_at DESC").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *ApprovalRequestRepository) List(
	types []models.ApprovalRequestType,
	statuses []models.ApprovalRequestStatus,
	start *time.Time,
	end *time.Time,
	limit int,
) ([]models.ApprovalRequest, error) {
	var items []models.ApprovalRequest
	q := r.db.Model(&models.ApprovalRequest{})
	if len(types) > 0 {
		q = q.Where("type IN ?", types)
	}
	if len(statuses) > 0 {
		q = q.Where("status IN ?", statuses)
	}
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Order("created_at DESC").Find(&items).Error
	return items, err
}

type ApprovalDailyCount struct {
	Day   time.Time `json:"day"`
	Count int64     `json:"count"`
}

func (r *ApprovalRequestRepository) CountCreatedByDay(start, end *time.Time) ([]ApprovalDailyCount, error) {
	var rows []ApprovalDailyCount
	q := r.db.Model(&models.ApprovalRequest{}).
		Select("date_trunc('day', created_at) as day, COUNT(*) as count")
	if start != nil {
		q = q.Where("created_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("created_at <= ?", *end)
	}
	if err := q.Group("day").Order("day ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *ApprovalRequestRepository) CountApprovedByDay(start, end *time.Time) ([]ApprovalDailyCount, error) {
	var rows []ApprovalDailyCount
	q := r.db.Model(&models.ApprovalRequest{}).
		Where("approved_at IS NOT NULL").
		Select("date_trunc('day', approved_at) as day, COUNT(*) as count")
	if start != nil {
		q = q.Where("approved_at >= ?", *start)
	}
	if end != nil {
		q = q.Where("approved_at <= ?", *end)
	}
	if err := q.Group("day").Order("day ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
