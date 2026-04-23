package handlers

import (
	"net/http"
	"regexp"
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

var orderIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{2,119}$`)

func parseOrderIDPathParam(c *gin.Context, paramName, label string) (string, bool) {
	raw, ok := parseRequiredPathParam(c, paramName, label)
	if !ok {
		return "", false
	}
	if !orderIDPattern.MatchString(raw) {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" is invalid")
		return "", false
	}
	return raw, true
}

func parseUintPathParam(c *gin.Context, paramName, label string) (uint, bool) {
	raw := strings.TrimSpace(c.Param(paramName))
	if raw == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" is required")
		return 0, false
	}
	parsed, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || parsed == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" must be a positive integer")
		return 0, false
	}
	return uint(parsed), true
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

func parseOptionalBoolQuery(c *gin.Context, queryKey, label string) (*bool, bool) {
	raw := strings.TrimSpace(c.Query(queryKey))
	if raw == "" {
		return nil, true
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, label+" must be true or false")
		return nil, false
	}
	return &parsed, true
}

func parseEnumQuery(c *gin.Context, queryKey, label string, allowed []string) (string, bool) {
	raw := strings.ToLower(strings.TrimSpace(c.Query(queryKey)))
	if raw == "" {
		return "", true
	}
	for _, item := range allowed {
		if raw == item {
			return raw, true
		}
	}
	utils.ErrorResponse(c, http.StatusBadRequest, label+" is invalid")
	return "", false
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
