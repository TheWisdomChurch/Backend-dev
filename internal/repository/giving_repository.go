package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type GivingRepository interface {
	CreateCategory(ctx context.Context, cat *models.GivingCategory) error
	ListCategories(ctx context.Context) ([]models.GivingCategory, error)

	Create(ctx context.Context, tx *models.GivingTransaction) error
	FindByRef(ctx context.Context, ref string) (*models.GivingTransaction, error)
	UpdateStatus(ctx context.Context, id, status string) error
	List(ctx context.Context, filter GivingFilter, limit, offset int) ([]models.GivingTransaction, int64, error)
	MonthlySummary(ctx context.Context, year, month int, campusID *string) ([]models.GivingMonthlySummary, error)
}

type GivingFilter struct {
	CategoryID *string
	MemberID   *string
	CampusID   *string
	Status     *string
	From       *time.Time
	To         *time.Time
}

type givingRepository struct {
	db *database.Database
}

func NewGivingRepository(db *database.Database) GivingRepository {
	return &givingRepository{db: db}
}

func (r *givingRepository) CreateCategory(ctx context.Context, cat *models.GivingCategory) error {
	return r.db.DB.WithContext(ctx).Create(cat).Error
}

func (r *givingRepository) ListCategories(ctx context.Context) ([]models.GivingCategory, error) {
	var cats []models.GivingCategory
	err := r.db.DB.WithContext(ctx).
		Where("is_active = true").
		Order("name ASC").
		Find(&cats).Error
	return cats, err
}

func (r *givingRepository) Create(ctx context.Context, tx *models.GivingTransaction) error {
	return r.db.DB.WithContext(ctx).Create(tx).Error
}

func (r *givingRepository) FindByRef(ctx context.Context, ref string) (*models.GivingTransaction, error) {
	var tx models.GivingTransaction
	err := r.db.DB.WithContext(ctx).
		Preload("Category").
		Where("payment_ref = ?", ref).
		First(&tx).Error
	if err != nil {
		return nil, err
	}
	return &tx, nil
}

func (r *givingRepository) UpdateStatus(ctx context.Context, id, status string) error {
	return r.db.DB.WithContext(ctx).
		Model(&models.GivingTransaction{}).
		Where("id = ?", id).
		Update("status", status).Error
}

func (r *givingRepository) List(ctx context.Context, filter GivingFilter, limit, offset int) ([]models.GivingTransaction, int64, error) {
	q := r.db.DB.WithContext(ctx).Model(&models.GivingTransaction{}).Preload("Category")
	q = applyGivingFilter(q, filter)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var txs []models.GivingTransaction
	err := q.Order("given_at DESC").Limit(limit).Offset(offset).Find(&txs).Error
	return txs, total, err
}

func (r *givingRepository) MonthlySummary(ctx context.Context, year, month int, campusID *string) ([]models.GivingMonthlySummary, error) {
	q := r.db.DB.WithContext(ctx).
		Table("giving_transactions").
		Select("EXTRACT(YEAR FROM given_at)::int AS year, EXTRACT(MONTH FROM given_at)::int AS month, category_id, SUM(amount_kobo) AS total_kobo, COUNT(*) AS count").
		Where("status = 'success' AND deleted_at IS NULL")

	if year > 0 {
		q = q.Where("EXTRACT(YEAR FROM given_at) = ?", year)
	}
	if month > 0 {
		q = q.Where("EXTRACT(MONTH FROM given_at) = ?", month)
	}
	if campusID != nil {
		q = q.Where("campus_id = ?", *campusID)
	}

	var rows []models.GivingMonthlySummary
	err := q.Group("year, month, category_id").Order("year DESC, month DESC").Scan(&rows).Error
	return rows, err
}

func applyGivingFilter(q *gorm.DB, f GivingFilter) *gorm.DB {
	if f.CategoryID != nil {
		q = q.Where("category_id = ?", *f.CategoryID)
	}
	if f.MemberID != nil {
		q = q.Where("member_id = ?", *f.MemberID)
	}
	if f.CampusID != nil {
		q = q.Where("campus_id = ?", *f.CampusID)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if f.From != nil {
		q = q.Where("given_at >= ?", *f.From)
	}
	if f.To != nil {
		q = q.Where("given_at <= ?", *f.To)
	}
	return q
}
