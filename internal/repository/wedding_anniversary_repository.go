package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
)

// WeddingAnniversaryFilter narrows an admin list query. Zero values mean "no
// filter on that dimension".
type WeddingAnniversaryFilter struct {
	Month       int
	Status      string // "active" | "archived" | ""
	SubjectType string
	NeedsSpouse bool // only rows with a blank spouse_name (backfilled)
	Offset      int
	Limit       int
}

type WeddingAnniversaryRepository interface {
	Upsert(ctx context.Context, row *models.WeddingAnniversary) (*models.WeddingAnniversary, error)
	GetByID(ctx context.Context, id string) (*models.WeddingAnniversary, error)
	GetBySubject(ctx context.Context, subjectType, subjectID string) (*models.WeddingAnniversary, error)
	List(ctx context.Context, f WeddingAnniversaryFilter) ([]models.WeddingAnniversaryView, int64, error)
	Delete(ctx context.Context, id string) error
	SetStatus(ctx context.Context, id, status string) error
	Stats(ctx context.Context) (*models.WeddingAnniversaryStats, error)
	// ListDueByMonthDay returns active rows whose anniversary falls on
	// month/day, hydrated with the greeted person's current name + email and
	// filtered to people who are still reachable (active member / approved
	// leader / serving worker with a non-empty email).
	ListDueByMonthDay(ctx context.Context, month, day int) ([]models.WeddingAnniversaryView, error)
}

type weddingAnniversaryRepository struct{ db *database.Database }

func NewWeddingAnniversaryRepository(db *database.Database) WeddingAnniversaryRepository {
	return &weddingAnniversaryRepository{db: db}
}

// Upsert writes one marriage row, keyed by (subject_type, subject_id). When the
// incoming row links a spouse who is also in our system, an existing mirror row
// (that spouse as subject, this person as their spouse) is updated in place
// instead of creating a second record for the same marriage.
func (r *weddingAnniversaryRepository) Upsert(ctx context.Context, row *models.WeddingAnniversary) (*models.WeddingAnniversary, error) {
	if row == nil {
		return nil, errors.New("wedding anniversary row is required")
	}
	row.SubjectType = models.WeddingAnniversarySubjectType(strings.TrimSpace(string(row.SubjectType)))
	row.SubjectID = strings.TrimSpace(row.SubjectID)
	if !models.ValidWeddingAnniversarySubjectType(string(row.SubjectType)) || row.SubjectID == "" {
		return nil, errors.New("valid subjectType and subjectId are required")
	}
	if row.Status == "" {
		row.Status = models.WeddingAnniversaryStatusActive
	}
	if row.Source == "" {
		row.Source = models.WeddingAnniversarySourceAdmin
	}

	db := r.db.WithContext(ctx)

	// Mirror-case merge: this row points at a spouse who is a subject in their
	// own right, and that spouse already has a row pointing back here.
	if row.SpouseSubjectID != nil && strings.TrimSpace(*row.SpouseSubjectID) != "" {
		var mirror models.WeddingAnniversary
		err := db.Where(
			"subject_type = ? AND subject_id = ? AND spouse_subject_id = ?",
			strings.TrimSpace(valueOrEmptyPtr(row.SpouseSubjectType)), strings.TrimSpace(*row.SpouseSubjectID), row.SubjectID,
		).First(&mirror).Error
		if err == nil {
			row.ID = mirror.ID
			row.SubjectType = mirror.SubjectType
			row.SubjectID = mirror.SubjectID
			row.CreatedAt = mirror.CreatedAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	if err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "subject_type"}, {Name: "subject_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"anniversary_month", "anniversary_day", "wedding_year",
			"spouse_name", "spouse_email", "spouse_subject_type", "spouse_subject_id",
			"spouse_is_external", "spouse_email_consent", "status", "source",
			"source_submission_id", "notes", "updated_at",
		}),
	}).Create(row).Error; err != nil {
		return nil, err
	}
	return r.GetBySubject(ctx, string(row.SubjectType), row.SubjectID)
}

func (r *weddingAnniversaryRepository) GetByID(ctx context.Context, id string) (*models.WeddingAnniversary, error) {
	var row models.WeddingAnniversary
	if err := r.db.WithContext(ctx).First(&row, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *weddingAnniversaryRepository) GetBySubject(ctx context.Context, subjectType, subjectID string) (*models.WeddingAnniversary, error) {
	var row models.WeddingAnniversary
	err := r.db.WithContext(ctx).
		First(&row, "subject_type = ? AND subject_id = ?", strings.TrimSpace(subjectType), strings.TrimSpace(subjectID)).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *weddingAnniversaryRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.WeddingAnniversary{}, "id = ?", strings.TrimSpace(id)).Error
}

func (r *weddingAnniversaryRepository) SetStatus(ctx context.Context, id, status string) error {
	if status != string(models.WeddingAnniversaryStatusActive) && status != string(models.WeddingAnniversaryStatusArchived) {
		return errors.New("status must be active or archived")
	}
	res := r.db.WithContext(ctx).Model(&models.WeddingAnniversary{}).
		Where("id = ?", strings.TrimSpace(id)).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

const weddingAnniversaryHydrateSelect = `
  wa.*,
  COALESCE(m.first_name, l.first_name, w.first_name, '') AS first_name,
  COALESCE(m.last_name,  l.last_name,  w.last_name,  '') AS last_name,
  COALESCE(m.email, l.email, w.email, '') AS email`

const weddingAnniversaryHydrateJoins = `
  FROM wedding_anniversaries wa
  LEFT JOIN members m            ON wa.subject_type = 'member'     AND wa.subject_id = m.id
  LEFT JOIN leadership_members l ON wa.subject_type = 'leadership' AND wa.subject_id = l.id
  LEFT JOIN workforce_members w  ON wa.subject_type = 'workforce'  AND wa.subject_id = w.id`

type weddingAnniversaryScanRow struct {
	models.WeddingAnniversary
	FirstName string
	LastName  string
	Email     string
}

func toView(rows []weddingAnniversaryScanRow) []models.WeddingAnniversaryView {
	out := make([]models.WeddingAnniversaryView, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.WeddingAnniversaryView{
			WeddingAnniversary: row.WeddingAnniversary,
			FirstName:          row.FirstName,
			LastName:           row.LastName,
			Email:              row.Email,
			NeedsSpouse:        strings.TrimSpace(row.SpouseName) == "",
		})
	}
	return out
}

func (r *weddingAnniversaryRepository) List(ctx context.Context, f WeddingAnniversaryFilter) ([]models.WeddingAnniversaryView, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.Month >= 1 && f.Month <= 12 {
		where = append(where, "wa.anniversary_month = ?")
		args = append(args, f.Month)
	}
	if s := strings.TrimSpace(f.Status); s != "" {
		where = append(where, "wa.status = ?")
		args = append(args, s)
	}
	if st := strings.TrimSpace(f.SubjectType); st != "" {
		where = append(where, "wa.subject_type = ?")
		args = append(args, st)
	}
	if f.NeedsSpouse {
		where = append(where, "COALESCE(TRIM(wa.spouse_name), '') = ''")
	}
	whereClause := strings.Join(where, " AND ")

	var total int64
	if err := r.db.WithContext(ctx).
		Raw("SELECT COUNT(*) "+weddingAnniversaryHydrateJoins+" WHERE "+whereClause, args...).
		Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	var rows []weddingAnniversaryScanRow
	q := "SELECT " + weddingAnniversaryHydrateSelect + weddingAnniversaryHydrateJoins +
		" WHERE " + whereClause + " ORDER BY wa.anniversary_month, wa.anniversary_day, last_name LIMIT ? OFFSET ?"
	if err := r.db.WithContext(ctx).Raw(q, append(args, limit, offset)...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return toView(rows), total, nil
}

func (r *weddingAnniversaryRepository) ListDueByMonthDay(ctx context.Context, month, day int) ([]models.WeddingAnniversaryView, error) {
	var rows []weddingAnniversaryScanRow
	q := "SELECT " + weddingAnniversaryHydrateSelect + weddingAnniversaryHydrateJoins +
		` WHERE wa.status = 'active' AND wa.anniversary_month = ? AND wa.anniversary_day = ?
		  AND (
		    (wa.subject_type = 'member'     AND m.is_active = true) OR
		    (wa.subject_type = 'leadership' AND l.status = 'approved') OR
		    (wa.subject_type = 'workforce'  AND w.status = 'serving')
		  )
		  AND COALESCE(m.email, l.email, w.email, '') <> ''`
	if err := r.db.WithContext(ctx).Raw(q, month, day).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return toView(rows), nil
}

func (r *weddingAnniversaryRepository) Stats(ctx context.Context) (*models.WeddingAnniversaryStats, error) {
	type monthRow struct {
		Month int
		Count int64
	}
	var monthRows []monthRow
	if err := r.db.WithContext(ctx).Model(&models.WeddingAnniversary{}).
		Select("anniversary_month as month, COUNT(*) as count").
		Where("status = ?", models.WeddingAnniversaryStatusActive).
		Group("anniversary_month").Scan(&monthRows).Error; err != nil {
		return nil, err
	}

	out := &models.WeddingAnniversaryStats{ByMonth: make([]models.BirthdayMonthCount, 0, 12)}
	byMonth := map[int]int64{}
	for _, m := range monthRows {
		if m.Month >= 1 && m.Month <= 12 {
			byMonth[m.Month] = m.Count
			out.Active += m.Count
		}
	}
	for month := 1; month <= 12; month++ {
		out.ByMonth = append(out.ByMonth, models.BirthdayMonthCount{Month: month, Count: byMonth[month]})
	}
	if err := r.db.WithContext(ctx).Model(&models.WeddingAnniversary{}).Count(&out.Total).Error; err != nil {
		return nil, err
	}
	out.Archived = out.Total - out.Active
	return out, nil
}

func valueOrEmptyPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
