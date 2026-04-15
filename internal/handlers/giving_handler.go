package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"wisdomHouse-backend/pkg/utils"
)

type GivingHandler struct{}

func NewGivingHandler() *GivingHandler {
	return &GivingHandler{}
}

func (h *GivingHandler) ListOptions(c *gin.Context) {
	options := []gin.H{
		{
			"title":       "Tithes, Offerings and Seeds",
			"description": "Give your tithes as an act of worship and obedience to God.",
			"accounts": []gin.H{
				{
					"bank":          "Keystone Bank",
					"accountNumber": "1012525608",
					"accountName":   "The Wisdom Church",
				},
			},
		},
		{
			"title":       "Building Projects, Outreach and Partnership",
			"description": "Support expansion projects and community outreach.",
			"accounts": []gin.H{
				{
					"bank":          "Keystone Bank",
					"accountNumber": "1012879868",
					"accountName":   "The Wisdom Church",
				},
			},
		},
		{
			"title":       "Diaspora Giving",
			"description": "International giving and partnership support.",
			"accounts": []gin.H{
				{
					"bank":          "Providus Bank",
					"accountNumber": "5403892948",
					"accountName":   "The Wisdom Church",
				},
			},
		},
	}

	utils.SuccessResponse(c, http.StatusOK, "Giving options loaded", options)
}
