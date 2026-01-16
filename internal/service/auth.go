// internal/service/auth.go
package service

type AuthService interface {
	Login(email, password string) (string, interface{}, error)
	Register(firstName, lastName, email, password, role string) (interface{}, error)
	GetUserByID(userID string) (interface{}, error)
	UpdateProfile(userID, firstName, lastName, email, username string) (interface{}, error)
	ChangePassword(userID, currentPassword, newPassword string) error
	DeleteAccount(userID string) error
	ClearData(userID string) error
}