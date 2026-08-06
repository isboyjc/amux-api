package constant

// DefaultParamSchemas 按 modality 提供一套内置参数 JSON Schema 模板。当管理员
// 没有为某个模型填写 param_schema 时，前端会回退到这些模板渲染右栏参数控件。
//
// Schema 基元到前端控件的映射：
//   - enum                   → Select
//   - integer/number + min/max → Slider + InputNumber
//   - boolean                → Switch
//   - string (无 enum)       → Input
//
// 这些模板保持"最小通用集"，避免和具体厂商/模型的参数耦合。管理员可以在后台
// 根据实际模型覆盖。
var DefaultParamSchemas = map[string]string{
	ModalityText: `{
  "type": "object",
  "properties": {
    "temperature": {
      "type": "number",
      "title": "随机性",
      "description": "0 最确定，2 最发散",
      "minimum": 0,
      "maximum": 2,
      "default": 1
    },
    "top_p": {
      "type": "number",
      "title": "Top P",
      "minimum": 0,
      "maximum": 1,
      "default": 1
    },
    "max_tokens": {
      "type": "integer",
      "title": "最大输出 Token",
      "minimum": 1,
      "maximum": 32768,
      "default": 4096
    },
    "frequency_penalty": {
      "type": "number",
      "title": "频率惩罚",
      "minimum": -2,
      "maximum": 2,
      "default": 0
    },
    "presence_penalty": {
      "type": "number",
      "title": "存在惩罚",
      "minimum": -2,
      "maximum": 2,
      "default": 0
    },
    "seed": {
      "type": "integer",
      "title": "随机种子"
    }
  }
}`,
	ModalityMultimodal: `{
  "type": "object",
  "properties": {
    "temperature": {
      "type": "number",
      "title": "随机性",
      "minimum": 0,
      "maximum": 2,
      "default": 1
    },
    "top_p": {
      "type": "number",
      "title": "Top P",
      "minimum": 0,
      "maximum": 1,
      "default": 1
    },
    "max_tokens": {
      "type": "integer",
      "title": "最大输出 Token",
      "minimum": 1,
      "maximum": 32768,
      "default": 4096
    }
  }
}`,
	ModalityImage: `{
  "type": "object",
  "properties": {
    "size": {
      "type": "string",
      "title": "分辨率",
      "enum": ["1024x1024", "1024x1792", "1792x1024", "auto"],
      "default": "1024x1024"
    },
    "quality": {
      "type": "string",
      "title": "质量",
      "enum": ["standard", "hd", "auto"],
      "default": "auto"
    },
    "n": {
      "type": "integer",
      "title": "生成数量",
      "minimum": 1,
      "maximum": 10,
      "default": 1
    }
  }
}`,
	ModalityEmbedding: `{
  "type": "object",
  "properties": {
    "dimensions": {
      "type": "integer",
      "title": "向量维度",
      "description": "部分模型支持降维，不填则返回默认维度",
      "minimum": 1,
      "maximum": 3072
    },
    "encoding_format": {
      "type": "string",
      "title": "编码格式",
      "enum": ["float", "base64"],
      "default": "float"
    }
  }
}`,
	ModalityVideo:  `{"type":"object","properties":{}}`,
	ModalityAudio:  `{"type":"object","properties":{}}`,
	ModalityRerank: `{"type":"object","properties":{"top_n":{"type":"integer","title":"返回条数","minimum":1,"maximum":100,"default":10}}}`,
}

// DefaultModelParamSchemas 为无需管理员额外配置即可使用的模型提供精确参数
// Schema。数据库中的 exact/rule param_schema 仍拥有更高优先级。
var DefaultModelParamSchemas = map[string]string{
	"happyhorse-1.1-t2v":    happyHorseVideoParamSchema,
	"happyhorse-1.0-t2v":    happyHorseVideoParamSchema,
	"wan2.7-i2v-2026-04-25": wan27VideoParamSchema,
}

const happyHorseVideoParamSchema = `{
  "type": "object",
  "properties": {
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["480P", "720P", "1080P"],
      "default": "1080P"
    },
    "ratio": {
      "type": "string",
      "title": "宽高比",
      "enum": ["16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9"],
      "default": "16:9"
    },
    "duration": {
      "type": "integer",
      "title": "时长（秒）",
      "minimum": 3,
      "maximum": 15,
      "default": 5
    },
    "watermark": {
      "type": "boolean",
      "title": "添加 Happy Horse 水印",
      "default": true
    },
    "seed": {
      "type": "integer",
      "title": "随机种子",
      "minimum": 0,
      "maximum": 2147483647
    }
  }
}`

const wan27VideoParamSchema = `{
  "type": "object",
  "properties": {
    "reference_images": {
      "type": "array",
      "title": "首帧 / 尾帧",
      "description": "一张图作为首帧，两张图依次作为首帧和尾帧；配合首段视频时一张图作为尾帧",
      "maxItems": 2,
      "items": {"type": "string", "format": "image"},
      "x-content-role": "reference_image",
      "x-max-mb-per-item": 20
    },
    "first_clip": {
      "type": "array",
      "title": "首段视频",
      "maxItems": 1,
      "items": {"type": "string", "format": "video"},
      "x-content-role": "reference_video",
      "x-max-mb-per-item": 100,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 10,
      "x-max-total-duration-seconds": 10
    },
    "driving_audio": {
      "type": "array",
      "title": "驱动音频",
      "maxItems": 1,
      "items": {"type": "string", "format": "audio"},
      "x-content-role": "reference_audio",
      "x-max-mb-per-item": 15,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 30,
      "x-max-total-duration-seconds": 30
    },
    "negative_prompt": {
      "type": "string",
      "title": "反向提示词"
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["720P", "1080P"],
      "default": "1080P"
    },
    "duration": {
      "type": "integer",
      "title": "总时长（秒）",
      "minimum": 2,
      "maximum": 15,
      "default": 5
    },
    "prompt_extend": {
      "type": "boolean",
      "title": "智能改写 Prompt",
      "default": true
    },
    "watermark": {
      "type": "boolean",
      "title": "添加 AI 生成水印",
      "default": false
    },
    "seed": {
      "type": "integer",
      "title": "随机种子",
      "minimum": 0,
      "maximum": 2147483647
    }
  }
}`

// GetDefaultParamSchema 返回某个 modality 的默认 JSON Schema 字符串，未知
// modality 返回空对象。
func GetDefaultParamSchema(modality string) string {
	if s, ok := DefaultParamSchemas[modality]; ok {
		return s
	}
	return `{"type":"object","properties":{}}`
}

func GetDefaultModelParamSchema(modelName string) string {
	return DefaultModelParamSchemas[modelName]
}
