package repository

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type WorkforceRepository interface {
	List(offset, limit int, department, status string) ([]models.WorkforceMember, int64, error)
	GetByID(id string) (*models.WorkforceMember, error)
	FindByEmail(email string) (*models.WorkforceMember, error)
	Create(member *models.WorkforceMember) error
	Update(id string, updates map[string]interface{}) (*models.WorkforceMember, error)
	Delete(id string) error
	Stats() (*models.WorkforceStatsResponse, error)

	// Birthday helpers
	ListByMonth(month int, status string) ([]models.WorkforceMember, error)
	ListByMonthDay(month, day int, status string) ([]models.WorkforceMember, error)
	BirthdayCountsByMonth(status string) (map[int]int64, int64, error)
}

type workforceRepository struct {
	db *database.Database
}

func NewWorkforceRepository(db *database.Database) WorkforceRepository {
	return &workforceRepository{db: db}
}

func (r *workforceRepository) List(offset, limit int, department, status string) ([]models.WorkforceMember, int64, error) {
	var items []models.WorkforceMember
	var total int64

	q := r.db.DB.Model(&models.WorkforceMember{})
	if department != "" {
		q = q.Where("department = ?", department)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := q.Order("updated_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&items).Error
	return items, total, err
}

func (r *workforceRepository) Create(member *models.WorkforceMember) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(member).Error; err != nil {
			return err
		}
		return syncWorkforceDepartment(tx, member.ID, member.Department)
	})
}

func (r *workforceRepository) GetByID(id string) (*models.WorkforceMember, error) {
	var member models.WorkforceMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *workforceRepository) FindByEmail(email string) (*models.WorkforceMember, error) {
	var member models.WorkforceMember
	if err := r.db.DB.Where("LOWER(email) = LOWER(?)", email).
		Order("updated_at DESC").
		First(&member).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *workforceRepository) Update(id string, updates map[string]interface{}) (*models.WorkforceMember, error) {
	var member models.WorkforceMember
	if err := r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.WorkforceMember{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&member, "id = ?", id).Error; err != nil {
			return err
		}
		if _, changed := updates["department"]; changed {
			return syncWorkforceDepartment(tx, member.ID, member.Department)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &member, nil
}

func syncWorkforceDepartment(tx *gorm.DB, workforceMemberID, department string) error {
	department = strings.TrimSpace(department)
	if department == "" {
		return errors.New("department is required")
	}
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
		return tx.Unscoped().Model(&existing).Updates(map[string]interface{}{"deleted_at": nil, "updated_at": time.Now().UTC()}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&models.MinistryWorkforceMember{MinistryID: ministry.ID, WorkforceMemberID: workforceMemberID, Role: models.MinistryRoleMember, Source: "department_sync"}).Error
}

func (r *workforceRepository) Delete(id string) error {
	return r.db.DB.Delete(&models.WorkforceMember{}, "id = ?", id).Error
}

func (r *workforceRepository) Stats() (*models.WorkforceStatsResponse, error) {
	var total int64
	if err := r.db.DB.Model(&models.WorkforceMember{}).Count(&total).Error; err != nil {
		return nil, err
	}

	type row struct {
		Key   string
		Count int64
	}

	byStatus := map[string]int64{}
	var statusRows []row
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Select("status as key, COUNT(*) as count").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, r := range statusRows {
		byStatus[r.Key] = r.Count
	}

	byDepartment := map[string]int64{}
	var deptRows []row
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Select("department as key, COUNT(*) as count").
		Group("department").
		Scan(&deptRows).Error; err != nil {
		return nil, err
	}
	for _, r := range deptRows {
		byDepartment[r.Key] = r.Count
	}

	bySource := map[string]int64{}
	var sourceRows []row
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Select("source_channel as key, COUNT(*) as count").
		Group("source_channel").
		Scan(&sourceRows).Error; err != nil {
		return nil, err
	}
	for _, r := range sourceRows {
		bySource[r.Key] = r.Count
	}

	frontendByDepartment := map[string]int64{}
	var frontendDeptRows []row
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Where("source_channel LIKE ?", "frontend:%").
		Select("department as key, COUNT(*) as count").
		Group("department").
		Scan(&frontendDeptRows).Error; err != nil {
		return nil, err
	}
	for _, r := range frontendDeptRows {
		frontendByDepartment[r.Key] = r.Count
	}

	var buckets []models.WorkforceBucket
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Select("department, status, COUNT(*) as count").
		Group("department, status").
		Scan(&buckets).Error; err != nil {
		return nil, err
	}

	return &models.WorkforceStatsResponse{
		Total:                total,
		ByStatus:             byStatus,
		ByDepartment:         byDepartment,
		BySource:             bySource,
		FrontendByDepartment: frontendByDepartment,
		ByDeptAndStatus:      buckets,
	}, nil
}

func (r *workforceRepository) ListByMonth(month int, status string) ([]models.WorkforceMember, error) {
	var items []models.WorkforceMember
	q := r.db.DB.Where("birthday_month = ?", month)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("birthday_day ASC, last_name ASC").
		Find(&items).Error
	return items, err
}

func (r *workforceRepository) ListByMonthDay(month, day int, status string) ([]models.WorkforceMember, error) {
	var items []models.WorkforceMember
	q := r.db.DB.Where("birthday_month = ? AND birthday_day = ?", month, day)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("last_name ASC").
		Find(&items).Error
	return items, err
}

func (r *workforceRepository) BirthdayCountsByMonth(status string) (map[int]int64, int64, error) {
	type row struct {
		Month int
		Count int64
	}

	var rows []row
	q := r.db.DB.Model(&models.WorkforceMember{}).
		Select("birthday_month as month, COUNT(*) as count").
		Where("birthday_month IS NOT NULL")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Group("birthday_month").
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	counts := make(map[int]int64, 12)
	var total int64
	for _, r := range rows {
		if r.Month < 1 || r.Month > 12 {
			continue
		}
		counts[r.Month] = r.Count
		total += r.Count
	}

	return counts, total, nil
}
