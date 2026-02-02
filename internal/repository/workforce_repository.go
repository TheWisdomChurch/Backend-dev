package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type WorkforceRepository interface {
	List(offset, limit int, department, status string) ([]models.WorkforceMember, int64, error)
	GetByID(id string) (*models.WorkforceMember, error)
	Create(member *models.WorkforceMember) error
	Update(id string, updates map[string]interface{}) (*models.WorkforceMember, error)
	Stats() (*models.WorkforceStatsResponse, error)

	// Birthday helpers
	ListByMonth(month int) ([]models.WorkforceMember, error)
	ListByMonthDay(month, day int) ([]models.WorkforceMember, error)
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
	return r.db.DB.Create(member).Error
}

func (r *workforceRepository) GetByID(id string) (*models.WorkforceMember, error) {
	var member models.WorkforceMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *workforceRepository) Update(id string, updates map[string]interface{}) (*models.WorkforceMember, error) {
	if err := r.db.DB.Model(&models.WorkforceMember{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	var member models.WorkforceMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
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
	_ = r.db.DB.Model(&models.WorkforceMember{}).
		Select("status as key, COUNT(*) as count").
		Group("status").
		Scan(&statusRows).Error
	for _, r := range statusRows {
		byStatus[r.Key] = r.Count
	}

	byDepartment := map[string]int64{}
	var deptRows []row
	_ = r.db.DB.Model(&models.WorkforceMember{}).
		Select("department as key, COUNT(*) as count").
		Group("department").
		Scan(&deptRows).Error
	for _, r := range deptRows {
		byDepartment[r.Key] = r.Count
	}

	var buckets []models.WorkforceBucket
	_ = r.db.DB.Model(&models.WorkforceMember{}).
		Select("department, status, COUNT(*) as count").
		Group("department, status").
		Scan(&buckets).Error

	return &models.WorkforceStatsResponse{
		Total:           total,
		ByStatus:        byStatus,
		ByDepartment:    byDepartment,
		ByDeptAndStatus: buckets,
	}, nil
}

func (r *workforceRepository) ListByMonth(month int) ([]models.WorkforceMember, error) {
	var items []models.WorkforceMember
	err := r.db.DB.
		Where("birthday_month = ?", month).
		Order("birthday_day ASC, last_name ASC").
		Find(&items).Error
	return items, err
}

func (r *workforceRepository) ListByMonthDay(month, day int) ([]models.WorkforceMember, error) {
	var items []models.WorkforceMember
	err := r.db.DB.
		Where("birthday_month = ? AND birthday_day = ?", month, day).
		Order("last_name ASC").
		Find(&items).Error
	return items, err
}
