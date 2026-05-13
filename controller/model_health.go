package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

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

	// 与 GetPricing 保持一致的展示规则：
	//   展示集合 = 公开基线（default + vip 旗下分组） ∪ 当前登录用户的可见分组
	// 未登录访客仅看公开基线，已登录用户额外加上自身分组的独有分组。
	displayGroups := map[string]string{}
	for code, desc := range service.GetUserUsableGroups("default") {
		displayGroups[code] = desc
	}
	for code, desc := range service.GetUserUsableGroups("vip") {
		displayGroups[code] = desc
	}

	userId, exists := c.Get("id")
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			for code, desc := range service.GetUserUsableGroups(user.Group) {
				displayGroups[code] = desc
			}
		}
	}

	filtered := model.FilterHealthByGroups(resp, displayGroups)

	c.JSON(200, gin.H{
		"success": true,
		"data":    filtered,
	})
}
