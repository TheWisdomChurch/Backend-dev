package repository

import (
	"context"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type CellGroupRepository interface {
	Create(ctx context.Context, g *models.CellGroup) error
	Update(ctx context.Context, id string, updates map[string]interface{}) error
	FindByID(ctx context.Context, id string) (*models.CellGroup, error)
	List(ctx context.Context, campusID *string, activeOnly bool, limit, offset int) ([]models.CellGroup, int64, error)
	Delete(ctx context.Context, id string) error

	AddMember(ctx context.Context, m *models.CellGroupMember) (*models.CellGroupMember, error)
	RemoveMember(ctx context.Context, groupID, memberID string) error
	ListMembers(ctx context.Context, groupID string) ([]models.CellGroupMember, error)
	MemberGroups(ctx context.Context, memberID string) ([]models.CellGroup, error)

	CreateMeeting(ctx context.Context, m *models.CellGroupMeeting) error
	ListMeetings(ctx context.Context, groupID string, limit, offset int) ([]models.CellGroupMeeting, int64, error)
}

type cellGroupRepository struct {
	db *database.Database
}

func NewCellGroupRepository(db *database.Database) CellGroupRepository {
	return &cellGroupRepository{db: db}
}

func (r *cellGroupRepository) Create(ctx context.Context, g *models.CellGroup) error {
	return r.db.DB.WithContext(ctx).Create(g).Error
}

func (r *cellGroupRepository) Update(ctx context.Context, id string, updates map[string]interface{}) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.CellGroup{}).Where("id = ?", id).Updates(updates).Error
}

func (r *cellGroupRepository) FindByID(ctx context.Context, id string) (*models.CellGroup, error) {
	var g models.CellGroup
	err := r.db.DB.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).First(&g).Error
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *cellGroupRepository) List(ctx context.Context, campusID *string, activeOnly bool, limit, offset int) ([]models.CellGroup, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.CellGroup{}).Where("deleted_at IS NULL")
	if campusID != nil {
		q = q.Where("campus_id = ?", *campusID)
	}
	if activeOnly {
		q = q.Where("is_active = true")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.CellGroup
	err := q.Order("name ASC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}

func (r *cellGroupRepository) Delete(ctx context.Context, id string) error {
	return r.db.DB.WithContext(ctx).Delete(&models.CellGroup{}, "id = ?", id).Error
}

func (r *cellGroupRepository) AddMember(ctx context.Context, m *models.CellGroupMember) (*models.CellGroupMember, error) {
	if err := r.db.DB.WithContext(ctx).Create(m).Error; err != nil {
		return nil, err
	}
	return m, nil
}

func (r *cellGroupRepository) RemoveMember(ctx context.Context, groupID, memberID string) error {
	return r.db.DB.WithContext(ctx).
		Where("group_id = ? AND member_id = ?", groupID, memberID).
		Delete(&models.CellGroupMember{}).Error
}

func (r *cellGroupRepository) ListMembers(ctx context.Context, groupID string) ([]models.CellGroupMember, error) {
	var rows []models.CellGroupMember
	err := r.db.DB.WithContext(ctx).
		Where("group_id = ? AND deleted_at IS NULL", groupID).
		Order("joined_at ASC").Find(&rows).Error
	return rows, err
}

func (r *cellGroupRepository) MemberGroups(ctx context.Context, memberID string) ([]models.CellGroup, error) {
	var rows []models.CellGroup
	err := r.db.DB.WithContext(ctx).
		Joins("JOIN cell_group_members m ON m.group_id = cell_groups.id AND m.deleted_at IS NULL").
		Where("m.member_id = ? AND cell_groups.deleted_at IS NULL", memberID).
		Find(&rows).Error
	return rows, err
}

func (r *cellGroupRepository) CreateMeeting(ctx context.Context, m *models.CellGroupMeeting) error {
	return r.db.DB.WithContext(ctx).Create(m).Error
}

func (r *cellGroupRepository) ListMeetings(ctx context.Context, groupID string, limit, offset int) ([]models.CellGroupMeeting, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.CellGroupMeeting{}).
		Where("group_id = ? AND deleted_at IS NULL", groupID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.CellGroupMeeting
	err := q.Order("date DESC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, total, err
}
