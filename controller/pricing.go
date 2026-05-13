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

	// 获取所有渠道分组的基础倍率（仅用作 fallback 数据源）
	allGroupRatio := ratio_setting.GetGroupRatioCopy()

	// 模型广场展示规则：
	//   展示集合 = 公开基线 ∪ 当前登录用户的可见分组
	//
	// - 公开基线 = default + vip 两个用户分组下可见的渠道分组并集
	//   （未登录访客 / 任意用户都能看到这部分，是平台对外的公开面）
	// - 当前用户的可见分组 = service.GetUserUsableGroups(user.Group)
	//   （已登录用户额外看到自己分组旗下独有的渠道分组，比如 svip 的 test
	//   或 qiye1 的企业专享分组）
	//
	// 对展示集合中但当前用户实际无权访问的分组（例如 default 用户看到 vip
	// 独有分组），前端通过 actuallyUsableGroups（来自 /api/user/self/groups）
	// 自动标记为不可用 + 升级引导样式，本接口不在此处理。
	publicGroups := map[string]string{}
	for code, desc := range service.GetUserUsableGroups("default") {
		publicGroups[code] = desc
	}
	for code, desc := range service.GetUserUsableGroups("vip") {
		publicGroups[code] = desc
	}

	displayGroups := map[string]string{}
	for code, desc := range publicGroups {
		displayGroups[code] = desc
	}

	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for code, desc := range service.GetUserUsableGroups(group) {
				displayGroups[code] = desc
			}
		}
	}

	// 倍率展示限制到 displayGroups 内。default / vip 倍率给前端做"升级对比"，
	// currentUserGroupRatio 是当前用户视角下的真实倍率（用于他自己可访问的分组）。
	defaultGroupRatio := map[string]float64{}
	vipGroupRatio := map[string]float64{}
	for g := range displayGroups {
		if base, ok := allGroupRatio[g]; ok {
			defaultGroupRatio[g] = base
			vipGroupRatio[g] = base
		}
	}
	for g := range displayGroups {
		if ratio, ok := ratio_setting.GetGroupGroupRatio("default", g); ok {
			defaultGroupRatio[g] = ratio
		}
		if ratio, ok := ratio_setting.GetGroupGroupRatio("vip", g); ok {
			vipGroupRatio[g] = ratio
		}
	}

	currentUserGroupRatio := map[string]float64{}
	if group != "" {
		for g := range displayGroups {
			if ratio, ok := ratio_setting.GetGroupGroupRatio(group, g); ok {
				currentUserGroupRatio[g] = ratio
			} else if base, has := allGroupRatio[g]; has {
				currentUserGroupRatio[g] = base
			}
		}
	}

	// 使用当前用户实际的倍率作为主倍率
	groupRatio := currentUserGroupRatio
	if len(groupRatio) == 0 {
		groupRatio = defaultGroupRatio
	}

	// 过滤每个模型的 enable_groups 到 displayGroups 内。pricing 是 GetPricing()
	// 返回的全局缓存切片，绝对不能原地修改 —— 必须重新构造切片副本。
	filteredPricing := make([]model.Pricing, 0, len(pricing))
	for _, p := range pricing {
		filtered := make([]string, 0, len(p.EnableGroup))
		for _, g := range p.EnableGroup {
			if _, ok := displayGroups[g]; ok {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) == 0 {
			// 该模型在展示集合内无可用渠道，从广场列表中剔除
			continue
		}
		p.EnableGroup = filtered
		filteredPricing = append(filteredPricing, p)
	}

	c.JSON(200, gin.H{
		"success":             true,
		"data":                filteredPricing,
		"vendors":             model.GetVendors(),
		"group_ratio":         groupRatio,
		"default_group_ratio": defaultGroupRatio,
		"vip_group_ratio":     vipGroupRatio,
		"user_group":          group,
		"usable_group":        displayGroups,
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
