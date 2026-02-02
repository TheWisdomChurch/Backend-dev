package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

// UploadHandler lets admins upload arbitrary images to Bunny and get back a CDN URL.
type UploadHandler struct {
	bunny *service.BunnyUploader
}

func NewUploadHandler(bunny *service.BunnyUploader) *UploadHandler {
	return &UploadHandler{bunny: bunny}
}

// UploadImage handles POST /api/v1/admin/uploads (multipart form with "file").
func (h *UploadHandler) UploadImage(c *gin.Context) {
	if h.bunny == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "CDN uploader not configured")
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is required")
		return
	}

	const maxBytes = 10 << 20 // 10MB
	if fh.Size > maxBytes {
		utils.ErrorResponse(c, http.StatusBadRequest, "file too large (max 10MB)")
		return
	}

	src, err := fh.Open()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to open file")
		return
	}
	defer src.Close()

	ct := fh.Header.Get("Content-Type")
	ext := ""
	switch ct {
	case "image/png":
		ext = "png"
	case "image/jpeg":
		ext = "jpg"
	case "image/webp":
		ext = "webp"
	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "only png, jpg, webp allowed")
		return
	}

	folder := c.DefaultPostForm("folder", "uploads")

	objectKey, err := h.bunny.BuildGenericAssetKey(folder, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	cdnURL, err := h.bunny.Upload(c.Request.Context(), objectKey, ct, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to CDN failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"folder": folder,
		"key":    objectKey,
		"url":    cdnURL,
	})
}
