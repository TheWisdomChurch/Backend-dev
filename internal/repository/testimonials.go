// internal/repository/testimonial_repository.go
package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type TestimonialRepository interface {
	Create(testimonial *models.Testimonial) error
	GetAll(approved bool) ([]models.Testimonial, error)
	GetByID(id uuid.UUID) (*models.Testimonial, error)
	Update(testimonial *models.Testimonial) error
	Delete(id uuid.UUID) error
	GetPaginated(page, limit int, approved bool) ([]models.Testimonial, int64, error)
	// Add these for admin service
	GetTotalCount() (int64, error)
	GetPendingCount() (int64, error)
	FindByApprovalStatus(approved bool) ([]models.Testimonial, error)
}

type testimonialRepository struct {
	db *database.Database
}

func NewTestimonialRepository(db *database.Database) TestimonialRepository {
	return &testimonialRepository{db: db}
}

func (r *testimonialRepository) Create(testimonial *models.Testimonial) error {
	return r.db.DB.Create(testimonial).Error
}

func (r *testimonialRepository) GetAll(approved bool) ([]models.Testimonial, error) {
	var testimonials []models.Testimonial
	query := r.db.DB.Order("created_at DESC")

	if approved {
		query = query.Where("is_approved = ?", true)
	}

	err := query.Find(&testimonials).Error
	return testimonials, err
}

func (r *testimonialRepository) GetByID(id uuid.UUID) (*models.Testimonial, error) {
	var testimonial models.Testimonial
	err := r.db.DB.Where("id = ?", id).First(&testimonial).Error
	if err != nil {
		return nil, err
	}
	return &testimonial, nil
}

func (r *testimonialRepository) Update(testimonial *models.Testimonial) error {
	return r.db.DB.Save(testimonial).Error
}

func (r *testimonialRepository) Delete(id uuid.UUID) error {
	result := r.db.DB.Unscoped().Delete(&models.Testimonial{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *testimonialRepository) GetPaginated(page, limit int, approved bool) ([]models.Testimonial, int64, error) {
	var testimonials []models.Testimonial
	var total int64

	query := r.db.DB.Model(&models.Testimonial{})

	if approved {
		query = query.Where("is_approved = ?", true)
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated records
	offset := (page - 1) * limit
	err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&testimonials).Error

	return testimonials, total, err
}

// New methods for admin service
func (r *testimonialRepository) GetTotalCount() (int64, error) {
	var count int64
	err := r.db.DB.Model(&models.Testimonial{}).Count(&count).Error
	return count, err
}

func (r *testimonialRepository) GetPendingCount() (int64, error) {
	var count int64
	err := r.db.DB.Model(&models.Testimonial{}).Where("is_approved = ?", false).Count(&count).Error
	return count, err
}

func (r *testimonialRepository) FindByApprovalStatus(approved bool) ([]models.Testimonial, error) {
	var testimonials []models.Testimonial
	err := r.db.DB.Where("is_approved = ?", approved).Find(&testimonials).Error
	return testimonials, err
}
