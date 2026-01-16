// internal/repository/user_repository.go
package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

// UserRepository interface
type UserRepository interface {
	Create(user *models.User) error
	FindByEmail(email string) (*models.User, error)
	FindByID(id string) (*models.User, error)
	FindAll() ([]models.User, error)
	Update(user *models.User) error
	Delete(id string) error
	GetTotalCount() (int64, error)
}

// User repository implementation
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *database.Database) UserRepository {
	return &userRepository{db: db.DB}
}

func (r *userRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) FindByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByID(id string) (*models.User, error) {
	var user models.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindAll() ([]models.User, error) {
	var users []models.User
	err := r.db.Find(&users).Error
	return users, err
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) Delete(id string) error {
	return r.db.Delete(&models.User{}, "id = ?", id).Error
}

func (r *userRepository) GetTotalCount() (int64, error) {
	var count int64
	err := r.db.Model(&models.User{}).Count(&count).Error
	return count, err
}