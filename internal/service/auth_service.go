// internal/service/auth_service.go
package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
)



// AuthService implementation
type authServiceImpl struct {
	userRepo      repository.UserRepository
	jwtSecret     string
	jwtExpiration time.Duration
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo repository.UserRepository, jwtSecret string, jwtExpiration time.Duration) AuthService {
	return &authServiceImpl{
		userRepo:      userRepo,
		jwtSecret:     jwtSecret,
		jwtExpiration: jwtExpiration,
	}
}

func (s *authServiceImpl) Login(email, password string) (string, interface{}, error) {
	// Find user by email
	user, err := s.userRepo.FindByEmail(email)
	if err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	// Check if user is active
	if !user.IsActive {
		return "", nil, errors.New("account is deactivated")
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", nil, errors.New("invalid credentials")
	}

	// Generate JWT token
	token, err := s.generateToken(user)
	if err != nil {
		return "", nil, err
	}

	// Remove password from user object before returning
	user.Password = ""
	
	return token, user, nil
}

func (s *authServiceImpl) Register(firstName, lastName, email, password, role string) (interface{}, error) {
	// Check if user already exists
	existingUser, _ := s.userRepo.FindByEmail(email)
	if existingUser != nil {
		return nil, errors.New("user already exists")
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user object
	user := &models.User{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Password:  string(hashedPassword),
		Role:      role,
		IsActive:  true,
	}

	// Save user
	if err := s.userRepo.Create(user); err != nil {
		return nil, err
	}

	// Remove password from response
	user.Password = ""
	return user, nil
}

func (s *authServiceImpl) GetUserByID(userID string) (interface{}, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	
	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}
	
	// Remove password from response
	user.Password = ""
	return user, nil
}

// UpdateProfile updates user profile information
func (s *authServiceImpl) UpdateProfile(userID, firstName, lastName, email, username string) (interface{}, error) {
	// Get current user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, errors.New("account is deactivated")
	}

	// Update fields if provided
	updated := false
	if firstName != "" && firstName != user.FirstName {
		user.FirstName = firstName
		updated = true
	}
	
	if lastName != "" && lastName != user.LastName {
		user.LastName = lastName
		updated = true
	}
	
	if email != "" && email != user.Email {
		// Check if email already exists for another user
		existingUser, _ := s.userRepo.FindByEmail(email)
		if existingUser != nil && existingUser.ID != userID {
			return nil, errors.New("email already in use")
		}
		user.Email = email
		updated = true
	}

	// Only update if something changed
	if updated {
		user.UpdatedAt = time.Now()
		
		// Save updated user
		if err := s.userRepo.Update(user); err != nil {
			return nil, errors.New("failed to update profile")
		}
	}

	// Remove password from response
	user.Password = ""
	return user, nil
}

// ChangePassword changes user password
func (s *authServiceImpl) ChangePassword(userID, currentPassword, newPassword string) error {
	// Get user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return errors.New("account is deactivated")
	}

	// Verify current password
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)); err != nil {
		return errors.New("current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("failed to hash password")
	}

	// Update password
	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now()
	
	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to update password")
	}

	return nil
}

// DeleteAccount deletes user account (soft delete)
func (s *authServiceImpl) DeleteAccount(userID string) error {
	// Get user
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if already deactivated
	if !user.IsActive {
		return errors.New("account already deactivated")
	}

	// Soft delete by marking as inactive
	user.IsActive = false
	user.UpdatedAt = time.Now()
	
	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to delete account")
	}

	return nil
}

// ClearData clears user data (placeholder implementation)
func (s *authServiceImpl) ClearData(userID string) error {
	// Get user to verify existence
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return errors.New("user not found")
	}

	// Check if user is active
	if !user.IsActive {
		return errors.New("account is deactivated")
	}

	// This is a placeholder - implement based on your application needs
	user.UpdatedAt = time.Now()
	if err := s.userRepo.Update(user); err != nil {
		return errors.New("failed to clear data")
	}

	return nil
}

// Helper method to generate JWT token
func (s *authServiceImpl) generateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"role":    user.Role,
		"exp":     time.Now().Add(s.jwtExpiration).Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}