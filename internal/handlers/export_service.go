package handlers

import "time"

type ExportSubmission struct {
	ID             string
	FormID         string
	Name           string
	Email          string
	ContactNumber  string
	ContactAddress string
	CreatedAt      time.Time
	Values         map[string]any
}

type ExportForm struct {
	ID    string
	Title string
}

type ExportService interface {
	// Auth email from your auth layer (whatever you already use)
	GetAuthEmail(ctx any) (string, bool)

	// Domain lookups
	GetFormByID(formID string) (*ExportForm, error)
	ListSubmissionsByFormID(formID string) ([]ExportSubmission, error)
}
