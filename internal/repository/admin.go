// internal/repository/admin_repository.go
package repository

import (
	"gorm.io/gorm"
	"wisdomHouse-backend/internal/database"
)

// AdminRepository interface
type AdminRepository interface {
	// For now, we'll keep it simple
	// You can add admin-specific queries later
}

// Admin repository implementation
type adminRepository struct {
	db *gorm.DB
}

// NewAdminRepository creates a new admin repository
func NewAdminRepository(db *database.Database) AdminRepository {
	return &adminRepository{db: db.DB}
}

// Add admin-specific methods here as needed
// For example:
// func (r *adminRepository) GetAdminStats() (map[string]interface{}, error) {
//     // Implement admin statistics
// }