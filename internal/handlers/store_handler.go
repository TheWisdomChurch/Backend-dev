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

func (h *StoreHandler) ListProductsAdmin(c *gin.Context) {
	includeInactivePtr, ok := parseOptionalBoolQuery(c, "includeInactive", "includeInactive")
	if !ok {
		return
	}
	includeInactive := includeInactivePtr != nil && *includeInactivePtr
	items, err := h.svc.ListProductsAdmin(includeInactive)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load products")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Products loaded", items)
}

func (h *StoreHandler) CreateProduct(c *gin.Context) {
	var req service.UpsertStoreProductRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.CreateProduct(req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Product created", item)
}

func (h *StoreHandler) UpdateProduct(c *gin.Context) {
	id, ok := parseUintPathParam(c, "id", "product id")
	if !ok {
		return
	}
	var req service.UpsertStoreProductRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.UpdateProduct(id, req)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Product updated", item)
}

func (h *StoreHandler) UpdateProductStock(c *gin.Context) {
	id, ok := parseUintPathParam(c, "id", "product id")
	if !ok {
		return
	}
	var req struct {
		Stock int `json:"stock"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.UpdateProductStock(id, req.Stock)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Stock updated", item)
}

func (h *StoreHandler) UpdateProductActive(c *gin.Context) {
	id, ok := parseUintPathParam(c, "id", "product id")
	if !ok {
		return
	}
	var req struct {
		IsActive bool `json:"isActive"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.UpdateProductActive(id, req.IsActive)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Product status updated", item)
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
	orderID, ok := parseOrderIDPathParam(c, "orderId", "order id")
	if !ok {
		return
	}
	order, err := h.svc.GetOrder(orderID, c.GetHeader("X-Order-Access-Token"))
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "Order not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Order loaded", mapOrderResponse(order))
}

func (h *StoreHandler) SubmitPaymentProof(c *gin.Context) {
	orderID, ok := parseOrderIDPathParam(c, "orderId", "order id")
	if !ok {
		return
	}
	var req struct {
		PaymentSlipURL string `json:"paymentSlipUrl" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	order, err := h.svc.SubmitPaymentProof(orderID, c.GetHeader("X-Order-Access-Token"), req.PaymentSlipURL)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Payment proof submitted", mapOrderResponse(order))
}

func (h *StoreHandler) ListOrdersAdmin(c *gin.Context) {
	page, limit, ok := parsePaginationQuery(c, 20, 100)
	if !ok {
		return
	}
	status, ok := parseEnumQuery(c, "status", "status", []string{"pending", "processing", "shipped", "delivered", "cancelled"})
	if !ok {
		return
	}
	items, total, err := h.svc.ListOrders(page, limit, status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to load orders")
		return
	}
	mapped := make([]gin.H, 0, len(items))
	for i := range items {
		mapped = append(mapped, mapOrderResponse(&items[i]))
	}
	utils.SuccessResponse(c, http.StatusOK, "Orders loaded", gin.H{
		"data":       mapped,
		"total":      total,
		"page":       page,
		"limit":      limit,
		"totalPages": (total + int64(limit) - 1) / int64(limit),
	})
}

func (h *StoreHandler) UpdateOrderStatus(c *gin.Context) {
	orderID, ok := parseOrderIDPathParam(c, "orderId", "order id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	item, err := h.svc.UpdateOrderStatus(orderID, req.Status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Order status updated", mapOrderResponse(item))
}

func (h *StoreHandler) UpdateOrderPaymentStatus(c *gin.Context) {
	orderID, ok := parseOrderIDPathParam(c, "orderId", "order id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
	order, err := h.svc.UpdateOrderPaymentStatus(orderID, req.Status)
	if err != nil {
		utils.ErrorResponse(c, http.StatusConflict, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Payment status updated", mapOrderResponse(order))
}

func mapOrderResponse(order *models.StoreOrder) gin.H {
	if order == nil {
		return gin.H{}
	}
	return gin.H{
		"orderId":        order.OrderID,
		"orderDate":      order.CreatedAt,
		"status":         order.Status,
		"paymentMethod":  order.PaymentMethod,
		"paymentStatus":  order.PaymentStatus,
		"paymentSlipUrl": order.PaymentSlipURL,
		"subtotal":       order.Subtotal,
		"deliveryFee":    order.DeliveryFee,
		"total":          order.Total,
		"items":          order.Items,
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
