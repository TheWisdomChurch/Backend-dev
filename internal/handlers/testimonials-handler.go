package handlers

import (
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/yourusername/church-api/internal/models"
    "github.com/yourusername/church-api/internal/service"
    "github.com/yourusername/church-api/pkg/utils"
)

type TestimonialHandler struct {
    service service.TestimonialService
}

func NewTestimonialHandler(service service.TestimonialService) *TestimonialHandler {
    return &TestimonialHandler{service: service}
}

// CreateTestimonial godoc
// @Summary Create a new testimonial
// @Tags testimonials
// @Accept json
// @Produce json
// @Param testimonial body models.CreateTestimonialRequest true "Testimonial data"
// @Success 201 {object} utils.Response
// @Router /testimonials [post]
func (h *TestimonialHandler) CreateTestimonial(c *gin.Context) {
    var req models.CreateTestimonialRequest
    
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
        return
    }
    
    testimonial, err := h.service.CreateTestimonial(&req)
    if err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to create testimonial")
        return
    }
    
    utils.SuccessResponse(c, http.StatusCreated, "Testimonial created successfully", testimonial)
}

// GetAllTestimonials godoc
// @Summary Get all testimonials
// @Tags testimonials
// @Produce json
// @Param approved query bool false "Filter by approved status"
// @Success 200 {object} utils.Response
// @Router /testimonials [get]
func (h *TestimonialHandler) GetAllTestimonials(c *gin.Context) {
    approved := c.DefaultQuery("approved", "true") == "true"
    
    testimonials, err := h.service.GetAllTestimonials(approved)
    if err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch testimonials")
        return
    }
    
    utils.SuccessResponse(c, http.StatusOK, "Testimonials fetched successfully", testimonials)
}

// GetPaginatedTestimonials godoc
// @Summary Get paginated testimonials
// @Tags testimonials
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Items per page" default(10)
// @Param approved query bool false "Filter by approved status"
// @Success 200 {object} utils.PaginatedResponse
// @Router /testimonials/paginated [get]
func (h *TestimonialHandler) GetPaginatedTestimonials(c *gin.Context) {
    page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
    limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
    approved := c.DefaultQuery("approved", "true") == "true"
    
    testimonials, total, err := h.service.GetPaginatedTestimonials(page, limit, approved)
    if err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to fetch testimonials")
        return
    }
    
    utils.PaginatedSuccessResponse(c, http.StatusOK, testimonials, page, limit, total)
}

// GetTestimonialByID godoc
// @Summary Get testimonial by ID
// @Tags testimonials
// @Produce json
// @Param id path string true "Testimonial ID"
// @Success 200 {object} utils.Response
// @Router /testimonials/{id} [get]
func (h *TestimonialHandler) GetTestimonialByID(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
        return
    }
    
    testimonial, err := h.service.GetTestimonialByID(id)
    if err != nil {
        utils.ErrorResponse(c, http.StatusNotFound, "Testimonial not found")
        return
    }
    
    utils.SuccessResponse(c, http.StatusOK, "Testimonial fetched successfully", testimonial)
}

// UpdateTestimonial godoc
// @Summary Update testimonial
// @Tags testimonials
// @Accept json
// @Produce json
// @Param id path string true "Testimonial ID"
// @Param testimonial body models.UpdateTestimonialRequest true "Updated testimonial data"
// @Success 200 {object} utils.Response
// @Router /testimonials/{id} [put]
func (h *TestimonialHandler) UpdateTestimonial(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
        return
    }
    
    var req models.UpdateTestimonialRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
        return
    }
    
    testimonial, err := h.service.UpdateTestimonial(id, &req)
    if err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to update testimonial")
        return
    }
    
    utils.SuccessResponse(c, http.StatusOK, "Testimonial updated successfully", testimonial)
}

// DeleteTestimonial godoc
// @Summary Delete testimonial
// @Tags testimonials
// @Produce json
// @Param id path string true "Testimonial ID"
// @Success 200 {object} utils.Response
// @Router /testimonials/{id} [delete]
func (h *TestimonialHandler) DeleteTestimonial(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
        return
    }
    
    if err := h.service.DeleteTestimonial(id); err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to delete testimonial")
        return
    }
    
    utils.SuccessResponse(c, http.StatusOK, "Testimonial deleted successfully", nil)
}

// ApproveTestimonial godoc
// @Summary Approve testimonial
// @Tags testimonials
// @Produce json
// @Param id path string true "Testimonial ID"
// @Success 200 {object} utils.Response
// @Router /testimonials/{id}/approve [patch]
func (h *TestimonialHandler) ApproveTestimonial(c *gin.Context) {
    id, err := uuid.Parse(c.Param("id"))
    if err != nil {
        utils.ErrorResponse(c, http.StatusBadRequest, "Invalid testimonial ID")
        return
    }
    
    testimonial, err := h.service.ApproveTestimonial(id)
    if err != nil {
        utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to approve testimonial")
        return
    }
    
    utils.SuccessResponse(c, http.StatusOK, "Testimonial approved successfully", testimonial)
}