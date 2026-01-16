// internal/service/admin.go
package service

type AdminService interface {
	GetDashboardStats() (interface{}, error)
	GetPendingTestimonials() (interface{}, error)
	GetAllUsers() (interface{}, error)
	GetUserByID(userID string) (interface{}, error)
	CreateUser(firstName, lastName, email, password, role string) (interface{}, error)
	UpdateUser(id string, data map[string]interface{}) (interface{}, error)    // Add this
	DeleteUser(id string) error                                               // Add this
}