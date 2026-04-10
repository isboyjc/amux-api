package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetModelHealth(c *gin.Context) {
	timeRange := c.DefaultQuery("range", "24h")
	if timeRange != "24h" && timeRange != "7d" {
		timeRange = "24h"
	}

	resp, err := model.GetModelHealthData(timeRange)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Determine user's usable groups (same logic as GetPricing)
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			usableGroup = service.GetUserUsableGroups(user.Group)
		}
	} else {
		// Unauthenticated users: return all groups
		allGroupRatio := ratio_setting.GetGroupRatioCopy()
		for g := range allGroupRatio {
			usableGroup[g] = g
		}
	}

	filtered := model.FilterHealthByGroups(resp, usableGroup)

	c.JSON(200, gin.H{
		"success": true,
		"data":    filtered,
	})
}
