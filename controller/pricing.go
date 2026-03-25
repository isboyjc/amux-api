package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	defaultGroupRatio := map[string]float64{}
	vipGroupRatio := map[string]float64{}
	
	// 获取所有分组的基础倍率（展示用）
	allGroupRatio := ratio_setting.GetGroupRatioCopy()
	for s, f := range allGroupRatio {
		defaultGroupRatio[s] = f
		vipGroupRatio[s] = f
	}
	
	var group string
	currentUserGroupRatio := map[string]float64{}
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			usableGroup = service.GetUserUsableGroups(group)
			// 获取当前用户分组的实际倍率
			for g := range allGroupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					currentUserGroupRatio[g] = ratio
				} else {
					currentUserGroupRatio[g] = allGroupRatio[g]
				}
			}
		}
	} else {
		// 未登录用户，返回所有分组
		usableGroup = map[string]string{}
		for g := range allGroupRatio {
			usableGroup[g] = g
		}
	}

	// 获取 default 用户分组的倍率
	for g := range allGroupRatio {
		ratio, ok := ratio_setting.GetGroupGroupRatio("default", g)
		if ok {
			defaultGroupRatio[g] = ratio
		}
	}
	
	// 获取 VIP 用户分组的倍率
	for g := range allGroupRatio {
		ratio, ok := ratio_setting.GetGroupGroupRatio("vip", g)
		if ok {
			vipGroupRatio[g] = ratio
		}
	}

	// 使用当前用户实际的倍率作为主倍率
	groupRatio := currentUserGroupRatio
	if len(groupRatio) == 0 {
		groupRatio = defaultGroupRatio
	}

	c.JSON(200, gin.H{
		"success":            true,
		"data":               pricing,
		"vendors":            model.GetVendors(),
		"group_ratio":        groupRatio,
		"default_group_ratio": defaultGroupRatio,
		"vip_group_ratio":    vipGroupRatio,
		"user_group":         group,
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"_":                  "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
