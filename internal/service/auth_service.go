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

	// Create user object - DO NOT set ID, let GORM/database generate it
	user := &models.User{
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Password:  string(hashedPassword),
		Role:      role,
		// DO NOT set ID - database will generate UUID
		// DO NOT set CreatedAt/UpdatedAt - GORM will auto-generate
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
	
	// Remove password from response
	user.Password = ""
	return user, nil
}

// Helper methods
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