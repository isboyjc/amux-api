package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

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
	}
	// 未登录 / 已登录但用户缓存失败 的 fallback：展示所有分组，
	// 避免下方 filterPricingByUsableGroups 因 usableGroup 为空返回空列表
	if len(usableGroup) == 0 {
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

	// 按上游行为过滤 pricing：仅保留用户可用分组启用的模型
	pricing = filterPricingByUsableGroups(pricing, usableGroup)

	c.JSON(200, gin.H{
		"success":             true,
		"data":                pricing,
		"vendors":             model.GetVendors(),
		"group_ratio":         groupRatio,
		"default_group_ratio": defaultGroupRatio,
		"vip_group_ratio":     vipGroupRatio,
		"user_group":          group,
		"usable_group":        usableGroup,
		"supported_endpoint":  model.GetSupportedEndpointMap(),
		"auto_groups":         service.GetUserAutoGroup(group),
		"pricing_version":     "a42d372ccf0b5dd13ecf71203521f9d2",
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
