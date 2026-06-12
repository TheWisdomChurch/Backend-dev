package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GivingCategory struct {
	ID        string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name      string         `gorm:"not null;uniqueIndex;size:150" json:"name"`
	Code      string         `gorm:"not null;uniqueIndex;size:50" json:"code"`
	IsActive  bool           `gorm:"default:true;not null" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g *GivingCategory) BeforeCreate(_ *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	return nil
}

type GivingTransaction struct {
	ID               string         `gorm:"type:uuid;primaryKey" json:"id"`
	CategoryID       string         `gorm:"type:uuid;not null;index" json:"category_id"`
	Category         *GivingCategory `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	MemberID         *string        `gorm:"type:uuid;index" json:"member_id,omitempty"`
	CampusID         *string        `gorm:"type:uuid;index" json:"campus_id,omitempty"`
	AmountKobo       int64          `gorm:"not null" json:"amount_kobo"` // amount in lowest currency unit (kobo/cents)
	Currency         string         `gorm:"size:3;not null;default:'NGN'" json:"currency"`
	Channel          string         `gorm:"size:50;not null" json:"channel"` // card, transfer, cash, ussd
	PaymentRef       string         `gorm:"size:255;uniqueIndex" json:"payment_ref"`
	PaymentProvider  string         `gorm:"size:50" json:"payment_provider"` // paystack, stripe, manual
	Status           string         `gorm:"size:50;not null;default:'pending';index" json:"status"` // pending, success, failed, reversed
	GiverName        string         `gorm:"size:200" json:"giver_name"`
	GiverEmail       string         `gorm:"size:255;index" json:"giver_email"`
	RecordedByID     *string        `gorm:"type:uuid" json:"recorded_by_id,omitempty"`
	GivenAt          time.Time      `gorm:"not null;index" json:"given_at"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

func (g *GivingTransaction) BeforeCreate(_ *gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	if g.GivenAt.IsZero() {
		g.GivenAt = time.Now().UTC()
	}
	return nil
}

// GivingMonthlySummary is a read-only projection populated from DB aggregation.
type GivingMonthlySummary struct {
	Year       int     `json:"year"`
	Month      int     `json:"month"`
	CategoryID string  `json:"category_id"`
	TotalKobo  int64   `json:"total_kobo"`
	Count      int     `json:"count"`
}
