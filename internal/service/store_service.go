package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)

type CreateStoreOrderRequest struct {
	IdempotencyKey string                       `json:"idempotencyKey"`
	CheckoutToken  string                       `json:"checkoutToken"`
	Subtotal       float64                      `json:"subtotal"`
	DeliveryFee    float64                      `json:"deliveryFee"`
	Total          float64                      `json:"total"`
	PaymentMethod  string                       `json:"paymentMethod"`
	PaymentSlipURL string                       `json:"paymentSlipUrl"`
	Items          []CreateStoreOrderItem       `json:"items"`
	Customer       CreateStoreOrderCustomer     `json:"customer"`
	BankDetails    *CreateStoreOrderBankDetails `json:"bankDetails,omitempty"`
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
	ListProductsAdmin(includeInactive bool) ([]models.StoreProduct, error)
	CreateProduct(req UpsertStoreProductRequest) (*models.StoreProduct, error)
	UpdateProduct(id uint, req UpsertStoreProductRequest) (*models.StoreProduct, error)
	UpdateProductStock(id uint, stock int) (*models.StoreProduct, error)
	UpdateProductActive(id uint, isActive bool) (*models.StoreProduct, error)
	CreateOrder(req CreateStoreOrderRequest) (*models.StoreOrder, error)
	GetOrder(orderID, checkoutToken string) (*models.StoreOrder, error)
	ListOrders(page, limit int, status string) ([]models.StoreOrder, int64, error)
	UpdateOrderStatus(orderID, status string) (*models.StoreOrder, error)
	SubmitPaymentProof(orderID, checkoutToken, proofURL string) (*models.StoreOrder, error)
	UpdateOrderPaymentStatus(orderID, status string) (*models.StoreOrder, error)
	ExpirePendingReservations(now time.Time, limit int) (int, error)
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

func (s *storeService) ListProductsAdmin(includeInactive bool) ([]models.StoreProduct, error) {
	return s.repo.ListProductsAdmin(includeInactive)
}

type UpsertStoreProductRequest struct {
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	Price         string   `json:"price"`
	OriginalPrice *string  `json:"originalPrice,omitempty"`
	Image         string   `json:"image"`
	Description   string   `json:"description"`
	Sizes         []string `json:"sizes"`
	Colors        []string `json:"colors"`
	Tags          []string `json:"tags"`
	Stock         int      `json:"stock"`
	IsActive      *bool    `json:"isActive,omitempty"`
}

func (s *storeService) CreateProduct(req UpsertStoreProductRequest) (*models.StoreProduct, error) {
	item, err := normalizeStoreProductPayload(req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateProduct(item); err != nil {
		return nil, err
	}
	return s.repo.GetProductByID(item.ID)
}

func (s *storeService) UpdateProduct(id uint, req UpsertStoreProductRequest) (*models.StoreProduct, error) {
	existing, err := s.repo.GetProductByID(id)
	if err != nil {
		return nil, err
	}
	item, err := normalizeStoreProductPayload(req, existing)
	if err != nil {
		return nil, err
	}
	item.ID = existing.ID
	item.CreatedAt = existing.CreatedAt
	if err := s.repo.UpdateProduct(item); err != nil {
		return nil, err
	}
	return s.repo.GetProductByID(id)
}

func (s *storeService) UpdateProductStock(id uint, stock int) (*models.StoreProduct, error) {
	if stock < 0 {
		return nil, errors.New("stock cannot be negative")
	}
	return s.repo.UpdateProductStock(id, stock)
}

func (s *storeService) UpdateProductActive(id uint, isActive bool) (*models.StoreProduct, error) {
	return s.repo.UpdateProductActive(id, isActive)
}

func (s *storeService) CreateOrder(req CreateStoreOrderRequest) (*models.StoreOrder, error) {
	key := strings.TrimSpace(req.IdempotencyKey)
	token := strings.TrimSpace(req.CheckoutToken)
	if _, err := uuid.Parse(key); err != nil {
		return nil, errors.New("idempotencyKey must be a valid UUID")
	}
	if _, err := uuid.Parse(token); err != nil {
		return nil, errors.New("checkoutToken must be a valid UUID")
	}
	if existing, err := s.repo.GetOrderByIdempotencyKey(key); err == nil {
		if !secureTokenMatches(token, existing.AccessTokenHash) {
			return nil, errors.New("checkout attempt conflicts with an existing order")
		}
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if strings.TrimSpace(req.PaymentMethod) == "" {
		return nil, errors.New("paymentMethod is required")
	}
	method := strings.ToLower(strings.TrimSpace(req.PaymentMethod))
	if method != "transfer" && method != "delivery" {
		return nil, errors.New("paymentMethod must be transfer or delivery")
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
		// The public order ID is a capability identifier and must never be
		// caller-controlled or predictable. A client-provided orderId is ignored
		// for backward compatibility with older frontends.
		OrderID:             newPublicOrderID(),
		Status:              "pending",
		PaymentStatus:       "unpaid",
		IdempotencyKey:      key,
		AccessTokenHash:     hashCheckoutToken(token),
		Subtotal:            req.Subtotal,
		DeliveryFee:         req.DeliveryFee,
		Total:               req.Total,
		PaymentMethod:       method,
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
	if slip := strings.TrimSpace(req.PaymentSlipURL); slip != "" {
		order.PaymentSlipURL = &slip
	}
	if method == "transfer" {
		expires := time.Now().UTC().Add(30 * time.Minute)
		order.ReservationExpiresAt = &expires
	}
	if req.BankDetails != nil {
		order.CustomerAccountName = strPtrOrNil(req.BankDetails.CustomerAccountName)
		order.CustomerBankName = strPtrOrNil(req.BankDetails.CustomerBankName)
	}

	order.Items = make([]models.StoreOrderItem, 0, len(req.Items))
	for _, item := range req.Items {
		if strings.TrimSpace(item.Name) == "" || item.Quantity <= 0 {
			return nil, errors.New("each order item must include quantity > 0")
		}
		if item.ProductID == nil {
			return nil, errors.New("productId is required for each order item")
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

func (s *storeService) ExpirePendingReservations(now time.Time, limit int) (int, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	return s.repo.ExpirePendingReservations(now.UTC(), limit)
}

func (s *storeService) GetOrder(orderID, checkoutToken string) (*models.StoreOrder, error) {
	order, err := s.repo.GetOrderByOrderID(strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	if !secureTokenMatches(strings.TrimSpace(checkoutToken), order.AccessTokenHash) {
		return nil, gorm.ErrRecordNotFound
	}
	return order, nil
}

func hashCheckoutToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func secureTokenMatches(token, expected string) bool {
	if token == "" || expected == "" {
		return false
	}
	actual := hashCheckoutToken(token)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func newPublicOrderID() string {
	return "ord_" + uuid.NewString()
}

func (s *storeService) ListOrders(page, limit int, status string) ([]models.StoreOrder, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.ListOrders((page-1)*limit, limit, strings.TrimSpace(status))
}

func (s *storeService) UpdateOrderStatus(orderID, status string) (*models.StoreOrder, error) {
	normalized := strings.TrimSpace(strings.ToLower(status))
	switch normalized {
	case "pending", "processing", "shipped", "delivered", "cancelled":
	default:
		return nil, errors.New("invalid order status")
	}
	if strings.TrimSpace(orderID) == "" {
		return nil, errors.New("orderId is required")
	}
	current, err := s.repo.GetOrderByOrderID(strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	allowed := map[string]map[string]bool{"pending": {"processing": true, "cancelled": true}, "processing": {"shipped": true, "cancelled": true}, "shipped": {"delivered": true}, "delivered": {}, "cancelled": {}}
	if normalized != current.Status && !allowed[current.Status][normalized] {
		return nil, fmt.Errorf("order cannot move from %s to %s", current.Status, normalized)
	}
	return s.repo.UpdateOrderStatus(strings.TrimSpace(orderID), normalized)
}

func (s *storeService) SubmitPaymentProof(orderID, checkoutToken, proofURL string) (*models.StoreOrder, error) {
	order, err := s.GetOrder(orderID, checkoutToken)
	if err != nil {
		return nil, err
	}
	if order.PaymentMethod != "transfer" {
		return nil, errors.New("payment proof is only valid for bank-transfer orders")
	}
	proofURL = strings.TrimSpace(proofURL)
	parsed, err := url.ParseRequestURI(proofURL)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("a valid payment proof URL is required")
	}
	return s.repo.UpdateOrderPaymentProof(order.OrderID, proofURL)
}

func (s *storeService) UpdateOrderPaymentStatus(orderID, status string) (*models.StoreOrder, error) {
	order, err := s.repo.GetOrderByOrderID(strings.TrimSpace(orderID))
	if err != nil {
		return nil, err
	}
	status = strings.ToLower(strings.TrimSpace(status))
	allowed := map[string]map[string]bool{"unpaid": {"proof_submitted": true, "paid": true, "failed": true}, "proof_submitted": {"paid": true, "failed": true}, "failed": {"proof_submitted": true}, "paid": {"refunded": true}, "refunded": {}}
	if status != order.PaymentStatus && !allowed[order.PaymentStatus][status] {
		return nil, fmt.Errorf("payment cannot move from %s to %s", order.PaymentStatus, status)
	}
	var paidAt *time.Time
	if status == "paid" {
		now := time.Now().UTC()
		paidAt = &now
	}
	return s.repo.UpdateOrderPaymentStatus(order.OrderID, status, paidAt)
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

func normalizeStoreProductPayload(req UpsertStoreProductRequest, base *models.StoreProduct) (*models.StoreProduct, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name is required")
	}
	if strings.TrimSpace(req.Category) == "" {
		return nil, errors.New("category is required")
	}
	if strings.TrimSpace(req.Price) == "" {
		return nil, errors.New("price is required")
	}
	if parseCurrency(req.Price) <= 0 {
		return nil, errors.New("price must be a valid positive amount")
	}
	if strings.TrimSpace(req.Image) == "" {
		return nil, errors.New("image is required")
	}
	if strings.TrimSpace(req.Description) == "" {
		return nil, errors.New("description is required")
	}
	if req.Stock < 0 {
		return nil, errors.New("stock cannot be negative")
	}

	sizes, _ := json.Marshal(normalizeStringList(req.Sizes))
	colors, _ := json.Marshal(normalizeStringList(req.Colors))
	tags, _ := json.Marshal(normalizeStringList(req.Tags))

	item := &models.StoreProduct{
		Name:          strings.TrimSpace(req.Name),
		Category:      strings.TrimSpace(strings.ToLower(req.Category)),
		Price:         strings.TrimSpace(req.Price),
		OriginalPrice: strPtrOrNil(ptrValue(req.OriginalPrice)),
		Image:         strings.TrimSpace(req.Image),
		Description:   strings.TrimSpace(req.Description),
		Sizes:         sizes,
		Colors:        colors,
		Tags:          tags,
		Stock:         req.Stock,
		IsActive:      true,
	}
	if base != nil {
		item.ID = base.ID
		item.CreatedAt = base.CreatedAt
		item.IsActive = base.IsActive
	}
	if req.IsActive != nil {
		item.IsActive = *req.IsActive
	}
	return item, nil
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func ptrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func parseCurrency(raw string) float64 {
	clean := strings.TrimSpace(raw)
	clean = strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '.' {
			return r
		}
		return -1
	}, clean)
	if clean == "" {
		return 0
	}
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return v
}
