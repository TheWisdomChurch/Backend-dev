package handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"

	"wisdomHouse-backend/internal/jobs"
	applog "wisdomHouse-backend/internal/logger"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type UploadHandler struct {
	storage    service.AssetUploader
	assets     service.AssetService
	images     service.ImageProcessor
	videos     service.VideoProcessor
	videoQueue *asynq.Client
}

func NewUploadHandler(storage service.AssetUploader, assets ...service.AssetService) *UploadHandler {
	h := &UploadHandler{storage: storage, images: service.NewImageProcessor(), videos: service.NewVideoProcessor()}
	if len(assets) > 0 {
		h.assets = assets[0]
	}
	return h
}

// SetVideoQueue wires the async transcode queue in after construction —
// separate from NewUploadHandler so callers that don't need video
// processing (tests, presign-only setups) aren't forced to configure asynq.
func (h *UploadHandler) SetVideoQueue(client *asynq.Client) {
	h.videoQueue = client
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), uploadTimeout(kind))
	defer cancel()

	if kind == "image" && h.images != nil {
		h.uploadImage(ctx, c, src, fh, contentType, module, ownerType, ownerID, folder)
		return
	}

	if kind == "video" && h.videos != nil && h.videos.Available() {
		h.uploadVideo(ctx, c, src, fh, contentType, ext, module, ownerType, ownerID, folder)
		return
	}

	objectKey, err := h.storage.BuildGenericAssetKey(folder, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	// Peek the header before streaming to storage: catches a spoofed
	// Content-Type (e.g. an .exe renamed to "invoice.pdf") without buffering
	// the whole file — Peek doesn't consume the bytes, so the same reader
	// still yields them for the actual upload below.
	buffered := bufio.NewReaderSize(src, documentSniffLen)
	if kind == "document" {
		header, _ := buffered.Peek(documentSniffLen)
		if !validateDocumentMagicBytes(contentType, header) {
			applog.L().Warn("document failed magic-byte validation", "module", module, "content_type", contentType, "filename", fh.Filename)
			utils.ErrorResponse(c, http.StatusBadRequest, "file content does not match its declared type")
			return
		}
	}

	hasher := sha256.New()
	reader := io.TeeReader(buffered, hasher)

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

// uploadImage runs the shared multi-size pipeline: decode + validate the
// real image bytes (not just the client's claimed Content-Type), strip EXIF,
// auto-correct orientation, and produce the original plus every variant in
// the size ladder that's smaller than the source. Every produced size is
// uploaded under one shared assetID so the set stays discoverable together.
func (h *UploadHandler) uploadImage(
	ctx context.Context,
	c *gin.Context,
	src multipart.File,
	fh *multipart.FileHeader,
	claimedContentType string,
	module, ownerType, ownerID, folder string,
) {
	data, err := io.ReadAll(src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to read file")
		return
	}

	sum := sha256.Sum256(data)
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	originalName := filepath.Base(fh.Filename)

	set, err := h.images.Process(data)
	if err != nil {
		// A decode failure here means the bytes are not actually a valid
		// image, regardless of what Content-Type the client claimed —
		// real validation the old byte-passthrough path never did.
		applog.L().Warn("image processing failed", "module", module, "folder", folder, "content_type", claimedContentType, "size", fh.Size, "error", err)
		utils.ErrorResponse(c, http.StatusBadRequest, "file is not a valid, processable image")
		return
	}

	assetID := h.storage.NewAssetID()
	ext := extFromContentType(set.Original.ContentType)

	upload := func(variant string, img service.ProcessedImage) (string, error) {
		key, err := h.storage.BuildImageVariantKey(folder, assetID, variant, ext)
		if err != nil {
			return "", err
		}
		return h.storage.Upload(ctx, key, img.ContentType, bytes.NewReader(img.Bytes))
	}

	originalURL, err := upload("original", set.Original)
	if err != nil {
		applog.L().Warn("image upload failed", "module", module, "folder", folder, "asset_id", assetID, "variant", "original", "error", err)
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	type variantOut struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	variantURLs := make(map[string]variantOut, len(set.Variants))
	for name, img := range set.Variants {
		url, err := upload(string(name), img)
		if err != nil {
			// One variant failing to upload shouldn't fail the whole
			// request — the original is already safely stored, and every
			// consumer of this response falls back to a larger size (or
			// the original) when a specific variant is absent.
			applog.L().Warn("image variant upload failed", "module", module, "folder", folder, "asset_id", assetID, "variant", name, "error", err)
			continue
		}
		variantURLs[string(name)] = variantOut{URL: url, Width: img.Width, Height: img.Height}
	}

	var asset *models.Asset
	if h.assets != nil {
		sizeBytes := int64(len(set.Original.Bytes))
		recordReq := &models.RecordUploadedAssetRequest{
			OwnerType:    nilIfEmptyString(ownerType),
			OwnerID:      nilIfEmptyString(ownerID),
			Kind:         nilIfEmptyString("image"),
			Folder:       nilIfEmptyString(folder),
			ObjectKey:    originalURL,
			PublicURL:    originalURL,
			ContentType:  set.Original.ContentType,
			SizeBytes:    sizeBytes,
			Checksum:     &checksum,
			OriginalName: &originalName,
			Metadata: map[string]any{
				"width":    set.OriginalWidth,
				"height":   set.OriginalHeight,
				"variants": variantURLs,
			},
		}
		var err error
		asset, err = h.assets.RecordUploadedAsset(recordReq, nil)
		if err != nil {
			applog.L().Warn("asset metadata record failed", "url", originalURL, "error", err)
			utils.ErrorResponse(c, http.StatusInternalServerError, "file uploaded but metadata save failed")
			return
		}
	}

	resp := gin.H{
		"folder":       folder,
		"url":          originalURL,
		"publicUrl":    originalURL,
		"contentType":  set.Original.ContentType,
		"mimeType":     set.Original.ContentType,
		"sizeBytes":    len(set.Original.Bytes),
		"kind":         "image",
		"module":       module,
		"ownerType":    ownerType,
		"ownerId":      ownerID,
		"checksum":     checksum,
		"originalName": originalName,
		"width":        set.OriginalWidth,
		"height":       set.OriginalHeight,
		"variants":     variantURLs,
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

// uploadVideo buffers the upload to a temp file (never the full file in
// memory — these can be up to 250MB), validates it's a real, decodable video
// via ffprobe, stores the original immediately so nothing is lost, extracts
// a poster frame for instant preview, then hands the file off to the async
// transcode queue rather than blocking the request on a multi-minute
// ffmpeg run. The client gets back a "processing" status and can poll
// GET /admin/uploads/:id (already wired) for the transcoded URL.
func (h *UploadHandler) uploadVideo(
	ctx context.Context,
	c *gin.Context,
	src multipart.File,
	fh *multipart.FileHeader,
	claimedContentType, ext string,
	module, ownerType, ownerID, folder string,
) {
	tmp, err := os.CreateTemp("", "wisdom-video-upload-*")
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to buffer upload")
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(src, hasher))
	closeErr := tmp.Close()
	if err != nil || closeErr != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to buffer upload")
		return
	}

	meta, err := h.videos.Probe(ctx, tmpPath)
	if err != nil {
		applog.L().Warn("video probe failed", "module", module, "folder", folder, "content_type", claimedContentType, "size", written, "error", err)
		utils.ErrorResponse(c, http.StatusBadRequest, "file is not a valid, processable video")
		return
	}

	assetID := h.storage.NewAssetID()
	checksum := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	originalName := filepath.Base(fh.Filename)

	originalKey, err := h.storage.BuildImageVariantKey(folder, assetID, "original", ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}
	origFile, err := os.Open(tmpPath)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to read buffered upload")
		return
	}
	originalURL, err := h.storage.Upload(ctx, originalKey, claimedContentType, origFile)
	origFile.Close()
	if err != nil {
		applog.L().Warn("video upload failed", "module", module, "folder", folder, "asset_id", assetID, "error", err)
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	var posterURL string
	if posterBytes, err := h.videos.ExtractPosterFrame(ctx, tmpPath, meta.DurationSeconds); err != nil {
		// Poster is a nice-to-have, not essential — the original video is
		// already safely stored, so a poster failure doesn't fail the
		// whole upload. The client falls back to a placeholder thumbnail.
		applog.L().Warn("poster frame extraction failed", "asset_id", assetID, "error", err)
	} else if posterKey, err := h.storage.BuildImageVariantKey(folder, assetID, "poster", "jpg"); err == nil {
		if url, err := h.storage.Upload(ctx, posterKey, "image/jpeg", bytes.NewReader(posterBytes)); err == nil {
			posterURL = url
		} else {
			applog.L().Warn("poster upload failed", "asset_id", assetID, "error", err)
		}
	}

	status := models.AssetStatusReady
	queued := false
	if h.videoQueue != nil {
		task, taskErr := jobs.NewVideoProcessTask(jobs.VideoProcessPayload{
			AssetID:   assetID,
			Folder:    folder,
			SourceURL: originalURL,
		})
		if taskErr == nil {
			if _, enqErr := h.videoQueue.Enqueue(task); enqErr == nil {
				status = models.AssetStatusPending
				queued = true
			} else {
				applog.L().Warn("failed to enqueue video transcode", "asset_id", assetID, "error", enqErr)
			}
		} else {
			applog.L().Warn("failed to build video transcode task", "asset_id", assetID, "error", taskErr)
		}
	}

	var asset *models.Asset
	if h.assets != nil {
		metadata := map[string]any{
			"width":    meta.Width,
			"height":   meta.Height,
			"duration": meta.DurationSeconds,
		}
		if posterURL != "" {
			metadata["poster"] = posterURL
		}
		recordReq := &models.RecordUploadedAssetRequest{
			ID:           &assetID,
			OwnerType:    nilIfEmptyString(ownerType),
			OwnerID:      nilIfEmptyString(ownerID),
			Kind:         nilIfEmptyString("video"),
			Folder:       nilIfEmptyString(folder),
			ObjectKey:    originalURL,
			PublicURL:    originalURL,
			ContentType:  claimedContentType,
			SizeBytes:    written,
			Checksum:     &checksum,
			OriginalName: &originalName,
			Metadata:     metadata,
			Status:       &status,
		}
		var recErr error
		asset, recErr = h.assets.RecordUploadedAsset(recordReq, nil)
		if recErr != nil {
			applog.L().Warn("video asset metadata record failed", "url", originalURL, "error", recErr)
			utils.ErrorResponse(c, http.StatusInternalServerError, "file uploaded but metadata save failed")
			return
		}
	}

	resp := gin.H{
		"folder":       folder,
		"url":          originalURL,
		"publicUrl":    originalURL,
		"posterUrl":    posterURL,
		"contentType":  claimedContentType,
		"mimeType":     claimedContentType,
		"sizeBytes":    written,
		"kind":         "video",
		"module":       module,
		"ownerType":    ownerType,
		"ownerId":      ownerID,
		"checksum":     checksum,
		"originalName": originalName,
		"width":        meta.Width,
		"height":       meta.Height,
		"duration":     meta.DurationSeconds,
		"status":       string(status),
		"processing":   queued,
	}

	if asset != nil {
		resp["id"] = asset.ID
		resp["assetId"] = asset.ID
		resp["bucket"] = asset.Bucket
		resp["provider"] = asset.Provider
	}

	utils.SuccessResponse(c, http.StatusOK, "Upload successful", resp)
}

func extFromContentType(ct string) string {
	if ct == "image/png" {
		return "png"
	}
	return "jpg"
}

// documentSniffLen only needs to cover the longest magic-byte signature
// checked below (8 bytes, the OLE Compound File header) — kept a little
// larger for headroom.
const documentSniffLen = 16

// validateDocumentMagicBytes checks the file's actual leading bytes against
// the signature its declared Content-Type implies — the same class of check
// browsers/OS file-type detection does, closing the gap where the passthrough
// upload path used to trust the client-supplied header outright. Plain-text
// formats (CSV/TXT) have no reliable magic bytes, so they pass unchecked.
func validateDocumentMagicBytes(ct string, header []byte) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch ct {
	case "application/pdf":
		return bytes.HasPrefix(header, []byte("%PDF-"))
	case "application/msword", "application/vnd.ms-excel":
		// Legacy OLE Compound File Binary Format — shared by .doc and .xls.
		return bytes.HasPrefix(header, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		// .docx/.xlsx are ZIP containers (OOXML).
		return bytes.HasPrefix(header, []byte{0x50, 0x4B, 0x03, 0x04})
	case "text/csv", "text/plain":
		return true
	default:
		return true
	}
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
	case "video":
		// Covers the synchronous part only (buffer to temp, ffprobe
		// validate, extract poster, upload original + poster) — the actual
		// transcode runs in the background job with its own timeout.
		return 5 * time.Minute
	case "audio":
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
