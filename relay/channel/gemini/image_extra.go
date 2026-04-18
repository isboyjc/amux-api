/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package gemini

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// geminiImageExtra 描述 Gemini 生图家族（Nano Banana / Nano Banana Pro /
// Nano Banana 2）可以被操练场或外部调用方通过 ImageRequest.ExtraBody
// 显式传入的参数子集。没列出的 key 会被静默忽略——不同模型支持不同参数，
// 后台"模型管理"里给每个模型填自己的 param_schema 即可，该 schema 的字段
// 会经由前端 /pg/images/generations 的 extra_body 透传到这里。
//
// 这是一个 adapter-local 的扩展结构，不会暴露到对外 /v1/images/generations
// 的契约上；对公 API 的调用方若未携带 extra_body，此处完全是无副作用的。
type geminiImageExtra struct {
	AspectRatio      string `json:"aspect_ratio,omitempty"`
	ImageSize        string `json:"image_size,omitempty"`
	Seed             *int64 `json:"seed,omitempty"`
	ThinkingLevel    string `json:"thinking_level,omitempty"`
	PersonGeneration string `json:"person_generation,omitempty"`
}

// parseGeminiImageExtra 容错解析 ImageRequest.ExtraBody。任何解析错误
// 都降级为零值——这类字段属于"可选增强"，不该因为格式不合阻断图片生成。
func parseGeminiImageExtra(raw json.RawMessage) geminiImageExtra {
	var extra geminiImageExtra
	if len(raw) == 0 {
		return extra
	}
	_ = common.Unmarshal(raw, &extra)
	return extra
}

// applyGeminiImageExtra 把 geminiImageExtra 合并到 GenerationConfig：
//   - aspect_ratio / image_size / person_generation → generationConfig.imageConfig
//     （Gemini 生图端点只接受 camelCase，这里做名字转换）
//   - seed → generationConfig.seed
//   - thinking_level → generationConfig.thinkingConfig.thinkingLevel（仅 Nano
//     Banana 2 / Gemini 3.1 Flash Image 生效，其它模型上游会忽略）
//
// 如果 cfg.ImageConfig 里已经有内容（例如调用方自己拼了 google 私有参数），
// 会被反序列化后按 key 合并，extra 指定的字段优先。
func applyGeminiImageExtra(cfg *dto.GeminiChatGenerationConfig, extra geminiImageExtra) {
	imageCfg := map[string]interface{}{}
	if len(cfg.ImageConfig) > 0 {
		_ = common.Unmarshal(cfg.ImageConfig, &imageCfg)
	}
	if extra.AspectRatio != "" {
		imageCfg["aspectRatio"] = extra.AspectRatio
	}
	if extra.ImageSize != "" {
		imageCfg["imageSize"] = extra.ImageSize
	}
	if extra.PersonGeneration != "" {
		imageCfg["personGeneration"] = extra.PersonGeneration
	}
	if len(imageCfg) > 0 {
		if bytes, err := common.Marshal(imageCfg); err == nil {
			cfg.ImageConfig = bytes
		}
	}

	if extra.Seed != nil {
		cfg.Seed = extra.Seed
	}

	if extra.ThinkingLevel != "" {
		if cfg.ThinkingConfig == nil {
			cfg.ThinkingConfig = &dto.GeminiThinkingConfig{}
		}
		cfg.ThinkingConfig.ThinkingLevel = extra.ThinkingLevel
	}
}
