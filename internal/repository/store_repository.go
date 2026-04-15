package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type StoreRepository interface {
	ListProducts() ([]models.StoreProduct, error)
	ListProductsAdmin(includeInactive bool) ([]models.StoreProduct, error)
	GetProductByID(id uint) (*models.StoreProduct, error)
	CreateProduct(item *models.StoreProduct) error
	UpdateProduct(item *models.StoreProduct) error
	UpdateProductStock(id uint, stock int) (*models.StoreProduct, error)
	UpdateProductActive(id uint, isActive bool) (*models.StoreProduct, error)
	CountProducts() (int64, error)
	CreateProducts(items []models.StoreProduct) error
	CreateOrder(order *models.StoreOrder) error
	GetOrderByOrderID(orderID string) (*models.StoreOrder, error)
	ListOrders(offset, limit int, status string) ([]models.StoreOrder, int64, error)
	UpdateOrderStatus(orderID, status string) (*models.StoreOrder, error)
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

func (r *storeRepository) ListProductsAdmin(includeInactive bool) ([]models.StoreProduct, error) {
	var items []models.StoreProduct
	q := r.db.DB.Model(&models.StoreProduct{}).Order("id ASC")
	if !includeInactive {
		q = q.Where("is_active = ?", true)
	}
	err := q.Find(&items).Error
	return items, err
}

func (r *storeRepository) GetProductByID(id uint) (*models.StoreProduct, error) {
	var item models.StoreProduct
	if err := r.db.DB.First(&item, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *storeRepository) CreateProduct(item *models.StoreProduct) error {
	return r.db.DB.Create(item).Error
}

func (r *storeRepository) UpdateProduct(item *models.StoreProduct) error {
	return r.db.DB.Save(item).Error
}

func (r *storeRepository) UpdateProductStock(id uint, stock int) (*models.StoreProduct, error) {
	var item models.StoreProduct
	err := r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, "id = ?", id).Error; err != nil {
			return err
		}
		item.Stock = stock
		return tx.Save(&item).Error
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *storeRepository) UpdateProductActive(id uint, isActive bool) (*models.StoreProduct, error) {
	var item models.StoreProduct
	err := r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, "id = ?", id).Error; err != nil {
			return err
		}
		item.IsActive = isActive
		return tx.Save(&item).Error
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
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
		subtotal := 0.0
		for i := range order.Items {
			item := &order.Items[i]
			if item.ProductID == nil {
				return errors.New("productId is required for each item")
			}

			var product models.StoreProduct
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				First(&product, "id = ? AND is_active = ?", *item.ProductID, true).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("product %d is not available", *item.ProductID)
				}
				return err
			}
			if item.Quantity <= 0 {
				return fmt.Errorf("product %d must have quantity greater than zero", *item.ProductID)
			}
			if product.Stock < item.Quantity {
				return fmt.Errorf("%s is out of stock. remaining: %d", product.Name, product.Stock)
			}

			sizes := decodeStringArray(product.Sizes)
			if len(sizes) > 0 && strings.TrimSpace(item.SelectedSize) != "" && !containsCI(sizes, item.SelectedSize) {
				return fmt.Errorf("invalid size for %s", product.Name)
			}

			colors := decodeStringArray(product.Colors)
			if len(colors) > 0 && strings.TrimSpace(item.SelectedColor) != "" && !containsCI(colors, item.SelectedColor) {
				return fmt.Errorf("invalid color for %s", product.Name)
			}

			product.Stock -= item.Quantity
			if err := tx.Model(&models.StoreProduct{}).
				Where("id = ?", product.ID).
				Update("stock", product.Stock).Error; err != nil {
				return err
			}

			item.Name = product.Name
			item.Price = product.Price
			subtotal += parsePriceToFloat(item.Price) * float64(item.Quantity)
		}
		if order.DeliveryFee < 0 {
			order.DeliveryFee = 0
		}
		order.Subtotal = subtotal
		order.Total = subtotal + order.DeliveryFee

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

func (r *storeRepository) ListOrders(offset, limit int, status string) ([]models.StoreOrder, int64, error) {
	var items []models.StoreOrder
	var total int64

	q := r.db.DB.Model(&models.StoreOrder{})
	if strings.TrimSpace(status) != "" {
		q = q.Where("status = ?", strings.TrimSpace(status))
	}

	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := q.Preload("Items").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *storeRepository) UpdateOrderStatus(orderID, status string) (*models.StoreOrder, error) {
	var item models.StoreOrder
	err := r.db.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&item, "order_id = ?", orderID).Error; err != nil {
			return err
		}
		item.Status = status
		if err := tx.Save(&item).Error; err != nil {
			return err
		}
		return tx.Preload("Items").First(&item, "order_id = ?", orderID).Error
	})
	if err != nil {
		return nil, err
	}
	return &item, nil
}

var nonDigitPriceRe = regexp.MustCompile(`[^0-9.]`)

func parsePriceToFloat(raw string) float64 {
	clean := strings.TrimSpace(nonDigitPriceRe.ReplaceAllString(raw, ""))
	if clean == "" {
		return 0
	}
	value, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return value
}

func decodeStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func containsCI(values []string, needle string) bool {
	target := strings.TrimSpace(strings.ToLower(needle))
	if target == "" {
		return false
	}
	for _, v := range values {
		if strings.TrimSpace(strings.ToLower(v)) == target {
			return true
		}
	}
	return false
}
