package repository

import (
	"context"
	"strings"

	"wisdomHouse-backend/internal/database"
)

// EmailAudienceContact is the minimal contact projection used by the shared
// campaign audience resolver. Keeping this query-only projection here avoids
// leaking unrelated personal data into the email subsystem.
type EmailAudienceContact struct {
	Email      string
	Name       string
	SourceType string
	SourceID   string
	SourceName string
}

type EmailAudienceRepository interface {
	ListContacts(ctx context.Context, audienceType string) ([]EmailAudienceContact, error)
}

type emailAudienceRepository struct{ db *database.Database }

func NewEmailAudienceRepository(db *database.Database) EmailAudienceRepository {
	return &emailAudienceRepository{db: db}
}

func (r *emailAudienceRepository) ListContacts(ctx context.Context, audienceType string) ([]EmailAudienceContact, error) {
	var rows []EmailAudienceContact
	var query string
	switch strings.ToLower(strings.TrimSpace(audienceType)) {
	case "members":
		query = `SELECT email, concat_ws(' ', first_name, last_name) AS name, 'members' AS source_type, id::text AS source_id, 'Active members' AS source_name FROM members WHERE is_active = true AND trim(email) <> ''`
	case "workforce":
		query = `SELECT email, concat_ws(' ', first_name, last_name) AS name, 'workforce' AS source_type, id::text AS source_id, 'Serving workforce' AS source_name FROM workforce_members WHERE status = 'serving' AND email IS NOT NULL AND trim(email) <> ''`
	case "leadership":
		query = `SELECT email, concat_ws(' ', first_name, last_name) AS name, 'leadership' AS source_type, id::text AS source_id, 'Approved leadership' AS source_name FROM leadership_members WHERE status = 'approved' AND email IS NOT NULL AND trim(email) <> ''`
	case "subscribers":
		query = `SELECT email, coalesce(name, '') AS name, 'subscribers' AS source_type, id::text AS source_id, 'Active newsletter subscribers' AS source_name FROM subscribers WHERE status = 'active' AND deleted_at IS NULL AND trim(email) <> ''`
	default:
		return rows, nil
	}
	if err := r.db.DB.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
