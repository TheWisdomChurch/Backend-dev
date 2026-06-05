// internal/repository/user_repository.go
package repository

import (
	"errors"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

// UserRepository interface
type UserRepository interface {
	Create(user *models.User) error
	FindByEmail(email string) (*models.User, error)
	FindByFederatedAccount(provider, subject string) (*models.User, error)
	FindByID(id string) (*models.User, error)
	FindAll() ([]models.User, error)
	FindByRoles(roles []string) ([]models.User, error)
	Update(user *models.User) error
	Delete(id string) error
	DeleteHard(id string) error
	GetTotalCount() (int64, error)
	// WithTx returns a new repository scoped to the given transaction.
	WithTx(tx *gorm.DB) UserRepository
}

// User repository implementation
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *database.Database) UserRepository {
	return &userRepository{db: db.DB}
}

func (r *userRepository) WithTx(tx *gorm.DB) UserRepository {
	return &userRepository{db: tx}
}

func (r *userRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *userRepository) FindByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByFederatedAccount(provider, subject string) (*models.User, error) {
	var user models.User
	err := r.db.Where("federated_provider = ? AND federated_subject = ?", provider, subject).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
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

func (r *userRepository) FindByRoles(roles []string) ([]models.User, error) {
	if len(roles) == 0 {
		return []models.User{}, nil
	}
	var users []models.User
	err := r.db.Where("role IN ?", roles).Find(&users).Error
	return users, err
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) Delete(id string) error {
	return r.db.Delete(&models.User{}, "id = ?", id).Error
}

func (r *userRepository) DeleteHard(id string) error {
	return r.db.Unscoped().Delete(&models.User{}, "id = ?", id).Error
}

func (r *userRepository) GetTotalCount() (int64, error) {
	var count int64
	err := r.db.Model(&models.User{}).Count(&count).Error
	return count, err
}
