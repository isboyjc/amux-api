package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)

	// 预先收集各分组的可用模型，供分组列表展示及 auto 聚合使用
	groupModelsCache := make(map[string][]string)
	autoModelSet := make(map[string]struct{})
	for groupName := range userUsableGroups {
		if groupName == "auto" {
			continue
		}
		models := model.GetGroupEnabledModels(groupName)
		groupModelsCache[groupName] = models
		for _, m := range models {
			autoModelSet[m] = struct{}{}
		}
	}

	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// UserUsableGroups contains the groups that the user can use
		if desc, ok := userUsableGroups[groupName]; ok {
			// 完全过滤掉用户自己的等级分组，避免在创建令牌时选择用户等级作为令牌分组
			// 用户等级分组应该只用于标识用户权限等级，不应该作为令牌的渠道分组
			if groupName == userGroup {
				continue
			}
			usableGroups[groupName] = map[string]interface{}{
				"ratio":       service.GetUserGroupRatio(userGroup, groupName),
				"desc":        desc,
				"model_count": len(groupModelsCache[groupName]),
			}
		}
	}
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio":       "自动",
			"desc":        setting.GetUsableGroupDescription("auto"),
			"model_count": len(autoModelSet),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
