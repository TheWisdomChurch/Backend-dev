package service

type AdminService interface {
	GetDashboardStats() (interface{}, error)
	GetSecurityOverview() (interface{}, error)
	GetPendingTestimonials() (interface{}, error)
	GetAllUsers() (interface{}, error)
	GetUserByID(userID string) (interface{}, error)
	CreateUser(firstName, lastName, email, password, role string) (interface{}, error)
	UpdateUser(id string, data map[string]interface{}) (interface{}, error)
	DeleteUser(id string) error
	ApproveUser(id string) (interface{}, error)
	RejectUser(id string, reason string) (interface{}, error)
}
