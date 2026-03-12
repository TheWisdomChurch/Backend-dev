// internal/service/auth.go
package service

import (
	"time"

	"wisdomHouse-backend/internal/models"
)

type AuthService interface {
	Login(email, password string, meta LoginMetadata) (*LoginResult, error)
	VerifyLoginMFA(email, code, purpose, method string, meta LoginMetadata) (*models.User, string, error)
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
	GetSecurityProfile(userID string) (*models.AuthSecurityProfile, error)
	BeginTOTPSetup(userID string) (*models.TOTPSetupResponse, error)
	EnableTOTP(userID, code string) (*models.AuthSecurityProfile, error)
	DisableTOTP(userID, code string) (*models.AuthSecurityProfile, error)
	SetPreferredMFAMethod(userID, method string) (*models.AuthSecurityProfile, error)
	CompleteGoogleLogin(googleSubject, email, firstName, lastName string, meta LoginMetadata) (*LoginResult, error)
}

type LoginMetadata struct {
	IP        string
	UserAgent string
	DeviceID  string
}

type LoginResult struct {
	User         *models.User
	OTPRequired  bool
	MFAMethod    string
	OTPPurpose   string
	OTPExpiresAt *time.Time
	OTPActionURL string
	AuthMethod   string
}
