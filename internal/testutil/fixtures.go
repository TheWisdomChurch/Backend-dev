package testutil

import (
	"fmt"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/models"
)

// seq is a package-level counter for generating unique test identifiers.
var seq atomic.Int64

func next() int64 { return seq.Add(1) }

// ---- User fixtures --------------------------------------------------------

// UserOpt is a functional option for BuildUser.
type UserOpt func(*models.User)

func WithRole(role string) UserOpt      { return func(u *models.User) { u.Role = role } }
func WithEmail(email string) UserOpt    { return func(u *models.User) { u.Email = email } }
func WithInactive() UserOpt             { return func(u *models.User) { u.IsActive = false } }
func WithUnapproved() UserOpt           { return func(u *models.User) { u.AdminApproved = false } }

// BuildUser creates and persists a User with bcrypt-hashed password "Test1234!".
// bcrypt cost 4 is used so tests run fast.
func BuildUser(t *testing.T, db *gorm.DB, opts ...UserOpt) *models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Test1234!"), 4)
	if err != nil {
		t.Fatalf("BuildUser: hash password: %v", err)
	}

	u := &models.User{
		FirstName:     "Test",
		LastName:      fmt.Sprintf("User%d", next()),
		Email:         fmt.Sprintf("user%d@test.example", next()),
		Password:      string(hash),
		Role:          "admin",
		IsActive:      true,
		AdminApproved: true,
	}
	for _, opt := range opts {
		opt(u)
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("BuildUser: create: %v", err)
	}
	return u
}

// ---- Member fixtures -------------------------------------------------------

// MemberOpt is a functional option for BuildMember.
type MemberOpt func(*models.Member)

func WithMemberEmail(email string) MemberOpt { return func(m *models.Member) { m.Email = email } }
func WithMemberInactive() MemberOpt          { return func(m *models.Member) { m.IsActive = false } }
func WithBirthday(month, day int) MemberOpt {
	return func(m *models.Member) {
		m.BirthdayMonth = &month
		m.BirthdayDay = &day
	}
}

// BuildMember creates and persists a Member record.
func BuildMember(t *testing.T, db *gorm.DB, opts ...MemberOpt) *models.Member {
	t.Helper()
	n := next()
	m := &models.Member{
		FirstName: "Alice",
		LastName:  fmt.Sprintf("Smith%d", n),
		Email:     fmt.Sprintf("member%d@test.example", n),
		IsActive:  true,
	}
	for _, opt := range opts {
		opt(m)
	}
	if err := db.Create(m).Error; err != nil {
		t.Fatalf("BuildMember: create: %v", err)
	}
	return m
}

// ---- Giving fixtures -------------------------------------------------------

// BuildGivingCategory creates and persists a GivingCategory.
func BuildGivingCategory(t *testing.T, db *gorm.DB) *models.GivingCategory {
	t.Helper()
	cat := &models.GivingCategory{
		Name:     fmt.Sprintf("Tithe%d", next()),
		Code:     fmt.Sprintf("TITHE%d", next()),
		IsActive: true,
	}
	if err := db.Create(cat).Error; err != nil {
		t.Fatalf("BuildGivingCategory: create: %v", err)
	}
	return cat
}
