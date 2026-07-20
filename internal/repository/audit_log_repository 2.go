package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type AuditLogRepository interface {
	Create(ctx context.Context, entry *models.AuditLog) error
	List(ctx context.Context, page, limit int, scope string) ([]models.AuditLog, int64, error)
	Recent(ctx context.Context, limit int) ([]models.AuditLog, error)
}

type auditLogRepository struct {
	db *database.Database
}

func NewAuditLogRepository(db *database.Database) AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Create(ctx context.Context, entry *models.AuditLog) error {
	return r.db.DB.WithContext(ctx).Create(entry).Error
}

func (r *auditLogRepository) List(ctx context.Context, page, limit int, scope string) ([]models.AuditLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	q := r.db.DB.WithContext(ctx).Model(&models.AuditLog{})
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []models.AuditLog
	if err := q.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *auditLogRepository) Recent(ctx context.Context, limit int) ([]models.AuditLog, error) {
	if limit < 1 || limit > 100 {
		limit = 10
	}

	var logs []models.AuditLog
	if err := r.db.DB.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		return nil, err
	}

	return logs, nil
}
