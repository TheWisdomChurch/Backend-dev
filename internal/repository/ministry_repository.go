package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
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

	AssignWorkforceMember(ctx context.Context, assignment *models.MinistryWorkforceMember) error
	UpdateWorkforceAssignment(ctx context.Context, ministryID, workforceMemberID string, updates map[string]interface{}) error
	RemoveWorkforceMember(ctx context.Context, ministryID, workforceMemberID string) error
	ListWorkforceMembers(ctx context.Context, ministryID string) ([]models.MinistryWorkforceMember, error)
	WorkforceMemberMinistries(ctx context.Context, workforceMemberID string) ([]models.Ministry, error)
	SyncDepartmentAssignment(ctx context.Context, workforceMemberID, department string) error
}

func (r *ministryRepository) AssignWorkforceMember(ctx context.Context, assignment *models.MinistryWorkforceMember) error {
	return r.db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.MinistryWorkforceMember
		err := tx.Unscoped().Where("ministry_id = ? AND workforce_member_id = ?", assignment.MinistryID, assignment.WorkforceMemberID).First(&existing).Error
		if err == nil {
			return tx.Unscoped().Model(&existing).Updates(map[string]interface{}{"role": assignment.Role, "title": assignment.Title, "deleted_at": nil, "updated_at": time.Now().UTC()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(assignment).Error
	})
}

func (r *ministryRepository) UpdateWorkforceAssignment(ctx context.Context, ministryID, workforceMemberID string, updates map[string]interface{}) error {
	result := r.db.DB.WithContext(ctx).Model(&models.MinistryWorkforceMember{}).
		Where("ministry_id = ? AND workforce_member_id = ? AND deleted_at IS NULL", ministryID, workforceMemberID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ministryRepository) RemoveWorkforceMember(ctx context.Context, ministryID, workforceMemberID string) error {
	return r.db.DB.WithContext(ctx).Where("ministry_id = ? AND workforce_member_id = ?", ministryID, workforceMemberID).Delete(&models.MinistryWorkforceMember{}).Error
}

func (r *ministryRepository) ListWorkforceMembers(ctx context.Context, ministryID string) ([]models.MinistryWorkforceMember, error) {
	var rows []models.MinistryWorkforceMember
	err := r.db.DB.WithContext(ctx).Preload("WorkforceMember").
		Where("ministry_id = ? AND deleted_at IS NULL", ministryID).
		Order("CASE role WHEN 'head' THEN 1 WHEN 'deputy_head' THEN 2 WHEN 'coordinator' THEN 3 ELSE 4 END, joined_at ASC").Find(&rows).Error
	return rows, err
}

func (r *ministryRepository) WorkforceMemberMinistries(ctx context.Context, workforceMemberID string) ([]models.Ministry, error) {
	var rows []models.Ministry
	err := r.db.DB.WithContext(ctx).
		Joins("JOIN ministry_workforce_members mwm ON mwm.ministry_id = ministries.id AND mwm.deleted_at IS NULL").
		Where("mwm.workforce_member_id = ? AND ministries.deleted_at IS NULL", workforceMemberID).
		Order("ministries.name ASC").Find(&rows).Error
	return rows, err
}

func (r *ministryRepository) SyncDepartmentAssignment(ctx context.Context, workforceMemberID, department string) error {
	department = strings.TrimSpace(department)
	if department == "" {
		return errors.New("department is required")
	}
	return r.db.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ministry models.Ministry
		err := tx.Where("deleted_at IS NULL AND lower(trim(name)) = lower(?)", department).First(&ministry).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ministry = models.Ministry{Name: department, Description: "Created from workforce department assignment.", Category: "department", IsActive: true}
			if err := tx.Create(&ministry).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if err := tx.Where("workforce_member_id = ? AND source = ? AND ministry_id <> ?", workforceMemberID, "department_sync", ministry.ID).Delete(&models.MinistryWorkforceMember{}).Error; err != nil {
			return err
		}
		var existing models.MinistryWorkforceMember
		err = tx.Unscoped().Where("ministry_id = ? AND workforce_member_id = ?", ministry.ID, workforceMemberID).First(&existing).Error
		if err == nil {
			updates := map[string]interface{}{"deleted_at": nil, "updated_at": time.Now().UTC()}
			if existing.Source == "department_sync" || existing.Source == "" {
				updates["source"] = "department_sync"
			}
			return tx.Unscoped().Model(&existing).Updates(updates).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return tx.Create(&models.MinistryWorkforceMember{MinistryID: ministry.ID, WorkforceMemberID: workforceMemberID, Role: models.MinistryRoleMember, Source: "department_sync"}).Error
	})
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
