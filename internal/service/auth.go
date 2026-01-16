// internal/service/auth.go
package service

type AuthService interface {
	Login(email, password string) (string, interface{}, error)
	Register(firstName, lastName, email, password, role string) (interface{}, error)
	GetUserByID(userID string) (interface{}, error)
}