package models

import "time"

type SermonVideo struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Thumbnail    string   `json:"thumbnail"`
	PublishedAt  string   `json:"publishedAt"`
	Duration     string   `json:"duration"`
	ViewCount    string   `json:"viewCount"`
	LikeCount    *string  `json:"likeCount,omitempty"`
	CommentCount *string  `json:"commentCount,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	URL          string   `json:"url"`
	EmbedURL     string   `json:"embedUrl"`
	Series       string   `json:"series"`
	Preacher     string   `json:"preacher"`
}

type SermonCollection struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Items       []SermonVideo `json:"items"`
}

type SermonDiscovery struct {
	Featured    *SermonVideo       `json:"featured,omitempty"`
	Recommended []SermonVideo      `json:"recommended"`
	Latest      []SermonVideo      `json:"latest"`
	Collections []SermonCollection `json:"collections"`
	Topics      []string           `json:"topics"`
	GeneratedAt time.Time          `json:"generatedAt"`
}
