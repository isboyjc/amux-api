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
	"happyhorse-1.1-t2v":        happyHorseT2VParamSchema,
	"happyhorse-1.0-t2v":        happyHorseT2VParamSchema,
	"happyhorse-1.1-i2v":        happyHorseI2VParamSchema,
	"happyhorse-1.0-i2v":        happyHorseI2VParamSchema,
	"happyhorse-1.1-r2v":        happyHorseR2VParamSchema,
	"happyhorse-1.0-r2v":        happyHorseR2VParamSchema,
	"happyhorse-1.0-video-edit": happyHorseVideoEditParamSchema,
	"wan2.7-i2v-2026-04-25":     wan27VideoParamSchema,
	"MiniMax-H3":                minimaxH3VideoParamSchema,
	// Seedance 2.5 的每个可用名字都要登记一份：这张表是【精确匹配模型名】的，
	// 漏一个，管理员用那个名字配渠道时操练场就拿不到参数面板和素材上传槽——
	// 请求能发、钱也收得对，唯独右栏是空的，很难往 schema 上想。
	// 名字与 relay/channel/task/doubao/constants.go 的 seedanceAliasMap 对齐。
	"doubao-seedance-2-5":        seedance25VideoParamSchema,
	"doubao-seedance-2-5-260628": seedance25VideoParamSchema,
	"doubao-seedance-2.5":        seedance25VideoParamSchema,
	"seedance-2.5":               seedance25VideoParamSchema,
	"seedance-2.5-api":           seedance25VideoParamSchema,
}

// seedance25VideoParamSchema 对应火山方舟 Seedance 2.5。
// 媒体槽的 x-content-role 与 relay/channel/task/doubao/seedance25.go 的 role
// 常量一一对应：操练场按 role 把上传素材拍平成 metadata.content，适配器再按
// role 还原。
// https://docs.volcengine.com/docs/82379/1520757
const seedance25VideoParamSchema = `{
  "type": "object",
  "x-prompt-optional": true,
  "properties": {
    "first_frame_image": {
      "type": "string",
      "format": "image",
      "title": "首帧",
      "x-content-role": "first_frame",
      "x-max-mb": 30
    },
    "last_frame_image": {
      "type": "string",
      "format": "image",
      "title": "尾帧",
      "x-content-role": "last_frame",
      "x-max-mb": 30
    },
    "reference_images": {
      "type": "array",
      "title": "参考图",
      "description": "最多 30 张。与首/尾帧互斥，不能混用",
      "maxItems": 30,
      "items": {"type": "string", "format": "image"},
      "x-content-role": "reference_image",
      "x-max-mb-per-item": 30
    },
    "reference_videos": {
      "type": "array",
      "title": "参考视频",
      "description": "最多 10 段，单段 2~30 秒，合计不超过 30 秒。与首/尾帧互斥",
      "maxItems": 10,
      "items": {"type": "string", "format": "video"},
      "x-content-role": "reference_video",
      "x-max-mb-per-item": 200,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 30,
      "x-max-total-duration-seconds": 30
    },
    "reference_audios": {
      "type": "array",
      "title": "参考音频",
      "description": "最多 10 段，单段 2~30 秒，合计不超过 30 秒。与首/尾帧互斥",
      "maxItems": 10,
      "items": {"type": "string", "format": "audio"},
      "x-content-role": "reference_audio",
      "x-max-mb-per-item": 15,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 30,
      "x-max-total-duration-seconds": 30
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["480p", "720p"],
      "default": "720p"
    },
    "aspect_ratio": {
      "type": "string",
      "title": "宽高比",
      "description": "adaptive 由模型按输入内容自动适配。首/尾帧与视频编辑场景仅支持 adaptive",
      "enum": ["adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"],
      "default": "adaptive"
    },
    "duration": {
      "type": "integer",
      "title": "时长（秒）",
      "description": "「由模型决定」即官方的 duration=-1，模型在 4~30 秒内自选；视频编辑场景只支持这一项。此时按预扣秒数冻结额度，生成完成后按实际时长退还差额",
      "enum": [-1, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30],
      "enumLabels": {"-1": "由模型决定"},
      "default": -1
    },
    "generate_audio": {
      "type": "boolean",
      "title": "生成有声视频",
      "default": true
    },
    "output_format": {
      "type": "string",
      "title": "输出格式",
      "description": "mov 为高色彩精度专业格式，部分播放器不兼容",
      "enum": ["mp4", "mov"],
      "default": "mp4"
    },
    "web_search": {
      "type": "boolean",
      "title": "联网搜索",
      "description": "由模型自主判断是否搜索互联网内容，可提升时效性但会增加时延",
      "default": false
    },
    "watermark": {
      "type": "boolean",
      "title": "添加 AI 生成水印",
      "default": false
    }
  }
}`

// minimaxH3VideoParamSchema 对应 MiniMax v2 视频协议。
// 媒体槽的 x-content-role 与 relay/channel/task/hailuo/h3.go 的 role 常量一一
// 对应：操练场按 role 把上传素材拍平成 metadata.content，适配器再按 role 还原。
// https://platform.minimaxi.com/docs/api-reference/video-generation-v2-create
const minimaxH3VideoParamSchema = `{
  "type": "object",
  "properties": {
    "first_frame_image": {
      "type": "string",
      "format": "image",
      "title": "首帧",
      "x-content-role": "first_frame",
      "x-max-mb": 30
    },
    "last_frame_image": {
      "type": "string",
      "format": "image",
      "title": "尾帧",
      "x-content-role": "last_frame",
      "x-max-mb": 30
    },
    "reference_images": {
      "type": "array",
      "title": "参考图",
      "description": "最多 9 张，用于人物/风格参考",
      "maxItems": 9,
      "items": {"type": "string", "format": "image"},
      "x-content-role": "reference_image",
      "x-max-mb-per-item": 30
    },
    "reference_videos": {
      "type": "array",
      "title": "参考视频",
      "description": "最多 3 段，单段 2~15 秒，合计不超过 15 秒",
      "maxItems": 3,
      "items": {"type": "string", "format": "video"},
      "x-content-role": "reference_video",
      "x-max-mb-per-item": 50,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 15,
      "x-max-total-duration-seconds": 15
    },
    "reference_audios": {
      "type": "array",
      "title": "参考音频",
      "description": "最多 3 段，单段 2~15 秒，合计不超过 15 秒",
      "maxItems": 3,
      "items": {"type": "string", "format": "audio"},
      "x-content-role": "reference_audio",
      "x-max-mb-per-item": 15,
      "x-min-duration-seconds": 2,
      "x-max-duration-seconds": 15,
      "x-max-total-duration-seconds": 15
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["768P", "2K"],
      "default": "2K"
    },
    "aspect_ratio": {
      "type": "string",
      "title": "宽高比",
      "enum": ["adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16"],
      "default": "16:9"
    },
    "duration": {
      "type": "integer",
      "title": "时长（秒）",
      "minimum": 4,
      "maximum": 15,
      "default": 6
    },
    "aigc_watermark": {
      "type": "boolean",
      "title": "添加 AIGC 水印",
      "default": false
    }
  }
}`

const happyHorseT2VParamSchema = `{
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

const happyHorseI2VParamSchema = `{
  "type": "object",
  "x-prompt-optional": true,
  "properties": {
    "first_frame": {
      "type": "string",
      "format": "image",
      "title": "首帧图像",
      "x-content-role": "first_frame",
      "x-max-mb": 20
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["480P", "720P", "1080P"],
      "default": "1080P"
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
  },
  "required": ["first_frame"]
}`

const happyHorseR2VParamSchema = `{
  "type": "object",
  "properties": {
    "reference_images": {
      "type": "array",
      "title": "参考图像",
      "description": "按上传顺序在 Prompt 中使用 [Image 1]、[Image 2] 等标识引用",
      "minItems": 1,
      "maxItems": 9,
      "items": {"type": "string", "format": "image"},
      "x-content-role": "reference_image",
      "x-max-mb-per-item": 20
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["480P", "720P", "1080P"],
      "default": "1080P"
    },
    "ratio": {
      "type": "string",
      "title": "宽高比",
      "enum": ["16:9", "9:16", "3:4", "4:3", "4:5", "5:4", "1:1", "9:21", "21:9"],
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
  },
  "required": ["reference_images"]
}`

const happyHorseVideoEditParamSchema = `{
  "type": "object",
  "properties": {
    "source_video": {
      "type": "string",
      "format": "video",
      "title": "待编辑视频",
      "x-content-role": "reference_video",
      "x-max-mb": 100,
      "x-min-duration-seconds": 3,
      "x-max-duration-seconds": 60,
      "x-max-total-duration-seconds": 60
    },
    "reference_images": {
      "type": "array",
      "title": "参考图像",
      "maxItems": 5,
      "items": {"type": "string", "format": "image"},
      "x-content-role": "reference_image",
      "x-max-mb-per-item": 20
    },
    "resolution": {
      "type": "string",
      "title": "分辨率",
      "enum": ["720P", "1080P"],
      "default": "1080P"
    },
    "watermark": {
      "type": "boolean",
      "title": "添加 Happy Horse 水印",
      "default": true
    },
    "audio_setting": {
      "type": "string",
      "title": "声音控制",
      "enum": ["auto", "origin"],
      "enumLabels": {"auto": "模型自动处理", "origin": "保留原始声音"},
      "default": "auto"
    },
    "seed": {
      "type": "integer",
      "title": "随机种子",
      "minimum": 0,
      "maximum": 2147483647
    }
  },
  "required": ["source_video"]
}`

const wan27VideoParamSchema = `{
  "type": "object",
  "x-prompt-optional": true,
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
