package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"wisdomHouse-backend/internal/models"
)

const maxEmbeddedSubmissionMediaBytes = 50 * 1024 * 1024 // 50MB safety limit

var submissionDataURLRe = regexp.MustCompile(`(?i)^data:([^;,]+);base64,`)

func (s *formService) materializeSubmissionMedia(form *models.Form, values map[string]any) (map[string]any, error) {
	if values == nil {
		return map[string]any{}, nil
	}

	out := make(map[string]any, len(values))

	for key, value := range values {
		next, err := s.materializeSubmissionMediaValue(form, key, value)
		if err != nil {
			return nil, err
		}

		out[key] = next
	}

	if containsSubmissionDataURL(out) {
		return nil, fmt.Errorf("submission still contains embedded base64 media after upload processing")
	}

	return out, nil
}

func (s *formService) materializeSubmissionMediaValue(form *models.Form, key string, value any) (any, error) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return typed, nil
		}

		if isSubmissionDataURL(trimmed) {
			return s.uploadSubmissionDataURL(form, key, trimmed)
		}

		return typed, nil

	case map[string]any:
		if url := firstSubmissionMediaURLFromMap(typed); url != "" {
			return url, nil
		}

		next := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			materialized, err := s.materializeSubmissionMediaValue(form, childKey, childValue)
			if err != nil {
				return nil, err
			}

			next[childKey] = materialized
		}

		return next, nil

	case []any:
		next := make([]any, 0, len(typed))

		for index, childValue := range typed {
			materialized, err := s.materializeSubmissionMediaValue(
				form,
				fmt.Sprintf("%s_%d", key, index),
				childValue,
			)
			if err != nil {
				return nil, err
			}

			next = append(next, materialized)
		}

		return next, nil

	default:
		return value, nil
	}
}

func (s *formService) uploadSubmissionDataURL(form *models.Form, fieldKey string, dataURL string) (string, error) {
	if s.uploader == nil {
		return "", fmt.Errorf("cannot upload media field %q: storage uploader not configured", fieldKey)
	}

	dataURL = strings.TrimSpace(dataURL)

	match := submissionDataURLRe.FindStringSubmatch(dataURL)
	if len(match) < 2 {
		return "", fmt.Errorf("invalid media data URL for field %q", fieldKey)
	}

	contentType := strings.ToLower(strings.TrimSpace(match[1]))
	if !isAllowedSubmissionMediaContentType(contentType) {
		return "", fmt.Errorf("unsupported media content type %q for field %q", contentType, fieldKey)
	}

	commaIndex := strings.Index(dataURL, ",")
	if commaIndex < 0 || commaIndex+1 >= len(dataURL) {
		return "", fmt.Errorf("invalid media payload for field %q", fieldKey)
	}

	encodedPayload := strings.TrimSpace(dataURL[commaIndex+1:])
	encodedPayload = strings.ReplaceAll(encodedPayload, "\n", "")
	encodedPayload = strings.ReplaceAll(encodedPayload, "\r", "")
	encodedPayload = strings.ReplaceAll(encodedPayload, "\t", "")
	encodedPayload = strings.ReplaceAll(encodedPayload, " ", "")

	decoded, err := base64.StdEncoding.DecodeString(encodedPayload)
	if err != nil {
		return "", fmt.Errorf("invalid base64 media payload for field %q", fieldKey)
	}

	if len(decoded) == 0 {
		return "", fmt.Errorf("empty media payload for field %q", fieldKey)
	}

	if len(decoded) > maxEmbeddedSubmissionMediaBytes {
		return "", fmt.Errorf(
			"media field %q exceeds fallback upload size of %dMB",
			fieldKey,
			maxEmbeddedSubmissionMediaBytes/(1024*1024),
		)
	}

	ext, err := submissionMediaExt(contentType)
	if err != nil {
		return "", err
	}

	folder := buildSubmissionMediaFolder(form, contentType)

	objectKey, err := s.uploader.BuildGenericAssetKey(folder, ext)
	if err != nil {
		return "", fmt.Errorf("failed to build storage object key for field %q: %w", fieldKey, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	publicURL, err := s.uploader.Upload(ctx, objectKey, contentType, bytes.NewReader(decoded))
	if err != nil {
		return "", fmt.Errorf("failed to upload media field %q to storage: %w", fieldKey, err)
	}

	publicURL = strings.TrimSpace(publicURL)
	if publicURL == "" {
		return "", fmt.Errorf("storage returned empty public URL for field %q", fieldKey)
	}

	return publicURL, nil
}

func buildSubmissionMediaFolder(form *models.Form, contentType string) string {
	formRef := "unknown-form"

	if form != nil {
		if slug := submissionStringPtrValue(form.Slug); slug != "" {
			formRef = sanitizeSubmissionStorageSegment(slug)
		} else if id := strings.TrimSpace(fmt.Sprint(form.ID)); id != "" {
			formRef = sanitizeSubmissionStorageSegment(id)
		}
	}

	kind := "files"
	contentType = strings.ToLower(strings.TrimSpace(contentType))

	if strings.HasPrefix(contentType, "image/") {
		kind = "images"
	} else if strings.HasPrefix(contentType, "video/") {
		kind = "videos"
	} else if strings.HasPrefix(contentType, "audio/") {
		kind = "audio"
	} else if contentType == "application/pdf" || strings.HasPrefix(contentType, "text/") {
		kind = "documents"
	}

	return "public-forms/" + formRef + "/" + kind
}

func submissionStringPtrValue(value *string) string {
	if value == nil {
		return ""
	}

	return strings.TrimSpace(*value)
}

// Important: this name is intentionally different from sanitizeStorageSegment
// in spaces_uploader.go. All files in internal/service share one package scope.
func sanitizeSubmissionStorageSegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}

	var b strings.Builder

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}

	clean := strings.Trim(b.String(), "-_")
	for strings.Contains(clean, "--") {
		clean = strings.ReplaceAll(clean, "--", "-")
	}

	if clean == "" {
		return "unknown"
	}

	return clean
}

func firstSubmissionMediaURLFromMap(record map[string]any) string {
	keys := []string{
		"publicUrl",
		"public_url",
		"url",
		"imageUrl",
		"image_url",
		"photoUrl",
		"photo_url",
		"src",
	}

	for _, key := range keys {
		raw, ok := record[key]
		if !ok {
			continue
		}

		value := strings.TrimSpace(fmt.Sprint(raw))
		if isSubmissionHTTPURL(value) {
			return value
		}
	}

	return ""
}

func containsSubmissionDataURL(values map[string]any) bool {
	for _, value := range values {
		if containsSubmissionDataURLValue(value) {
			return true
		}
	}

	return false
}

func containsSubmissionDataURLValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return isSubmissionDataURL(typed)

	case map[string]any:
		for _, child := range typed {
			if containsSubmissionDataURLValue(child) {
				return true
			}
		}

	case []any:
		for _, child := range typed {
			if containsSubmissionDataURLValue(child) {
				return true
			}
		}
	}

	return false
}

func isSubmissionDataURL(value string) bool {
	return submissionDataURLRe.MatchString(strings.TrimSpace(value))
}

func isSubmissionHTTPURL(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func isAllowedSubmissionMediaContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/webp",
		"image/gif",
		"video/mp4",
		"video/webm",
		"video/quicktime",
		"audio/mpeg",
		"audio/mp4",
		"audio/wav",
		"audio/x-wav",
		"audio/webm",
		"audio/ogg",
		"application/pdf",
		"text/plain",
		"text/csv",
		"application/msword",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return true
	default:
		return false
	}
}

func submissionMediaExt(contentType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return "jpg", nil
	case "image/png":
		return "png", nil
	case "image/webp":
		return "webp", nil
	case "image/gif":
		return "gif", nil
	case "video/mp4":
		return "mp4", nil
	case "video/webm":
		return "webm", nil
	case "video/quicktime":
		return "mov", nil
	case "audio/mpeg":
		return "mp3", nil
	case "audio/mp4":
		return "m4a", nil
	case "audio/wav", "audio/x-wav":
		return "wav", nil
	case "audio/webm":
		return "webm", nil
	case "audio/ogg":
		return "ogg", nil
	case "application/pdf":
		return "pdf", nil
	case "text/plain":
		return "txt", nil
	case "text/csv":
		return "csv", nil
	case "application/msword":
		return "doc", nil
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx", nil
	case "application/vnd.ms-excel":
		return "xls", nil
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx", nil
	default:
		return "", fmt.Errorf("unsupported media content type %q", contentType)
	}
}
