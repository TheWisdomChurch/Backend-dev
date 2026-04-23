package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"wisdomHouse-backend/pkg/utils"
)

func parseUUIDParam(c *gin.Context, paramName, label string) (string, bool) {
	raw := strings.TrimSpace(c.Param(paramName))
	if raw == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" is required")
		return "", false
	}
	if _, err := uuid.Parse(raw); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" must be a valid UUID")
		return "", false
	}
	return raw, true
}

func parseRequiredPathParam(c *gin.Context, paramName, label string) (string, bool) {
	raw := strings.TrimSpace(c.Param(paramName))
	if raw == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" is required")
		return "", false
	}
	return raw, true
}

func parsePaginationQuery(c *gin.Context, defaultLimit, maxLimit int) (int, int, bool) {
	page := 1
	limit := defaultLimit

	if raw := strings.TrimSpace(c.Query("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			utils.ErrorResponse(c, http.StatusBadRequest, "page must be a positive integer")
			return 0, 0, false
		}
		page = v
	}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxLimit {
			utils.ErrorResponse(c, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(maxLimit))
			return 0, 0, false
		}
		limit = v
	}

	return page, limit, true
}

func parseMonthPathParam(c *gin.Context, paramName string) (int, bool) {
	raw := strings.TrimSpace(c.Param(paramName))
	month, err := strconv.Atoi(raw)
	if err != nil || month < 1 || month > 12 {
		utils.ErrorResponse(c, http.StatusBadRequest, "month must be 1-12")
		return 0, false
	}
	return month, true
}

