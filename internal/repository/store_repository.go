package repository

import (
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
)

type StoreRepository interface {
	ListProducts() ([]models.StoreProduct, error)
	CountProducts() (int64, error)
	CreateProducts(items []models.StoreProduct) error
	CreateOrder(order *models.StoreOrder) error
	GetOrderByOrderID(orderID string) (*models.StoreOrder, error)
}

type storeRepository struct {
	db *database.Database
}

func NewStoreRepository(db *database.Database) StoreRepository {
	return &storeRepository{db: db}
}

func (r *storeRepository) ListProducts() ([]models.StoreProduct, error) {
	var items []models.StoreProduct
	err := r.db.DB.Model(&models.StoreProduct{}).
		Where("is_active = ?", true).
		Order("id ASC").
		Find(&items).Error
	return items, err
}

func (r *storeRepository) CountProducts() (int64, error) {
	var count int64
	err := r.db.DB.Model(&models.StoreProduct{}).Count(&count).Error
	return count, err
}

func (r *storeRepository) CreateProducts(items []models.StoreProduct) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.DB.Create(&items).Error
}

func (r *storeRepository) CreateOrder(order *models.StoreOrder) error {
	return r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		for i := range order.Items {
			order.Items[i].StoreOrderID = order.ID
		}
		if len(order.Items) > 0 {
			if err := tx.Create(&order.Items).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *storeRepository) GetOrderByOrderID(orderID string) (*models.StoreOrder, error) {
	var order models.StoreOrder
	if err := r.db.DB.Preload("Items").
		First(&order, "order_id = ?", orderID).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

