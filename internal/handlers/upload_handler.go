package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type UploadHandler struct {
	spaces *service.SpacesUploader
}

func NewUploadHandler(spaces *service.SpacesUploader) *UploadHandler {
	return &UploadHandler{spaces: spaces}
}

func (h *UploadHandler) UploadImage(c *gin.Context) {
	if h.spaces == nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Storage uploader not configured")
		return
	}

	fh, err := c.FormFile("file")
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "file is required")
		return
	}

	const maxBytes = 10 << 20
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
	ext, ok := allowedImageExt(ct)
	if !ok {
		utils.ErrorResponse(c, http.StatusBadRequest, "only png, jpg, webp allowed")
		return
	}

	folder := c.DefaultPostForm("folder", "uploads")

	objectKey, err := h.spaces.BuildGenericAssetKey(folder, ext)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to build storage key")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	cdnURL, err := h.spaces.Upload(ctx, objectKey, ct, src)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadGateway, "upload to storage failed")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Upload successful", gin.H{
		"folder": folder,
		"key":    objectKey,
		"url":    cdnURL,
	})
}
