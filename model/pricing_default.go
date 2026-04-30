package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// 简化的供应商映射规则
var defaultVendorRules = map[string]string{
	"gpt":      "OpenAI",
	"dall-e":   "OpenAI",
	"whisper":  "OpenAI",
	"o1":       "OpenAI",
	"o3":       "OpenAI",
	"claude":   "Anthropic",
	"gemini":   "Google",
	"moonshot": "Moonshot",
	"kimi":     "Moonshot",
	"chatglm":  "智谱",
	"glm-":     "智谱",
	"qwen":     "阿里巴巴",
	"deepseek": "DeepSeek",
	"abab":     "MiniMax",
	"minimax":  "MiniMax",
	"ernie":    "百度",
	"spark":    "讯飞",
	"hunyuan":  "腾讯",
	"command":  "Cohere",
	"@cf/":     "Cloudflare",
	"360":      "360",
	"yi":       "零一万物",
	"jina":     "Jina",
	"mistral":  "Mistral",
	"grok":     "xAI",
	"llama":    "Meta",
	"doubao":   "ByteDance",
	"kling":    "快手",
	"jimeng":   "即梦",
	"vidu":     "Vidu",
	"mimo":     "Xiaomi",
	"xiaomi":   "Xiaomi",
}

// 供应商默认图标映射
var defaultVendorIcons = map[string]string{
	"OpenAI":     "OpenAI",
	"Anthropic":  "Claude.Color",
	"Google":     "Gemini.Color",
	"Moonshot":   "Moonshot",
	"智谱":         "Zhipu.Color",
	"阿里巴巴":       "Qwen.Color",
	"DeepSeek":   "DeepSeek.Color",
	"MiniMax":    "Minimax.Color",
	"百度":         "Wenxin.Color",
	"讯飞":         "Spark.Color",
	"腾讯":         "Hunyuan.Color",
	"Cohere":     "Cohere.Color",
	"Cloudflare": "Cloudflare.Color",
	"360":        "Ai360.Color",
	"零一万物":       "Yi.Color",
	"Jina":       "Jina",
	"Mistral":    "Mistral.Color",
	"xAI":        "XAI",
	"Meta":       "Ollama",
	"ByteDance":  "Doubao.Color",
	"快手":         "Kling.Color",
	"即梦":         "Jimeng.Color",
	"Vidu":       "Vidu",
	"微软":         "AzureAI",
	"Microsoft":  "AzureAI",
	"Azure":      "AzureAI",
	"Xiaomi":     "Xiaomi",
}

// initDefaultVendorMapping 简化的默认供应商映射
func initDefaultVendorMapping(metaMap map[string]*Model, vendorMap map[int]*Vendor, enableAbilities []AbilityWithChannel) {
	for _, ability := range enableAbilities {
		modelName := ability.Model
		if _, exists := metaMap[modelName]; exists {
			continue
		}

		// 匹配供应商
		vendorID := 0
		modelLower := strings.ToLower(modelName)
		for pattern, vendorName := range defaultVendorRules {
			if strings.Contains(modelLower, pattern) {
				vendorID = getOrCreateVendor(vendorName, vendorMap)
				break
			}
		}

		// 创建模型元数据
		metaMap[modelName] = &Model{
			ModelName: modelName,
			VendorID:  vendorID,
			Status:    1,
			NameRule:  NameRuleExact,
		}
	}
}

// 查找或创建供应商
func getOrCreateVendor(vendorName string, vendorMap map[int]*Vendor) int {
	// 查找现有供应商
	for id, vendor := range vendorMap {
		if vendor.Name == vendorName {
			return id
		}
	}

	// 创建新供应商
	newVendor := &Vendor{
		Name:   vendorName,
		Status: 1,
		Icon:   getDefaultVendorIcon(vendorName),
	}

	if err := newVendor.Insert(); err != nil {
		return 0
	}

	vendorMap[newVendor.Id] = newVendor
	return newVendor.Id
}

// 获取供应商默认图标
func getDefaultVendorIcon(vendorName string) string {
	if icon, exists := defaultVendorIcons[vendorName]; exists {
		return icon
	}
	return ""
}

// EnsureCommonVendors 确保常见的供应商在数据库中存在
func EnsureCommonVendors() {
	// 从 defaultVendorIcons 获取所有常见供应商列表
	commonVendors := []string{
		"OpenAI", "Anthropic", "Google", "Moonshot", "智谱", "阿里巴巴",
		"DeepSeek", "MiniMax", "百度", "讯飞", "腾讯", "Cohere",
		"Cloudflare", "360", "零一万物", "Jina", "Mistral", "xAI",
		"Meta", "ByteDance", "快手", "即梦", "Vidu", "微软", "Xiaomi",
	}

	for _, vendorName := range commonVendors {
		// 检查是否已存在
		var existing Vendor
		if err := DB.Where("name = ?", vendorName).First(&existing).Error; err == nil {
			continue
		}

		// 不存在则创建
		vendor := &Vendor{
			Name:   vendorName,
			Status: 1,
			Icon:   getDefaultVendorIcon(vendorName),
		}
		_ = vendor.Insert()
	}
}

// MigrateModelVendorIDs 一次性迁移：修复历史模型的 vendor_id
// 通过 Option 表记录是否已执行，避免重复执行
func MigrateModelVendorIDs() {
	// 检查是否已执行过此迁移
	migrationKey := "model_vendor_id_migration_v1"
	var opt Option
	if err := DB.Where(commonKeyCol+" = ?", migrationKey).First(&opt).Error; err == nil {
		if opt.Value == "done" {
			return
		}
	}

	// 构建供应商名称到ID的映射
	var vendors []Vendor
	if err := DB.Find(&vendors).Error; err != nil {
		return
	}
	vendorNameToID := make(map[string]int)
	for _, v := range vendors {
		vendorNameToID[v.Name] = v.Id
	}

	// 获取所有 vendor_id 为 0 的模型
	var modelsWithoutVendor []Model
	if err := DB.Where("vendor_id = ?", 0).Find(&modelsWithoutVendor).Error; err != nil {
		return
	}

	// 更新模型的 vendor_id
	updateCount := 0
	for _, model := range modelsWithoutVendor {
		modelLower := strings.ToLower(model.ModelName)
		// 根据规则匹配供应商
		for pattern, vendorName := range defaultVendorRules {
			if strings.Contains(modelLower, pattern) {
				if vendorID, ok := vendorNameToID[vendorName]; ok && vendorID > 0 {
					// 更新模型的 vendor_id
					if err := DB.Model(&Model{}).Where("id = ?", model.Id).Update("vendor_id", vendorID).Error; err == nil {
						updateCount++
					}
				}
				break
			}
		}
	}

	// 标记迁移已完成
	_ = UpdateOption(migrationKey, "done")

	if updateCount > 0 {
		common.SysLog(fmt.Sprintf("migrated vendor_id for %d models", updateCount))
	}
}

// RenameBytedanceVendor 历史默认表里写的是中文「字节跳动」，统一改为英文 "ByteDance"
// 与其它默认厂商命名风格对齐。直接重命名旧行（保留 ID），避免 EnsureCommonVendors
// 重新建一条新行造成重复 + 模型脱链。无 Option 标记，幂等：旧行不存在直接跳过。
func RenameBytedanceVendor() {
	const oldName = "字节跳动"
	const newName = "ByteDance"

	var oldVendor Vendor
	if err := DB.Where("name = ?", oldName).First(&oldVendor).Error; err != nil {
		return // 旧行不存在（新装或已迁移），无事可做
	}

	var newVendor Vendor
	if err := DB.Where("name = ?", newName).First(&newVendor).Error; err == nil {
		common.SysLog("both 字节跳动 and ByteDance vendor rows exist; skipping rename, please reconcile manually")
		return
	}

	oldVendor.Name = newName
	if oldVendor.Icon == "" {
		oldVendor.Icon = "Doubao.Color"
	}
	if err := oldVendor.Update(); err != nil {
		common.SysLog("rename vendor 字节跳动 -> ByteDance failed: " + err.Error())
		return
	}
	common.SysLog("renamed vendor 字节跳动 -> ByteDance")
}
