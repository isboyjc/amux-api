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

// GetDefaultParamSchema 返回某个 modality 的默认 JSON Schema 字符串，未知
// modality 返回空对象。
func GetDefaultParamSchema(modality string) string {
	if s, ok := DefaultParamSchemas[modality]; ok {
		return s
	}
	return `{"type":"object","properties":{}}`
}
