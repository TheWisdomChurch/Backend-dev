package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type MinistryRepository interface {
	Create(ctx context.Context, m *models.Ministry) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	FindByID(ctx context.Context, id string) (*models.Ministry, error)
	List(ctx context.Context, campusID, category *string, activeOnly bool, limit, offset int) ([]models.Ministry, int64, error)
	Delete(ctx context.Context, id string) error

	AddMember(ctx context.Context, m *models.MinistryMember) error
	RemoveMember(ctx context.Context, ministryID, memberID string) error
	ListMembers(ctx context.Context, ministryID string) ([]models.MinistryMember, error)
	MemberMinistries(ctx context.Context, memberID string) ([]models.Ministry, error)
}

type ministryRepository struct {
	db *database.Database
}

func NewMinistryRepository(db *database.Database) MinistryRepository {
	return &ministryRepository{db: db}
}

func (r *ministryRepository) Create(ctx context.Context, m *models.Ministry) error {
	return r.db.DB.WithContext(ctx).Create(m).Error
}

func (r *ministryRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.Ministry{}).Where("id = ?", id).Updates(updates).Error
}

func (r *ministryRepository) FindByID(ctx context.Context, id string) (*models.Ministry, error) {
	var m models.Ministry
	err := r.db.DB.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ministryRepository) List(ctx context.Context, campusID, category *string, activeOnly bool, limit, offset int) ([]models.Ministry, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.Ministry{}).Where("deleted_at IS NULL")
	if campusID != nil {
		q = q.Where("campus_id = ?", *campusID)
	}
	if category != nil {
		q = q.Where("category = ?", *category)
	}
	if activeOnly {
		q = q.Where("is_active = true")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.Ministry
	err := q.Order("name ASC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *ministryRepository) Delete(ctx context.Context, id string) error {
	return r.db.DB.WithContext(ctx).Delete(&models.Ministry{}, "id = ?", id).Error
}

func (r *ministryRepository) AddMember(ctx context.Context, m *models.MinistryMember) error {
	return r.db.DB.WithContext(ctx).Create(m).Error
}

func (r *ministryRepository) RemoveMember(ctx context.Context, ministryID, memberID string) error {
	return r.db.DB.WithContext(ctx).
		Where("ministry_id = ? AND member_id = ?", ministryID, memberID).
		Delete(&models.MinistryMember{}).Error
}

func (r *ministryRepository) ListMembers(ctx context.Context, ministryID string) ([]models.MinistryMember, error) {
	var rows []models.MinistryMember
	err := r.db.DB.WithContext(ctx).
		Where("ministry_id = ? AND deleted_at IS NULL", ministryID).
		Order("joined_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ministryRepository) MemberMinistries(ctx context.Context, memberID string) ([]models.Ministry, error) {
	var rows []models.Ministry
	err := r.db.DB.WithContext(ctx).
		Joins("JOIN ministry_members mm ON mm.ministry_id = ministries.id AND mm.deleted_at IS NULL").
		Where("mm.member_id = ? AND ministries.deleted_at IS NULL", memberID).
		Find(&rows).Error
	return rows, err
}
