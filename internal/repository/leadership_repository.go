package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

type LeadershipRepository interface {
	List(offset, limit int, role, status string) ([]models.LeadershipMember, int64, error)
	ListApproved(role string) ([]models.LeadershipMember, error)
	GetByID(id string) (*models.LeadershipMember, error)
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

func (r *leadershipRepository) GetByID(id string) (*models.LeadershipMember, error) {
	var member models.LeadershipMember
	if err := r.db.DB.First(&member, "id = ?", id).Error; err != nil {
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
