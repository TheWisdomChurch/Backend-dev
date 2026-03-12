package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"wisdomHouse-backend/internal/exportpdf"
	"wisdomHouse-backend/pkg/utils"
)

func (h *FormHandler) ExportAdminSubmissionsPDF(c *gin.Context) {
	formID := strings.TrimSpace(c.Param("id"))
	if formID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "missing form id")
		return
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	filename, pdfBytes, err := h.svc.BuildAdminReportPDF(formID, start, end)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			utils.ErrorResponse(c, http.StatusNotFound, "form not found")
			return
		}
		utils.ErrorResponse(c, http.StatusInternalServerError, "failed to generate pdf")
		return
	}

	output := pdfBytes
	if shouldEncryptPDF(c) {
		password := strings.TrimSpace(c.Query("password"))
		if password == "" {
			password = getAuthEmailFromContext(c)
		}
		if password == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "unable to determine export password")
			return
		}

		encrypted, encErr := exportpdf.EncryptPDF(pdfBytes, password)
		if encErr != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "failed to encrypt pdf")
			return
		}
		output = encrypted
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "application/pdf", output)
}

func shouldEncryptPDF(c *gin.Context) bool {
	protect := strings.TrimSpace(c.Query("protect"))
	if protect == "1" || strings.EqualFold(protect, "true") || strings.EqualFold(protect, "yes") {
		return true
	}
	return strings.TrimSpace(c.Query("password")) != ""
}

func getAuthEmailFromContext(c *gin.Context) string {
	for _, key := range []string{"email", "userEmail", "authEmail"} {
		if v, ok := c.Get(key); ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
