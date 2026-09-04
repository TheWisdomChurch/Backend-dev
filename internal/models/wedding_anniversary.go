package models

import "time"

// WeddingAnniversarySubjectType identifies which people-table the greeted
// person lives in. The three tables have no shared identity, so a wedding
// anniversary points at one of them explicitly.
type WeddingAnniversarySubjectType string

const (
	WeddingAnniversarySubjectMember     WeddingAnniversarySubjectType = "member"
	WeddingAnniversarySubjectLeadership WeddingAnniversarySubjectType = "leadership"
	WeddingAnniversarySubjectWorkforce  WeddingAnniversarySubjectType = "workforce"
)

func ValidWeddingAnniversarySubjectType(v string) bool {
	switch WeddingAnniversarySubjectType(v) {
	case WeddingAnniversarySubjectMember, WeddingAnniversarySubjectLeadership, WeddingAnniversarySubjectWorkforce:
		return true
	default:
		return false
	}
}

// WeddingAnniversaryStatus gates the greeting automation. "archived" is the
// pastoral escape hatch — divorce, separation, bereavement — the row is kept
// for history but never triggers an email.
type WeddingAnniversaryStatus string

const (
	WeddingAnniversaryStatusActive   WeddingAnniversaryStatus = "active"
	WeddingAnniversaryStatusArchived WeddingAnniversaryStatus = "archived"
)

// WeddingAnniversarySource records where a row came from, for audit and
// conflict resolution (admin edits always win over form/import writes).
type WeddingAnniversarySource string

const (
	WeddingAnniversarySourceAdmin  WeddingAnniversarySource = "admin"
	WeddingAnniversarySourceForm   WeddingAnniversarySource = "form"
	WeddingAnniversarySourceImport WeddingAnniversarySource = "import"
	WeddingAnniversarySourceCSV    WeddingAnniversarySource = "csv"
)

// WeddingAnniversary is the single record of one marriage as it concerns the
// church: who we greet, when, who they're married to, and whether we're still
// allowed to reach out. One row per marriage — when both spouses are in our
// system the row links them via SpouseSubject* and the mirror row is merged
// away on upsert.
type WeddingAnniversary struct {
	ID string `gorm:"type:uuid;default:gen_random_uuid();primaryKey" json:"id"`

	SubjectType WeddingAnniversarySubjectType `gorm:"size:20;not null;uniqueIndex:ux_wedding_anniversary_subject" json:"subjectType"`
	SubjectID   string                        `gorm:"type:uuid;not null;uniqueIndex:ux_wedding_anniversary_subject" json:"subjectId"`

	AnniversaryMonth int  `gorm:"type:smallint;not null;index:idx_wedding_anniversary_due" json:"anniversaryMonth"`
	AnniversaryDay   int  `gorm:"type:smallint;not null;index:idx_wedding_anniversary_due" json:"anniversaryDay"`
	WeddingYear      *int `gorm:"type:smallint" json:"weddingYear,omitempty"` // reserved; not used yet

	SpouseName         string  `gorm:"size:200;not null;default:''" json:"spouseName"`
	SpouseEmail        *string `gorm:"size:255" json:"spouseEmail,omitempty"`
	SpouseSubjectType  *string `gorm:"size:20" json:"spouseSubjectType,omitempty"`
	SpouseSubjectID    *string `gorm:"type:uuid" json:"spouseSubjectId,omitempty"`
	SpouseIsExternal   bool    `gorm:"not null;default:false" json:"spouseIsExternal"`
	SpouseEmailConsent bool    `gorm:"not null;default:false" json:"spouseEmailConsent"`

	Status WeddingAnniversaryStatus `gorm:"size:20;not null;default:'active';index:idx_wedding_anniversary_due" json:"status"`

	Source             WeddingAnniversarySource `gorm:"size:20;not null;default:'admin'" json:"source"`
	SourceSubmissionID *string                  `gorm:"type:uuid" json:"sourceSubmissionId,omitempty"`
	Notes              *string                  `gorm:"type:text" json:"notes,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

func (WeddingAnniversary) TableName() string { return "wedding_anniversaries" }

// WeddingAnniversaryInput is the shared write payload — from admin CRUD, form
// submission mapping, or CSV import. Either (Month,Day) or the raw "DD/MM"
// Anniversary string must be provided.
type WeddingAnniversaryInput struct {
	AnniversaryMonth *int    `json:"anniversaryMonth,omitempty"`
	AnniversaryDay   *int    `json:"anniversaryDay,omitempty"`
	Anniversary      *string `json:"anniversary,omitempty"` // "DD/MM" or "DD/MM/YYYY" (year ignored)

	SpouseName         string  `json:"spouseName"`
	SpouseEmail        *string `json:"spouseEmail,omitempty"`
	SpouseEmailConsent bool    `json:"spouseEmailConsent"`
	SpouseIsExternal   *bool   `json:"spouseIsExternal,omitempty"`
	Notes              *string `json:"notes,omitempty"`
}

// WeddingAnniversaryView is a read row joined to the greeted person's identity.
type WeddingAnniversaryView struct {
	WeddingAnniversary
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	Email        string `json:"email"`
	NeedsSpouse  bool   `json:"needsSpouse"` // true when SpouseName is blank (backfilled rows)
}

// WeddingAnniversaryStats mirrors BirthdayStatsResponse for the admin dashboard.
type WeddingAnniversaryStats struct {
	Total    int64                `json:"total"`
	Active   int64                `json:"active"`
	Archived int64                `json:"archived"`
	ByMonth  []BirthdayMonthCount `json:"byMonth"`
}
