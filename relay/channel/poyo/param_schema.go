package poyo

// Seedream45ParamSchema 是 Seedream 4.5 模型的参数 JSON Schema。
// 前端操练场根据此 schema 渲染右侧栏参数控件。
//
// 关键点:
//   - resolution: 分辨率档位(2K/4K),可与 aspect_ratio 组合使用
//   - aspect_ratio: 宽高比,可与 resolution 组合使用
//   - 前端根据两者计算最终 size 值传给后端(见 useImageGeneration.js 的处理逻辑)
//   - n: 生成数量 1-15 张(Poyo 限制)
//   - enable_safety_checker: 安全检查开关
//   - image: 图生图参考图槽位,声明 format:image 触发前端上传 UI
const Seedream45ParamSchema = `{
  "type": "object",
  "properties": {
    "resolution": {
      "type": "string",
      "title": "分辨率档位",
      "description": "质量档位,可与宽高比组合",
      "enum": ["2K", "4K"],
      "default": "2K"
    },
    "aspect_ratio": {
      "type": "string",
      "title": "宽高比",
      "description": "图片比例,可与分辨率档位组合",
      "enum": ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "21:9"]
    },
    "n": {
      "type": "integer",
      "title": "生成数量",
      "description": "一次生成的图片张数",
      "minimum": 1,
      "maximum": 15,
      "default": 1
    },
    "enable_safety_checker": {
      "type": "boolean",
      "title": "启用安全检查",
      "description": "检测并过滤不当内容",
      "default": true
    },
    "image": {
      "type": "array",
      "title": "参考图",
      "description": "图生图模式:上传 1-10 张参考图",
      "items": {
        "type": "string",
        "format": "image"
      },
      "minItems": 1,
      "maxItems": 10
    }
  }
}`

// Seedream50LiteParamSchema 是 Seedream 5.0-lite 模型的参数 JSON Schema。
// 与 4.5 的区别:支持 2K/3K 档位(不支持 4K)。
const Seedream50LiteParamSchema = `{
  "type": "object",
  "properties": {
    "resolution": {
      "type": "string",
      "title": "分辨率档位",
      "description": "质量档位,可与宽高比组合",
      "enum": ["2K", "3K"],
      "default": "2K"
    },
    "aspect_ratio": {
      "type": "string",
      "title": "宽高比",
      "description": "图片比例,可与分辨率档位组合",
      "enum": ["1:1", "4:3", "3:4", "16:9", "9:16", "3:2", "2:3", "21:9"]
    },
    "n": {
      "type": "integer",
      "title": "生成数量",
      "description": "一次生成的图片张数",
      "minimum": 1,
      "maximum": 15,
      "default": 1
    },
    "enable_safety_checker": {
      "type": "boolean",
      "title": "启用安全检查",
      "description": "检测并过滤不当内容",
      "default": true
    },
    "image": {
      "type": "array",
      "title": "参考图",
      "description": "图生图模式:上传 1-10 张参考图",
      "items": {
        "type": "string",
        "format": "image"
      },
      "minItems": 1,
      "maxItems": 10
    }
  }
}`
