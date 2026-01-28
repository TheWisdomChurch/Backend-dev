package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/internal/database"
	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/pkg/utils"
)

type AnalyticsHandler struct {
	db *database.Database
}

func NewAnalyticsHandler(db *database.Database) *AnalyticsHandler {
	return &AnalyticsHandler{db: db}
}

func (h *AnalyticsHandler) GetAdminAnalytics(c *gin.Context) {
	var totalEvents int64
	var totalAttendees int64
	var upcomingEvents int64

	if err := h.db.Model(&models.Event{}).Count(&totalEvents).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	// upcoming
	if err := h.db.Model(&models.Event{}).Where("status = ?", models.EventStatusUpcoming).Count(&upcomingEvents).Error; err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to compute analytics")
		return
	}

	// sum attendees
	type sumRow struct{ Sum int64 }
	var row sumRow
	_ = h.db.Model(&models.Event{}).Select("COALESCE(SUM(attendees),0) as sum").Scan(&row).Error
	totalAttendees = row.Sum

	// eventsByCategory
	type catRow struct {
		Category string
		Count    int64
	}
	var cats []catRow
	_ = h.db.Model(&models.Event{}).
		Select("category, COUNT(*) as count").
		Group("category").
		Scan(&cats).Error

	eventsByCategory := map[string]uint64{}
	for _, r := range cats {
		eventsByCategory[r.Category] = uint64(r.Count)
	}

	// monthlyStats (simple placeholder: you can refine to real month grouping later)
	monthlyStats := []gin.H{}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"totalEvents":      totalEvents,
			"upcomingEvents":   upcomingEvents,
			"totalAttendees":   totalAttendees,
			"eventsByCategory": eventsByCategory,
			"monthlyStats":     monthlyStats,
		},
		"message": "Analytics retrieved successfully",
		"status":  "success",
	})
}
