package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type CreateStoreOrderRequest struct {
	OrderID       string                       `json:"orderId"`
	Subtotal      float64                      `json:"subtotal"`
	DeliveryFee   float64                      `json:"deliveryFee"`
	Total         float64                      `json:"total"`
	PaymentMethod string                       `json:"paymentMethod"`
	Items         []CreateStoreOrderItem       `json:"items"`
	Customer      CreateStoreOrderCustomer     `json:"customer"`
	BankDetails   *CreateStoreOrderBankDetails `json:"bankDetails,omitempty"`
}

type CreateStoreOrderItem struct {
	ID            string `json:"id"`
	ProductID     *uint  `json:"productId,omitempty"`
	Name          string `json:"name"`
	Price         string `json:"price"`
	Quantity      int    `json:"quantity"`
	SelectedSize  string `json:"selectedSize"`
	SelectedColor string `json:"selectedColor"`
}

type CreateStoreOrderCustomer struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	City      string `json:"city"`
	State     string `json:"state"`
	ZipCode   string `json:"zipCode"`
}

type CreateStoreOrderBankDetails struct {
	CustomerAccountName string `json:"customerAccountName"`
	CustomerBankName    string `json:"customerBankName"`
}

type StoreService interface {
	ListProducts() ([]models.StoreProduct, error)
	CreateOrder(req CreateStoreOrderRequest) (*models.StoreOrder, error)
	GetOrder(orderID string) (*models.StoreOrder, error)
}

type storeService struct {
	repo repository.StoreRepository
}

func NewStoreService(repo repository.StoreRepository) StoreService {
	s := &storeService{repo: repo}
	_ = s.seedDefaultProducts()
	return s
}

func (s *storeService) ListProducts() ([]models.StoreProduct, error) {
	return s.repo.ListProducts()
}

func (s *storeService) CreateOrder(req CreateStoreOrderRequest) (*models.StoreOrder, error) {
	if strings.TrimSpace(req.OrderID) == "" {
		return nil, errors.New("orderId is required")
	}
	if strings.TrimSpace(req.PaymentMethod) == "" {
		return nil, errors.New("paymentMethod is required")
	}
	if len(req.Items) == 0 {
		return nil, errors.New("at least one item is required")
	}
	if strings.TrimSpace(req.Customer.FirstName) == "" || strings.TrimSpace(req.Customer.LastName) == "" {
		return nil, errors.New("customer first and last name are required")
	}
	if strings.TrimSpace(req.Customer.Email) == "" || strings.TrimSpace(req.Customer.Phone) == "" {
		return nil, errors.New("customer email and phone are required")
	}

	order := &models.StoreOrder{
		OrderID:             strings.TrimSpace(req.OrderID),
		Status:              "pending",
		Subtotal:            req.Subtotal,
		DeliveryFee:         req.DeliveryFee,
		Total:               req.Total,
		PaymentMethod:       strings.TrimSpace(req.PaymentMethod),
		CustomerFirstName:   strings.TrimSpace(req.Customer.FirstName),
		CustomerLastName:    strings.TrimSpace(req.Customer.LastName),
		CustomerEmail:       strings.TrimSpace(strings.ToLower(req.Customer.Email)),
		CustomerPhone:       strings.TrimSpace(req.Customer.Phone),
		CustomerAddress:     strPtrOrNil(req.Customer.Address),
		CustomerCity:        strPtrOrNil(req.Customer.City),
		CustomerState:       strPtrOrNil(req.Customer.State),
		CustomerZipCode:     strPtrOrNil(req.Customer.ZipCode),
		CustomerAccountName: nil,
		CustomerBankName:    nil,
	}
	if req.BankDetails != nil {
		order.CustomerAccountName = strPtrOrNil(req.BankDetails.CustomerAccountName)
		order.CustomerBankName = strPtrOrNil(req.BankDetails.CustomerBankName)
	}

	order.Items = make([]models.StoreOrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 {
			return nil, errors.New("each order item must include name and quantity > 0")
		}
		order.Items = append(order.Items, models.StoreOrderItem{
			ProductID:     item.ProductID,
			Name:          strings.TrimSpace(item.Name),
			Price:         strings.TrimSpace(item.Price),
			Quantity:      item.Quantity,
			SelectedSize:  strings.TrimSpace(item.SelectedSize),
			SelectedColor: strings.TrimSpace(item.SelectedColor),
		})
	}

	if err := s.repo.CreateOrder(order); err != nil {
		return nil, err
	}
	return s.repo.GetOrderByOrderID(order.OrderID)
}

func (s *storeService) GetOrder(orderID string) (*models.StoreOrder, error) {
	return s.repo.GetOrderByOrderID(strings.TrimSpace(orderID))
}

func (s *storeService) seedDefaultProducts() error {
	count, err := s.repo.CountProducts()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	type productSeed struct {
		Name          string
		Category      string
		Price         string
		OriginalPrice string
		Image         string
		Description   string
		Sizes         []string
		Colors        []string
		Tags          []string
		Stock         int
	}
	defaults := []productSeed{
		{"Wisdom House Polo Shirt", "clothing", "N6000", "N7500", "/images/placeholder.jpg", "Premium quality polo shirt with Wisdom House logo", []string{"S", "M", "L", "XL", "XXL"}, []string{"Navy", "Black", "White", "Gray"}, []string{"polo", "shirt", "clothing", "premium"}, 50},
		{"Wisdom House Classic Tee", "clothing", "N4500", "N5500", "/images/placeholder.jpg", "Comfortable classic t-shirt with Wisdom House design", []string{"S", "M", "L", "XL"}, []string{"White", "Black", "Gray"}, []string{"t-shirt", "tee", "casual"}, 75},
		{"Wisdom House Cap", "accessories", "N3500", "N4500", "/images/placeholder.jpg", "Stylish cap with Wisdom House logo embroidery", []string{"One Size"}, []string{"Black", "Navy", "Red"}, []string{"cap", "hat", "accessory"}, 30},
		{"Wisdom House Inspirational Mug", "utilities", "N2500", "N3000", "/images/placeholder.jpg", "Ceramic mug with daily inspirational quotes", []string{"One Size"}, []string{"White", "Black"}, []string{"mug", "cup", "drinkware"}, 100},
		{"Wisdom House Tote Bag", "accessories", "N4000", "N5000", "/images/placeholder.jpg", "Eco-friendly tote bag with motivational message", []string{"One Size"}, []string{"Natural", "Black", "Blue"}, []string{"tote", "bag", "eco-friendly"}, 40},
	}

	items := make([]models.StoreProduct, 0, len(defaults))
	for _, seed := range defaults {
		sizes, _ := json.Marshal(seed.Sizes)
		colors, _ := json.Marshal(seed.Colors)
		tags, _ := json.Marshal(seed.Tags)
		original := strings.TrimSpace(seed.OriginalPrice)
		items = append(items, models.StoreProduct{
			Name:          seed.Name,
			Category:      seed.Category,
			Price:         seed.Price,
			OriginalPrice: strPtrOrNil(original),
			Image:         seed.Image,
			Description:   seed.Description,
			Sizes:         sizes,
			Colors:        colors,
			Tags:          tags,
			Stock:         seed.Stock,
			IsActive:      true,
		})
	}

	if err := s.repo.CreateProducts(items); err != nil {
		return fmt.Errorf("seed store products: %w", err)
	}
	return nil
}

func strPtrOrNil(v string) *string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
