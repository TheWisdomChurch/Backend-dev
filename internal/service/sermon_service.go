package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
)

type SermonFilter struct {
	Search   string
	Series   string
	Preacher string
	Sort     string
	Limit    int
}

type SermonService interface {
	List(filter SermonFilter) ([]models.SermonVideo, error)
}

type sermonService struct {
	httpClient *http.Client
}

func NewSermonService() SermonService {
	return &sermonService{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (s *sermonService) List(filter SermonFilter) ([]models.SermonVideo, error) {
	apiKey := strings.TrimSpace(os.Getenv("YOUTUBE_API_KEY"))
	if apiKey == "" {
		return []models.SermonVideo{}, nil
	}
	channelID := strings.TrimSpace(os.Getenv("YOUTUBE_CHANNEL_ID"))
	if channelID == "" {
		channelID = "UCJuXOj075x81CYK-cCuXwdg"
	}
	limit := filter.Limit
	if limit <= 0 || limit > 50 {
		limit = 50
	}

	searchURL := "https://www.googleapis.com/youtube/v3/search?" + url.Values{
		"part":      []string{"snippet"},
		"type":      []string{"video"},
		"channelId": []string{channelID},
		"maxResults": []string{fmt.Sprintf("%d", limit)},
		"order":     []string{"date"},
		"key":       []string{apiKey},
	}.Encode()

	searchResp, err := s.httpClient.Get(searchURL)
	if err != nil {
		return nil, err
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode < 200 || searchResp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtube search request failed: status %d", searchResp.StatusCode)
	}

	var searchPayload struct {
		Items []struct {
			ID struct {
				VideoID string `json:"videoId"`
			} `json:"id"`
			Snippet struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				PublishedAt string `json:"publishedAt"`
				Thumbnails  struct {
					High struct {
						URL string `json:"url"`
					} `json:"high"`
					Medium struct {
						URL string `json:"url"`
					} `json:"medium"`
					Default struct {
						URL string `json:"url"`
					} `json:"default"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(searchResp.Body).Decode(&searchPayload); err != nil {
		return nil, err
	}

	videoIDs := make([]string, 0, len(searchPayload.Items))
	for _, item := range searchPayload.Items {
		if item.ID.VideoID != "" {
			videoIDs = append(videoIDs, item.ID.VideoID)
		}
	}
	if len(videoIDs) == 0 {
		return []models.SermonVideo{}, nil
	}

	detailsURL := "https://www.googleapis.com/youtube/v3/videos?" + url.Values{
		"part": []string{"contentDetails,statistics,snippet"},
		"id":   []string{strings.Join(videoIDs, ",")},
		"key":  []string{apiKey},
	}.Encode()

	detailsResp, err := s.httpClient.Get(detailsURL)
	if err != nil {
		return nil, err
	}
	defer detailsResp.Body.Close()
	if detailsResp.StatusCode < 200 || detailsResp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtube details request failed: status %d", detailsResp.StatusCode)
	}

	var detailsPayload struct {
		Items []struct {
			ID            string `json:"id"`
			ContentDetails struct {
				Duration string `json:"duration"`
			} `json:"contentDetails"`
			Statistics struct {
				ViewCount    string `json:"viewCount"`
				LikeCount    string `json:"likeCount"`
				CommentCount string `json:"commentCount"`
			} `json:"statistics"`
			Snippet struct {
				Tags []string `json:"tags"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := json.NewDecoder(detailsResp.Body).Decode(&detailsPayload); err != nil {
		return nil, err
	}

	detailMap := make(map[string]struct {
		Duration     string
		ViewCount    string
		LikeCount    string
		CommentCount string
		Tags         []string
	}, len(detailsPayload.Items))
	for _, item := range detailsPayload.Items {
		detailMap[item.ID] = struct {
			Duration     string
			ViewCount    string
			LikeCount    string
			CommentCount string
			Tags         []string
		}{
			Duration:     item.ContentDetails.Duration,
			ViewCount:    item.Statistics.ViewCount,
			LikeCount:    item.Statistics.LikeCount,
			CommentCount: item.Statistics.CommentCount,
			Tags:         item.Snippet.Tags,
		}
	}

	out := make([]models.SermonVideo, 0, len(searchPayload.Items))
	for _, item := range searchPayload.Items {
		meta := extractSermonMetadata(item.Snippet.Title)
		detail := detailMap[item.ID.VideoID]
		thumb := item.Snippet.Thumbnails.High.URL
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Medium.URL
		}
		if thumb == "" {
			thumb = item.Snippet.Thumbnails.Default.URL
		}

		like := strPtr(detail.LikeCount)
		comment := strPtr(detail.CommentCount)

		out = append(out, models.SermonVideo{
			ID:           item.ID.VideoID,
			Title:        item.Snippet.Title,
			Description:  item.Snippet.Description,
			Thumbnail:    thumb,
			PublishedAt:  item.Snippet.PublishedAt,
			Duration:     formatYouTubeDuration(detail.Duration),
			ViewCount:    zeroIfEmpty(detail.ViewCount),
			LikeCount:    like,
			CommentCount: comment,
			Tags:         detail.Tags,
			URL:          "https://www.youtube.com/watch?v=" + item.ID.VideoID,
			EmbedURL:     "https://www.youtube.com/embed/" + item.ID.VideoID,
			Series:       meta.series,
			Preacher:     meta.preacher,
		})
	}

	out = filterSermons(out, filter)
	sortSermons(out, filter.Sort)
	return out, nil
}

func filterSermons(items []models.SermonVideo, filter SermonFilter) []models.SermonVideo {
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	series := strings.ToLower(strings.TrimSpace(filter.Series))
	preacher := strings.ToLower(strings.TrimSpace(filter.Preacher))
	if search == "" && series == "" && preacher == "" {
		return items
	}

	filtered := make([]models.SermonVideo, 0, len(items))
	for _, v := range items {
		if search != "" {
			if !strings.Contains(strings.ToLower(v.Title), search) &&
				!strings.Contains(strings.ToLower(v.Description), search) &&
				!strings.Contains(strings.ToLower(v.Series), search) &&
				!strings.Contains(strings.ToLower(v.Preacher), search) {
				continue
			}
		}
		if series != "" && series != "all" && !strings.Contains(strings.ToLower(v.Series), series) {
			continue
		}
		if preacher != "" && preacher != "all" && !strings.Contains(strings.ToLower(v.Preacher), preacher) {
			continue
		}
		filtered = append(filtered, v)
	}
	return filtered
}

func sortSermons(items []models.SermonVideo, sortBy string) {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "oldest":
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].PublishedAt < items[j].PublishedAt
		})
	case "popular":
		sort.SliceStable(items, func(i, j int) bool {
			return atoi(items[i].ViewCount) > atoi(items[j].ViewCount)
		})
	default:
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].PublishedAt > items[j].PublishedAt
		})
	}
}

func formatYouTubeDuration(raw string) string {
	if raw == "" {
		return "N/A"
	}
	raw = strings.TrimPrefix(raw, "PT")
	hours, minutes, seconds := 0, 0, 0
	var cur strings.Builder
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			cur.WriteRune(ch)
			continue
		}
		switch ch {
		case 'H':
			hours = atoi(cur.String())
		case 'M':
			minutes = atoi(cur.String())
		case 'S':
			seconds = atoi(cur.String())
		}
		cur.Reset()
	}
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

type sermonMetadata struct {
	series   string
	preacher string
}

func extractSermonMetadata(title string) sermonMetadata {
	out := sermonMetadata{series: "General", preacher: "Wisdom House Ministry"}
	parts := strings.Split(title, "|")
	if len(parts) >= 2 {
		series := strings.TrimSpace(parts[0])
		preacher := strings.TrimSpace(parts[1])
		if series != "" {
			out.series = series
		}
		if preacher != "" {
			out.preacher = strings.TrimSpace(strings.ReplaceAll(preacher, "THE ", ""))
		}
	}
	return out
}

func atoi(raw string) int {
	v := 0
	for _, ch := range raw {
		if ch >= '0' && ch <= '9' {
			v = v*10 + int(ch-'0')
		}
	}
	return v
}

func strPtr(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	out := strings.TrimSpace(v)
	return &out
}

func zeroIfEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return "0"
	}
	return strings.TrimSpace(v)
}

