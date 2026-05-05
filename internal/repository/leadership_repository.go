package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type LeadershipRepository interface {
	List(offset, limit int, role, status string) ([]models.LeadershipMember, int64, error)
	ListApproved(role string) ([]models.LeadershipMember, error)
	ListByBirthdayMonth(month int, status string) ([]models.LeadershipMember, error)
	ListByBirthdayMonthDay(month, day int, status string) ([]models.LeadershipMember, error)
	BirthdayCountsByMonth(status string) (map[int]int64, int64, error)
	ListByAnniversaryMonth(month int, status string) ([]models.LeadershipMember, error)
	ListByAnniversaryMonthDay(month, day int, status string) ([]models.LeadershipMember, error)
	AnniversaryCountsByMonth(status string) (map[int]int64, int64, error)
	GetByID(id string) (*models.LeadershipMember, error)
	GetByEmail(email string) (*models.LeadershipMember, error)
	Create(member *models.LeadershipMember) error
	Update(id string, updates map[string]interface{}) (*models.LeadershipMember, error)
	Delete(id string) error
}

type leadershipRepository struct {
	db *database.Database
}

func NewLeadershipRepository(db *database.Database) LeadershipRepository {
	return &leadershipRepository{db: db}
}

func (r *leadershipRepository) List(offset, limit int, role, status string) ([]models.LeadershipMember, int64, error) {
	var items []models.LeadershipMember
	var total int64

	q := r.db.DB.Model(&models.LeadershipMember{})
	if role != "" {
		q = q.Where("role = ?", role)
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

func (r *leadershipRepository) ListApproved(role string) ([]models.LeadershipMember, error) {
	var items []models.LeadershipMember
	q := r.db.DB.Model(&models.LeadershipMember{}).
		Where("status = ?", models.LeadershipStatusApproved)
	if role != "" {
		q = q.Where("role = ?", role)
	}
	err := q.Order("role ASC, last_name ASC, first_name ASC").
		Find(&items).Error
	return items, err
}

func (r *leadershipRepository) ListByBirthdayMonth(month int, status string) ([]models.LeadershipMember, error) {
	var items []models.LeadershipMember
	q := r.db.DB.Where("birthday_month = ?", month)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("birthday_day ASC, last_name ASC, first_name ASC").Find(&items).Error
	return items, err
}

func (r *leadershipRepository) ListByBirthdayMonthDay(month, day int, status string) ([]models.LeadershipMember, error) {
	var items []models.LeadershipMember
	q := r.db.DB.Where("birthday_month = ? AND birthday_day = ?", month, day)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("last_name ASC, first_name ASC").Find(&items).Error
	return items, err
}

func (r *leadershipRepository) BirthdayCountsByMonth(status string) (map[int]int64, int64, error) {
	type row struct {
		Month int
		Count int64
	}

	counts := make(map[int]int64, 12)
	var rows []row
	var total int64

	q := r.db.DB.Model(&models.LeadershipMember{}).
		Select("birthday_month as month, COUNT(*) as count").
		Where("birthday_month IS NOT NULL")
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Group("birthday_month").Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, r := range rows {
		counts[r.Month] = r.Count
		total += r.Count
	}
	return counts, total, nil
}

func (r *leadershipRepository) ListByAnniversaryMonth(month int, status string) ([]models.LeadershipMember, error) {
	var items []models.LeadershipMember
	q := r.db.DB.Where("anniversary_month = ?", month)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("anniversary_day ASC, last_name ASC, first_name ASC").Find(&items).Error
	return items, err
}

func (r *leadershipRepository) ListByAnniversaryMonthDay(month, day int, status string) ([]models.LeadershipMember, error) {
	var items []models.LeadershipMember
	q := r.db.DB.Where("anniversary_month = ? AND anniversary_day = ?", month, day)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Order("last_name ASC, first_name ASC").Find(&items).Error
	return items, err
}

func (r *leadershipRepository) AnniversaryCountsByMonth(status string) (map[int]int64, int64, error) {
	type row struct {
		Month int
		Count int64
	}

	counts := make(map[int]int64, 12)
	var rows []row
	var total int64

	q := r.db.DB.Model(&models.LeadershipMember{}).
		Select("anniversary_month as month, COUNT(*) as count").
		Where("anniversary_month IS NOT NULL")
	if status != "" {
		q = q.Where("status = ?", status)
	}

	if err := q.Group("anniversary_month").Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	for _, r := range rows {
		counts[r.Month] = r.Count
		total += r.Count
	}
	return counts, total, nil
}

func (r *leadershipRepository) GetByID(id string) (*models.LeadershipMember, error) {
	var member models.LeadershipMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *leadershipRepository) GetByEmail(email string) (*models.LeadershipMember, error) {
	var member models.LeadershipMember
	if err := r.db.DB.First(&member, "LOWER(email) = LOWER(?)", email).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *leadershipRepository) Create(member *models.LeadershipMember) error {
	return r.db.DB.Create(member).Error
}

func (r *leadershipRepository) Update(id string, updates map[string]interface{}) (*models.LeadershipMember, error) {
	if err := r.db.DB.Model(&models.LeadershipMember{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	var member models.LeadershipMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *leadershipRepository) Delete(id string) error {
	return r.db.DB.Delete(&models.LeadershipMember{}, "id = ?", id).Error
}
