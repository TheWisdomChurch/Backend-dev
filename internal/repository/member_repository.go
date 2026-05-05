package repository

import (
	"fmt"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type MemberRepository interface {
	List(offset, limit int, active *bool) ([]models.Member, int64, error)
	ListAll(active *bool) ([]models.Member, error)
	GetByID(id string) (*models.Member, error)
	Create(member *models.Member) error
	Update(id string, updates map[string]interface{}) (*models.Member, error)
	Delete(id string) error

	BirthdayCountsByMonth(activeOnly bool) (map[int]int64, int64, error)
	ListByMonth(month int, activeOnly bool) ([]models.Member, error)
	ListByMonthDay(month, day int, activeOnly bool) ([]models.Member, error)
	Stats() (*models.MemberStatsResponse, error)
}

type memberRepository struct {
	db *database.Database
}

func NewMemberRepository(db *database.Database) MemberRepository {
	return &memberRepository{db: db}
}

func (r *memberRepository) List(offset, limit int, active *bool) ([]models.Member, int64, error) {
	var items []models.Member
	var total int64

	q := r.db.DB.Model(&models.Member{})
	if active != nil {
		q = q.Where("is_active = ?", *active)
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

func (r *memberRepository) ListAll(active *bool) ([]models.Member, error) {
	var items []models.Member
	q := r.db.DB.Model(&models.Member{})
	if active != nil {
		q = q.Where("is_active = ?", *active)
	}
	err := q.Order("updated_at DESC").Find(&items).Error
	return items, err
}

func (r *memberRepository) GetByID(id string) (*models.Member, error) {
	var member models.Member
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *memberRepository) Create(member *models.Member) error {
	return r.db.DB.Create(member).Error
}

func (r *memberRepository) Update(id string, updates map[string]interface{}) (*models.Member, error) {
	if err := r.db.DB.Model(&models.Member{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	var member models.Member
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *memberRepository) Delete(id string) error {
	return r.db.DB.Delete(&models.Member{}, "id = ?", id).Error
}

func (r *memberRepository) BirthdayCountsByMonth(activeOnly bool) (map[int]int64, int64, error) {
	type row struct {
		Month int
		Count int64
	}

	q := r.db.DB.Model(&models.Member{}).
		Select("birthday_month as month, COUNT(*) as count").
		Where("birthday_month IS NOT NULL")
	if activeOnly {
		q = q.Where("is_active = true")
	}

	var rows []row
	if err := q.Group("birthday_month").Scan(&rows).Error; err != nil {
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

func (r *memberRepository) ListByMonth(month int, activeOnly bool) ([]models.Member, error) {
	var items []models.Member
	q := r.db.DB.Where("birthday_month = ?", month)
	if activeOnly {
		q = q.Where("is_active = true")
	}
	err := q.Order("birthday_day ASC, last_name ASC").Find(&items).Error
	return items, err
}

func (r *memberRepository) ListByMonthDay(month, day int, activeOnly bool) ([]models.Member, error) {
	var items []models.Member
	q := r.db.DB.Where("birthday_month = ? AND birthday_day = ?", month, day)
	if activeOnly {
		q = q.Where("is_active = true")
	}
	err := q.Order("last_name ASC").Find(&items).Error
	return items, err
}

func (r *memberRepository) Stats() (*models.MemberStatsResponse, error) {
	var total int64
	if err := r.db.DB.Model(&models.Member{}).Count(&total).Error; err != nil {
		return nil, err
	}

	var active int64
	if err := r.db.DB.Model(&models.Member{}).Where("is_active = true").Count(&active).Error; err != nil {
		return nil, err
	}

	monthly, err := r.countCreatedByPeriod("month")
	if err != nil {
		return nil, err
	}
	yearly, err := r.countCreatedByPeriod("year")
	if err != nil {
		return nil, err
	}

	return &models.MemberStatsResponse{
		Total:         total,
		Active:        active,
		Inactive:      total - active,
		MonthlyGrowth: monthly,
		YearlyGrowth:  yearly,
	}, nil
}

func (r *memberRepository) countCreatedByPeriod(period string) ([]models.GrowthBucket, error) {
	switch period {
	case "month", "year":
	default:
		period = "month"
	}
	bucketExpr := fmt.Sprintf("date_trunc('%s', created_at)", period)

	var rows []models.GrowthBucket
	err := r.db.DB.Model(&models.Member{}).
		Select("to_char(" + bucketExpr + ", 'YYYY-MM-DD') as period, COUNT(*) as count").
		Group(bucketExpr).
		Order(bucketExpr + " ASC").
		Scan(&rows).Error
	return rows, err
}
