package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/repository"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/pkg/utils"
)

type ApprovalRequestHandler struct {
	svc  service.ApprovalService
	repo *repository.ApprovalRequestRepository
}

func NewApprovalRequestHandler(svc service.ApprovalService, repo *repository.ApprovalRequestRepository) *ApprovalRequestHandler {
	return &ApprovalRequestHandler{svc: svc, repo: repo}
}

type approvalRequestActionLink struct {
	Label  string `json:"label"`
	Method string `json:"method"`
	URL    string `json:"url"`
}

type approvalRequestResponse struct {
	ID               string                       `json:"id"`
	TicketCode       string                       `json:"ticketCode"`
	Type             models.ApprovalRequestType   `json:"type"`
	Status           models.ApprovalRequestStatus `json:"status"`
	EntityID         *string                      `json:"entityId,omitempty"`
	EntityLabel      *string                      `json:"entityLabel,omitempty"`
	RequestedByID    *string                      `json:"requestedById,omitempty"`
	RequestedByName  *string                      `json:"requestedByName,omitempty"`
	RequestedByEmail *string                      `json:"requestedByEmail,omitempty"`
	ApprovedByID     *string                      `json:"approvedById,omitempty"`
	ApprovedByName   *string                      `json:"approvedByName,omitempty"`
	ApprovedByEmail  *string                      `json:"approvedByEmail,omitempty"`
	ApprovedAt       *time.Time                   `json:"approvedAt,omitempty"`
	CreatedAt        time.Time                    `json:"createdAt"`
	UpdatedAt        time.Time                    `json:"updatedAt"`
	EntityType       string                       `json:"entityType"`
	CanApprove       bool                         `json:"canApprove"`
	CanReject        bool                         `json:"canReject"`
	ApproveAction    *approvalRequestActionLink   `json:"approveAction,omitempty"`
	RejectAction     *approvalRequestActionLink   `json:"rejectAction,omitempty"`
	DeleteAction     *approvalRequestActionLink   `json:"deleteAction,omitempty"`
}

func presentApprovalRequest(req models.ApprovalRequest) approvalRequestResponse {
	resp := approvalRequestResponse{
		ID:               req.ID,
		TicketCode:       req.TicketCode,
		Type:             req.Type,
		Status:           req.Status,
		EntityID:         req.EntityID,
		EntityLabel:      req.EntityLabel,
		RequestedByID:    req.RequestedByID,
		RequestedByName:  req.RequestedByName,
		RequestedByEmail: req.RequestedByEmail,
		ApprovedByID:     req.ApprovedByID,
		ApprovedByName:   req.ApprovedByName,
		ApprovedByEmail:  req.ApprovedByEmail,
		ApprovedAt:       req.ApprovedAt,
		CreatedAt:        req.CreatedAt,
		UpdatedAt:        req.UpdatedAt,
		EntityType:       string(req.Type),
		CanApprove:       req.Status == models.ApprovalStatusPending,
		CanReject:        req.Status == models.ApprovalStatusPending,
	}

	if req.EntityID == nil || strings.TrimSpace(*req.EntityID) == "" || req.Status != models.ApprovalStatusPending {
		return resp
	}

	entityID := strings.TrimSpace(*req.EntityID)
	switch req.Type {
	case models.ApprovalTypeAdminUser:
		resp.ApproveAction = &approvalRequestActionLink{Label: "Approve admin account", Method: http.MethodPost, URL: "/api/v1/admin/users/" + entityID + "/approve"}
		resp.RejectAction = &approvalRequestActionLink{Label: "Reject admin account", Method: http.MethodPost, URL: "/api/v1/admin/users/" + entityID + "/reject"}
	case models.ApprovalTypeTestimonial:
		resp.ApproveAction = &approvalRequestActionLink{Label: "Approve testimonial", Method: http.MethodPatch, URL: "/api/v1/admin/testimonials/" + entityID + "/approve"}
		resp.DeleteAction = &approvalRequestActionLink{Label: "Reject/delete testimonial", Method: http.MethodDelete, URL: "/api/v1/admin/testimonials/" + entityID}
	case models.ApprovalTypeEvent:
		resp.ApproveAction = &approvalRequestActionLink{Label: "Approve event", Method: http.MethodPatch, URL: "/api/v1/admin/events/" + entityID + "/approve"}
		resp.DeleteAction = &approvalRequestActionLink{Label: "Reject/delete event", Method: http.MethodDelete, URL: "/api/v1/admin/events/" + entityID}
	case models.ApprovalTypeLeadershipDelete:
		resp.ApproveAction = &approvalRequestActionLink{Label: "Approve leadership delete", Method: http.MethodPost, URL: "/api/v1/admin/leadership/" + entityID + "/delete/approve"}
		resp.RejectAction = &approvalRequestActionLink{Label: "Decline leadership request", Method: http.MethodPost, URL: "/api/v1/admin/leadership/" + entityID + "/decline"}
	case models.ApprovalTypeWorkforceDelete:
		resp.ApproveAction = &approvalRequestActionLink{Label: "Approve workforce delete", Method: http.MethodPost, URL: "/api/v1/admin/workforce/" + entityID + "/delete/approve"}
	}

	return resp
}

func (h *ApprovalRequestHandler) List(c *gin.Context) {
	typeFilter := strings.TrimSpace(c.Query("type"))
	statusFilter := strings.TrimSpace(c.Query("status"))
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}

	var types []models.ApprovalRequestType
	if typeFilter != "" {
		for _, t := range strings.Split(typeFilter, ",") {
			tt := strings.TrimSpace(t)
			if tt != "" {
				types = append(types, models.ApprovalRequestType(tt))
			}
		}
	}

	var statuses []models.ApprovalRequestStatus
	if statusFilter != "" {
		for _, s := range strings.Split(statusFilter, ",") {
			ss := strings.TrimSpace(s)
			if ss != "" {
				statuses = append(statuses, models.ApprovalRequestStatus(ss))
			}
		}
	}
	if len(statuses) == 0 {
		statuses = []models.ApprovalRequestStatus{models.ApprovalStatusPending}
	}

	start, end, err := parseTimeRange(c)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	items, err := h.svc.ListRequests(types, statuses, start, end, limit)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load requests")
		return
	}

	resp := make([]approvalRequestResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, presentApprovalRequest(item))
	}

	utils.SuccessResponse(c, http.StatusOK, "Requests loaded", gin.H{
		"data":  resp,
		"total": len(resp),
	})
}

func (h *ApprovalRequestHandler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "request id is required")
		return
	}

	item, err := h.svc.GetRequest(id)
	if err != nil || item == nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Approval request not found")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Request loaded", presentApprovalRequest(*item))
}

func (h *ApprovalRequestHandler) Timeline(c *gin.Context) {
	days := 7
	if raw := strings.TrimSpace(c.Query("days")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 && v <= 90 {
			days = v
		}
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -days+1)

	created, err := h.repo.CountCreatedByDay(&start, &end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load timeline")
		return
	}
	approved, err := h.repo.CountApprovedByDay(&start, &end)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load timeline")
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Timeline loaded", gin.H{
		"start":    start,
		"end":      end,
		"created":  created,
		"approved": approved,
	})
}
