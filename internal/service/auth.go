// internal/service/auth.go
package service

import (
	"time"

	"wisdomHouse-backend/internal/models"
)

type AuthService interface {
	Login(email, password string, meta LoginMetadata) (*LoginResult, error)
	VerifyLoginOTP(email, code, purpose string, meta LoginMetadata) (*models.User, error)
	Register(firstName, lastName, email, password, role string) (interface{}, error)
	GetUserByID(userID string) (interface{}, error)
	UpdateProfile(userID, firstName, lastName, email, username string) (interface{}, error)
	ChangePassword(userID, currentPassword, newPassword string) error
	DeleteAccount(userID string) error
	ClearData(userID string) error
	RequestPasswordReset(email, actionURL string) (*models.SendOTPResponse, error)
	ResetPasswordWithOTP(email, code, purpose, newPassword string) error
	UntrustDevice(userID, deviceID string) error
	ResendLoginOTP(email string, meta LoginMetadata) (*LoginResult, error)
}

type LoginMetadata struct {
	IP        string
	UserAgent string
	DeviceID  string
}

type LoginResult struct {
	User         *models.User
	OTPRequired  bool
	OTPPurpose   string
	OTPExpiresAt *time.Time
	OTPActionURL string
}
