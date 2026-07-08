package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type UploadHandler struct {
	storage service.AssetUploader
	assets  service.AssetService
}

func NewUploadHandler(storage service.AssetUploader, assets ...service.AssetService) *UploadHandler {
	h := &UploadHandler{storage: storage}
	if len(assets) > 0 {
		h.assets = assets[0]
	}
	return h
}

func (h *UploadHandler) UploadImage(c *gin.Context) {
	h.uploadFile(c, "image", 10<<20)
}

func (h *UploadHandler) UploadFile(c *gin.Context) {
	kind := normalizeUploadKind(c.DefaultPostForm("kind", "file"))

	switch kind {
	case "image":
		h.uploadFile(c, "image", 10<<20)
	case "document":
		h.uploadFile(c, "document", 25<<20)
	case "audio":
		h.uploadFile(c, "audio", 100<<20)
	case "video":
		h.uploadFile(c, "video", 250<<20) // 250MB; very large media should still use presigned upload
	default:
		h.uploadFile(c, "file", 25<<20)
	}
}

func (h *UploadHandler) uploadFile(c *gin.Context, forcedKind string, maxBytes int64) {
	if h.storage == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Storage uploader not configured")
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is required")
		return
	}

	if fh.Size <= 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is empty")
		return
	}

	if fh.Size > maxBytes {
		utils.ErrorResponse(c, http.StatusBadRequest, "file too large")
		return
	}

	src, err := fh.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer src.Close()

	contentType := strings.TrimSpace(fh.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = contentTypeFromFilename(fh.Filename)
	}

	kind := normalizeUploadKind(firstNonEmpty(forcedKind, c.PostForm("kind"), kindFromContentType(contentType)))
	if !allowedUploadContentType(kind, contentType) {
		utils.ErrorResponse(c, http.StatusBadRequest, "unsupported file type")
		return
	}

	ext := extFromUploadContentType(contentType, fh.Filename)
	if ext == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "unsupported file extension")
		return
	}

	module := sanitizeAssetSegment(c.DefaultPostForm("module", c.DefaultPostForm("ownerType", "uploads")))
	ownerType := strings.ToLower(strings.TrimSpace(c.DefaultPostForm("ownerType", module)))
	ownerID := strings.TrimSpace(c.DefaultPostForm("ownerId", c.DefaultPostForm("relatedId", "")))
	folder := sanitizeAssetFolder(c.DefaultPostForm("folder", defaultAssetFolder(module, kind)))

	objectKey, err := h.storage.BuildGenericAssetKey(folder, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	hasher := sha256.New()
	reader := io.TeeReader(src, hasher)

	ctx, cancel := context.WithTimeout(c.Request.Context(), uploadTimeout(kind))
	defer cancel()

	publicURL, err := h.storage.Upload(ctx, objectKey, contentType, reader)
	if err != nil {
		applog.L().Warn("asset upload failed", "module", module, "kind", kind, "folder", folder, "key", objectKey, "content_type", contentType, "size", fh.Size, "error", err)
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	checksum := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	originalName := filepath.Base(fh.Filename)

	var asset *models.Asset
	if h.assets != nil {
		recordReq := &models.RecordUploadedAssetRequest{
			OwnerType:    nilIfEmptyString(ownerType),
			OwnerID:      nilIfEmptyString(ownerID),
			Kind:         nilIfEmptyString(kind),
			Folder:       nilIfEmptyString(folder),
			ObjectKey:    objectKey,
			PublicURL:    publicURL,
			ContentType:  contentType,
			SizeBytes:    fh.Size,
			Checksum:     &checksum,
			OriginalName: &originalName,
		}

		asset, err = h.assets.RecordUploadedAsset(recordReq, nil)
		if err != nil {
			applog.L().Warn("asset metadata record failed", "key", objectKey, "url", publicURL, "error", err)
			utils.ErrorResponse(c, http.StatusInternalServerError, "file uploaded but metadata save failed")
			return
		}
	}

	resp := gin.H{
		"folder":       folder,
		"key":          objectKey,
		"objectKey":    objectKey,
		"url":          publicURL,
		"publicUrl":    publicURL,
		"contentType":  contentType,
		"mimeType":     contentType,
		"sizeBytes":    fh.Size,
		"kind":         kind,
		"module":       module,
		"ownerType":    ownerType,
		"ownerId":      ownerID,
		"checksum":     checksum,
		"originalName": originalName,
	}

	if asset != nil {
		resp["id"] = asset.ID
		resp["assetId"] = asset.ID
		resp["bucket"] = asset.Bucket
		resp["provider"] = asset.Provider
		resp["status"] = string(asset.Status)
	}

	utils.SuccessResponse(c, http.StatusOK, "Upload successful", resp)
}

func normalizeUploadKind(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "image", "photo", "picture", "avatar", "thumbnail", "banner", "passport":
		return "image"
	case "video", "movie", "reel", "clip":
		return "video"
	case "audio", "sound", "voice":
		return "audio"
	case "document", "pdf", "doc", "docs", "docx", "sheet", "spreadsheet", "csv", "xls", "xlsx", "txt":
		return "document"
	case "file":
		return "file"
	default:
		return "file"
	}
}

func allowedUploadContentType(kind, ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))

	switch kind {
	case "image":
		return ct == "image/png" ||
			ct == "image/jpeg" ||
			ct == "image/jpg" ||
			ct == "image/webp" ||
			ct == "image/gif"
	case "document":
		return ct == "application/pdf" ||
			ct == "application/msword" ||
			ct == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" ||
			ct == "application/vnd.ms-excel" ||
			ct == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
			ct == "text/csv" ||
			ct == "text/plain"
	case "audio":
		return ct == "audio/mpeg" ||
			ct == "audio/mp4" ||
			ct == "audio/wav" ||
			ct == "audio/x-wav" ||
			ct == "audio/aac" ||
			ct == "audio/ogg" ||
			ct == "audio/webm"
	case "video":
		return ct == "video/mp4" ||
			ct == "video/webm" ||
			ct == "video/quicktime" ||
			ct == "video/x-msvideo" ||
			ct == "video/x-matroska" ||
			ct == "video/mpeg"
	default:
		return allowedUploadContentType("image", ct) ||
			allowedUploadContentType("document", ct) ||
			allowedUploadContentType("audio", ct) ||
			allowedUploadContentType("video", ct)
	}
}

func extFromUploadContentType(ct, filename string) string {
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "application/pdf":
		return "pdf"
	case "application/msword":
		return "doc"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.ms-excel":
		return "xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "text/csv":
		return "csv"
	case "text/plain":
		return "txt"
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4":
		return "m4a"
	case "audio/wav", "audio/x-wav":
		return "wav"
	case "video/mp4":
		return "mp4"
	case "video/webm":
		return "webm"
	case "video/quicktime":
		return "mov"
	case "video/x-msvideo":
		return "avi"
	case "video/x-matroska":
		return "mkv"
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".")
	switch ext {
	case "png", "jpg", "jpeg", "webp", "gif", "pdf", "doc", "docx", "xls", "xlsx", "csv", "txt", "mp3", "m4a", "wav", "mp4", "webm", "mov", "avi", "mkv":
		if ext == "jpeg" {
			return "jpg"
		}
		return ext
	default:
		return ""
	}
}

func contentTypeFromFilename(filename string) string {
	switch strings.TrimPrefix(strings.ToLower(filepath.Ext(filename)), ".") {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	case "pdf":
		return "application/pdf"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "csv":
		return "text/csv"
	case "txt":
		return "text/plain"
	case "mp3":
		return "audio/mpeg"
	case "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mov":
		return "video/quicktime"
	default:
		return "application/octet-stream"
	}
}

func kindFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case ct == "application/pdf" || strings.Contains(ct, "word") || strings.Contains(ct, "excel") || strings.HasPrefix(ct, "text/"):
		return "document"
	default:
		return "file"
	}
}

func defaultAssetFolder(module, kind string) string {
	module = sanitizeAssetSegment(module)
	kind = sanitizeAssetSegment(kind)
	if module == "" {
		module = "uploads"
	}
	if kind == "" {
		kind = "file"
	}
	return module + "/" + kind
}

func sanitizeAssetFolder(v string) string {
	v = strings.Trim(strings.ToLower(strings.TrimSpace(v)), "/")
	if v == "" {
		return "uploads/file"
	}

	parts := strings.Split(v, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := sanitizeAssetSegment(part); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 {
		return "uploads/file"
	}
	return strings.Join(clean, "/")
}

func sanitizeAssetSegment(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	var b strings.Builder
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '/':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-_")
}

func uploadTimeout(kind string) time.Duration {
	switch kind {
	case "video", "audio":
		return 90 * time.Second
	default:
		return 30 * time.Second
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nilIfEmptyString(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
