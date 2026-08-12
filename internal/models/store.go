package models

import (
	"time"

	"gorm.io/datatypes"
)

type StoreProduct struct {
	ID            uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string         `gorm:"size:200;not null" json:"name"`
	Category      string         `gorm:"size:80;index;not null" json:"category"`
	Price         string         `gorm:"size:40;not null" json:"price"`
	OriginalPrice *string        `gorm:"size:40" json:"originalPrice,omitempty"`
	Image         string         `gorm:"type:text;not null" json:"image"`
	Description   string         `gorm:"type:text;not null" json:"description"`
	Sizes         datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"sizes"`
	Colors        datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"colors"`
	Tags          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"tags"`
	Stock         int            `gorm:"not null;default:0" json:"stock"`
	IsActive      bool           `gorm:"not null;default:true" json:"isActive"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

func (StoreProduct) TableName() string {
	return "store_products"
}

type StoreOrder struct {
	ID              string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	OrderID         string `gorm:"size:120;uniqueIndex;not null" json:"orderId"`
	Status          string `gorm:"size:30;not null;default:'pending'" json:"status"`
	PaymentStatus   string `gorm:"size:30;not null;default:'unpaid';index" json:"paymentStatus"`
	IdempotencyKey  string `gorm:"size:100;uniqueIndex;not null" json:"-"`
	AccessTokenHash string `gorm:"size:64;not null" json:"-"`

	Subtotal    float64 `gorm:"not null;default:0" json:"subtotal"`
	DeliveryFee float64 `gorm:"not null;default:0" json:"deliveryFee"`
	Total       float64 `gorm:"not null;default:0" json:"total"`

	PaymentMethod        string     `gorm:"size:40;not null" json:"paymentMethod"`
	PaymentSlipURL       *string    `gorm:"type:text" json:"paymentSlipUrl,omitempty"`
	PaidAt               *time.Time `json:"paidAt,omitempty"`
	CancelledAt          *time.Time `json:"cancelledAt,omitempty"`
	StockReleasedAt      *time.Time `json:"stockReleasedAt,omitempty"`
	ReservationExpiresAt *time.Time `gorm:"index" json:"reservationExpiresAt,omitempty"`

	CustomerFirstName string  `gorm:"size:120;not null" json:"-"`
	CustomerLastName  string  `gorm:"size:120;not null" json:"-"`
	CustomerEmail     string  `gorm:"size:255;index;not null" json:"-"`
	CustomerPhone     string  `gorm:"size:64;not null" json:"-"`
	CustomerAddress   *string `gorm:"type:text" json:"-"`
	CustomerCity      *string `gorm:"size:120" json:"-"`
	CustomerState     *string `gorm:"size:120" json:"-"`
	CustomerZipCode   *string `gorm:"size:40" json:"-"`

	CustomerAccountName *string `gorm:"size:180" json:"-"`
	CustomerBankName    *string `gorm:"size:180" json:"-"`

	Items []StoreOrderItem `gorm:"foreignKey:StoreOrderID;constraint:OnDelete:CASCADE" json:"items"`

	CreatedAt time.Time `json:"orderDate"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (StoreOrder) TableName() string {
	return "store_orders"
}

type StoreOrderItem struct {
	ID           string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`
	StoreOrderID string `gorm:"type:uuid;index;not null" json:"-"`
	ProductID    *uint  `gorm:"index" json:"productId,omitempty"`

	Name          string `gorm:"size:220;not null" json:"name"`
	Price         string `gorm:"size:40;not null" json:"price"`
	Quantity      int    `gorm:"not null;default:1" json:"quantity"`
	SelectedSize  string `gorm:"size:80;not null" json:"selectedSize"`
	SelectedColor string `gorm:"size:80;not null" json:"selectedColor"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (StoreOrderItem) TableName() string {
	return "store_order_items"
}
