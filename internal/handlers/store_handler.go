package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wisdomHouse-backend/internal/models"
	"wisdomHouse-backend/internal/service"
	"wisdomHouse-backend/internal/validation"
	"wisdomHouse-backend/pkg/utils"
)

type StoreHandler struct {
	svc service.StoreService
}

func NewStoreHandler(svc service.StoreService) *StoreHandler {
	return &StoreHandler{svc: svc}
}

func (h *StoreHandler) ListProducts(c *gin.Context) {
	items, err := h.svc.ListProducts()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load products")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Products loaded", items)
}

func (h *StoreHandler) CreateOrder(c *gin.Context) {
	var req service.CreateStoreOrderRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	order, err := h.svc.CreateOrder(req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Order created", mapOrderResponse(order))
}

func (h *StoreHandler) GetOrder(c *gin.Context) {
	orderID := c.Param("orderId")
	if orderID == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "orderId is required")
		return
	}
	order, err := h.svc.GetOrder(orderID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Order not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Order loaded", mapOrderResponse(order))
}

func mapOrderResponse(order *models.StoreOrder) gin.H {
	if order == nil {
		return gin.H{}
	}
	return gin.H{
		"orderId":      order.OrderID,
		"orderDate":    order.CreatedAt,
		"status":       order.Status,
		"paymentMethod": order.PaymentMethod,
		"subtotal":     order.Subtotal,
		"deliveryFee":  order.DeliveryFee,
		"total":        order.Total,
		"items":        order.Items,
		"customer": gin.H{
			"firstName": order.CustomerFirstName,
			"lastName":  order.CustomerLastName,
			"email":     order.CustomerEmail,
			"phone":     order.CustomerPhone,
			"address":   order.CustomerAddress,
			"city":      order.CustomerCity,
			"state":     order.CustomerState,
			"zipCode":   order.CustomerZipCode,
		},
		"bankDetails": gin.H{
			"customerAccountName": order.CustomerAccountName,
			"customerBankName":    order.CustomerBankName,
		},
	}
}
