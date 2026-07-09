# Poyo 聚合渠道接入方案

> 状态:已定稿,开工中
> 最后更新:2026-07
> 目标:接入 poyo(https://api.poyo.ai)聚合中转站的图片(优先 seedream)、后续视频模型,
> 保持站内"同步返回 base64"的统一体验,并让生图结果可在日志面板回看。

---

## 0. 背景与核心矛盾

- poyo 是聚合中转站,一个 key 可调多厂商的图片/视频模型。我们接它的 seedream,
  上游实际走火山方舟,但对外仍以火山官方模型协议 + 站内统一 OpenAI 协议暴露。
- **优先接入(测试用)**:`seedream-5.0-lite`、`seedream-4.5` 两个生图模型。
- **核心矛盾**:站内生图是「单次同步 + 返回 base64」;poyo 是「异步(submit+poll)+ 只回 URL(24h)」。
  → 方案在渠道适配器内部做「异步转同步」,对客户端保持同步 base64,无感知。

### Poyo API 摘要(实测/文档)

| 项 | 内容 |
|---|---|
| Base URL | `https://api.poyo.ai` |
| 鉴权 | `Authorization: Bearer <key>` |
| 提交 | `POST /api/generate/submit` → `{ model, callback_url?, input:{ prompt, size, n, image_urls?, enable_safety_checker? } }` |
| 提交返回 | `{ code:200, data:{ task_id, status:"not_started", created_time } }` |
| 查询 | `GET /api/generate/status/{task_id}` → `{ code, data:{ status, progress, files:[{ file_url, file_type }] } }` |
| 结果 | 只给 URL(24h 有效),**无 base64**,**无 token/usage** |
| 计费 | poyo 侧扁平**按张**(seedream 5 credits/张,不分辨率、不变体) |
| 限速 | 20 次/10s(每 key),100+ 并发任务 |
| Seedream 模型名 | `seedream-5.0-lite(-edit)`、`seedream-4.5(-edit)` |
| input 字段 | `prompt`(≤3000)、`size`(`2K`/`4K`/比例/`WxH`/`{width,height}`)、`n`(1–15)、`image_urls`(edit,1–10)、`enable_safety_checker` |

---

## 1. 与现有架构的对应

站内已有两条成熟路径,poyo 各占一条:

1. **同步生图** `channel.Adaptor`(参考 `volcengine` 现接的 seedream-4.0、`jimeng`)
   入口 `relay/image_handler.go:23`:`ConvertImageRequest → DoRequest → DoResponse`,
   一个 HTTP 往返内把 base64 写回。`jimeng/image.go:32` 是组装 `dto.ImageResponse{B64Json}` 的范例。
   → **poyo 图片走这条**,在 `DoResponse` 内部轮询,把异步伪装成同步。

2. **异步任务** `TaskAdaptor`(参考 `task/doubao` 的 Seedance 视频)
   submit→存 `model.Task`→后台 15s 轮询(`service/task_polling.go`)→客户端查 task_id。
   → **poyo 视频(后续)走这条**,保持异步,不做同步阻塞。

---

## 2. 锁定决策一览

| 维度 | 决策 |
|---|---|
| 图片渠道 | 新建同步 `channel.Adaptor`(`relay/channel/poyo/`) |
| 异步转同步 | submit→请求内轮询→下载→**base64** 同步回,客户端无感 |
| 结果形态 | 统一 base64(忽略 `response_format=url`) |
| 轮询超时 | 随张数缩放 **`60s + 30s×n`**,上限 600s;`SyncImagePollTimeout` 退化为可配下限。实测 n=2 约 51s、n=4 约 78s |
| 端点 | `/v1/images/generations`(+`/edits`)+ 火山原生 `/api/v3/images/generations`(raw) |
| 对外命名 | `doubao-seedream-4-5`、`doubao-seedream-5-0-lite`(火山原生风格去日期);一个 id 兼文生/图生 |
| `-edit` | 内部细节,model_mapping 之后按"有无图输入"拼;不进模型列表 |
| 映射 | 复用渠道自带 `model_mapping`(`model/channel.go:40`),零新增 |
| 计费 | 纯按张:每模型一口价 × 分组 × n;不分档;`OtherRatios` 留未来分档后手 |
| 归档回看 | poyo 内联下载字节→R2→`Log.Other["image_url"]`→前端 log 展开行 `<img>` |
| R2 | 必启用;只兜底上传偶发失败(不塞 url、不阻断出图) |
| 可见性 | 属主见自己、管理员见全部(`image_url` 不进 `formatUserLogs` 剥离名单) |
| 多账号 | 渠道多 key(`ChannelIsMultiKey`)或多渠道,原生支持,零代码 |

### 命名与 -edit 内部路由

```
对外 doubao-seedream-4-5
  └─(渠道 model_mapping)→ UpstreamModelName = "seedream-4.5"
       └─(请求带 image?)→ 有图: "seedream-4.5-edit" / 无图: "seedream-4.5"   ← adaptor 内部拼 -edit
```
- 顺序关键:**先 model_mapping,后拼 -edit**(与 `task/doubao/adaptor.go:433` 同套路)。
- 计费/日志记 `OriginModelName`(对外名);`UpstreamModelName` 仅用于打 poyo。

### 建议的渠道 model_mapping

```json
{
  "doubao-seedream-4-5": "seedream-4.5",
  "doubao-seedream-5-0-lite": "seedream-5.0-lite"
}
```

---

## 3. 改动清单(按期 × 文件)

### 一期 — 文生图 + 图生图跑通
- `constant/channel.go`:`ChannelTypePoyo`(插在 `ChannelTypeAmux=58` 与 `ChannelTypeDummy` 之间,值=59;Dummy 自动顺延,它是 `controller/model.go:95` 循环上界)+ `ChannelBaseURLs` 追加 `https://api.poyo.ai` + `ChannelTypeNames` 加 `"Poyo"`
- `constant/api_type.go`:`APITypePoyo`
- `common/api_type.go:5`:`ChannelType2APIType` 加 `case ChannelTypePoyo → APITypePoyo`
- `relay/relay_adaptor.go:54`:`GetAdaptor` 加 `case APITypePoyo → &poyo.Adaptor{}`
- `relay/channel/poyo/`:`adaptor.go`(ConvertImageRequest:文生+图生,按有无参考图拼 `-edit`;`collectImageURLsFromRequest` 从 image/images 取 http(s) URL)+ `constants.go`(对外两模型 + 请求/响应结构)+ `image.go`(submit/poll/base64)
- `setting/model_setting/global.go`:`SyncImagePollTimeout` map + 默认 + `GetSyncImagePollTimeout()`(复用 `matchPatterns`)
- `controller/channel-test.go`:加 poyo 探活分支(见 §6-③)
- 前端:渠道类型下拉加 "Poyo" + 默认 baseurl/模型/model_mapping 预填
- 配置侧(非开发):两模型各配按次价

**图生图参考图**:poyo 的 `image_urls` 只吃 URL。**adaptor 只透传请求里已经是 http(s) URL 的参考图,不做任何托管**(裸 base64/上传文件是调用方的事)。参考图的 R2 托管由**通用 presign 上传**(`controller/upload.go` 的 `PresignUpload` → `POST /api/upload/presign` → `storage.PresignPut`,已复用给操练场与 API key 用户)在**调用方/操练场侧**完成,到 adaptor 时已是 URL。有任意参考图 → 内部切 `-edit`,对外仍同一个模型 id。`/v1/images/generations`(JSON 带 image URL)与 `/v1/images/edits` 两个入口都覆盖。

### 二期 — 归档 + log 回看
- `service/storage/archive.go`:`ArchiveImageToR2`(对标 `ArchiveVideoToR2` @ `archive.go:110`)
- `service.MaybeArchiveGeneratedImage(c, bytes)` → `c.Set("image_archive_url")`(poyo `DoResponse` 内联调)
- `service/text_quota.go:402` 附近:`if u := c.GetString("image_archive_url"); u != "" { other["image_url"] = u }`
- `web/src/hooks/usage-logs/useUsageLogsData.jsx` ~447(type===2 块):`other.image_url` 时 push `<img>`

### 三期 — 火山原生端点
- `router/relay-router.go`:注册火山原生 `/api/v3/images/generations` 入站路由,置 `poyo_raw_format`;
  adaptor 识别 raw 按火山字段解析(字段对照表实现时补,不支持的静默丢弃)
- (图生图已在一期完成)

### 四期 — 透传渠道归档
- volcengine/jimeng 等的生图归档:异步 worker + log 事后 UPDATE(仅主节点跑,注意部署拓扑)

### 五期 — poyo 视频
- `relay/channel/task/poyo/`:实现 `TaskAdaptor`,照抄 `task/doubao`;走异步 + 客户端查 task_id;
  超时走 task 路径的 `TaskTimeoutMinutes`,与图片隔离

---

## 4. 计费(纯按张)

- 站内按次计费机制:`text_quota.go:289-298`,`quota = ModelPrice × QuotaPerUnit × 分组倍率`,
  再乘 `OtherRatios` 里每个倍率。
- `n`(张数)由 `image_handler.go:127` 自动经 `OtherRatios` 乘入。
- 每模型各配一个按次价(4.5 一个价、5-lite 一个价)→ 正好对上 poyo 扁平按张成本,**不用分档**。
- 未来若接按分辨率收费的模型:adaptor 里 `info.PriceData.AddOtherRatio("size", x)` 即可叠加,不碰计费主干。
- gpt-image-1 的 size→价格表(`setting/operation_setting/tools.go:152`)是**硬编码特例**,不复用。

---

## 5. 轮询超时(按模型可配)

`setting/model_setting/global.go` 的 `GlobalSettings` 新增:

```go
SyncImagePollTimeout        map[string]int `json:"sync_image_poll_timeout"`         // pattern → 秒
SyncImagePollTimeoutDefault int            `json:"sync_image_poll_timeout_default"` // 全局兜底,如 90
```

出厂默认(与 `DefaultCustomModalityPatterns` 并列):

```go
var DefaultSyncImagePollTimeout = map[string]int{
    "seedream": 120,   // 包含匹配:doubao-seedream-4-5 / 5-0-lite 都命中 → 2 分钟
}
```

解析(复用 `matchPatterns` @ `global.go:311`,按 `OriginModelName` 匹配):

```go
func GetSyncImagePollTimeout(model string) time.Duration {
    for pat, sec := range GlobalSettings.SyncImagePollTimeout {
        if matchPatterns(model, []string{pat}) { return time.Duration(sec) * time.Second }
    }
    return time.Duration(GlobalSettings.SyncImagePollTimeoutDefault) * time.Second
}
```

**以后接别的图片模型三级兜底(都不改代码)**:①命中已有 pattern ②面板加一条 ③全局默认。

**图片 vs 视频超时按代码路径隔离**:图片走本节;视频走 task 路径 `TaskTimeoutMinutes`,互不干扰。

---

## 6. 集成点(实现注意)

- **① 火山原生端点需新建入站路由**:现有 `/v1/images/generations` 在 `relayV1Router`(`relay-router.go:129`);
  `/api/v3/images/generations` 入站不存在,要新加组注册并置 `poyo_raw_format`。与 volcengine 无冲突
  (那是上游 URL,非入站路由)。
- **② 渠道探活 = 真跑一次生图**:`channel-test.go:140` 已有 `VolcEngine+seedream→/v1/images/generations` 先例;
  poyo 加同款分支。注意探活会真提交+轮询、扣 poyo credits、耗数秒~数十秒。
- **③ edits 模式**:`RelayModeImagesGenerations` / `RelayModeImagesEdits` 都有(`relay_mode.go:14-15`);
  `ConvertImageRequest` 两种 mode 都接,edits 必走 `-edit`,generations 带 image 也走 `-edit`。

---

## 7. 风险登记册

| # | 风险 | 状态 | 处理 |
|---|---|---|---|
| 🔴1 | 同步阻塞 × 渠道重试放大 | 一期必做 | **失败分流**见 §8;网关读超时 ≥150s |
| 🔴2 | poyo 20次/10s 限速 | 一期方案 | 首轮延迟+间隔退避+429退避 + 多 key;webhook 留二期 |
| 🟠3 | 归档通用化时序冲突 | 已拆期 | 一期只 poyo 内联;透传渠道四期 worker+log-update |
| 🟠4 | 上游已出图但后续失败→成本泄漏 | 可接受 | 记 upstream task_id 对账 |
| 🟡5 | 火山原生字段 poyo 未必吃 | 三期 | 能映射的映射、不支持静默丢;给字段对照表确认 |
| 🟡6 | 规范 | 知悉 | 新代码用 `common.Marshal`;i18n;探活分支 |
| 🟢7 | log 渲染判据 | 已定 | 以 `other.image_url` 存在为准,不依赖 `other.image` |

---

## 8. 🔴1 / 🔴2 的处理建议(定稿)

### 🔴1 失败分流(只让"提交前快失败"可重试)

| 失败点 | 何时 | 处理 | 理由 |
|---|---|---|---|
| 提交失败(网络/4xx/5xx/key失效/模型不存在) | 提交阶段,秒级 | **可重试** | 快,换 key/渠道可能就好 |
| 提交 429 | 提交阶段 | **可重试** | 自动 failover 到另一把 key/渠道,分摊限速 |
| 轮询超时(`60s+30s×n` 内没出图) | 轮询阶段,已耗时 | **SkipRetry** | 再试=重新提交+再等+再扣费,灾难 |
| 任务失败(poyo 回 failed) | 轮询阶段 | **SkipRetry** | 重试同结果、再扣费 |
| 下载超时/超总量上限 | 下载阶段,图已出 | **SkipRetry** | 上游已计费,重试=重新提交+再扣费 |

- 落地:poyo `DoResponse` 对后三类加 `types.ErrOptionWithSkipRetry()`。
- 硬约束:**网关/nginx `proxy_read_timeout` ≥ 750s**。上界 = 轮询 600s + 下载
  `downloadPhaseTimeout` 120s + 余量。超时是 SkipRetry、单提交单轮询窗,不堆叠。
- ⚠️ 已知缺口:`RELAY_TIMEOUT` 默认 0 → `service.GetHttpClient()` 无 Timeout,而
  `GetImageFromUrl` 不接受 ctx。`downloadPhaseTimeout` 只在两张图**之间**生效,
  单张下载卡死时拦不住。缓解:设置 `RELAY_TIMEOUT` 环境变量;根治需要给
  `service.GetImageFromUrl` 加 ctx 版本(影响全站,另议)。

### 🔴2 限速(一期简单方案 + webhook 后手)

- **少轮**:首轮延迟 4~5s 再开始(图不可能秒出),之后间隔 3s;命中 429 对该请求指数退避,
  **绝不把 429 当任务失败**,超时窗内继续等。
- **多 key**:20次/10s 是每 key 的 → 渠道填 N 把 key,额度 ×N,零代码。**有并发就建渠道时填多 key**。
- **webhook(二期)**:poyo 支持 `callback_url`,用回调替代轮询可把轮询限速压力归零,
  但需"webhook 唤醒被阻塞请求"的改造(task_id 做 pubsub 唤醒)。一期不做,多 key+退避扛不住再上。

---

## 9. 配置流程(方案落地后,管理员操作)

1. 渠道管理 → 新增渠道 → 类型选 **Poyo**(默认带出 Base URL)
2. 填 **API Key**(可多把)
3. **模型**填对外 id:`doubao-seedream-4-5`、`doubao-seedream-5-0-lite`
4. **model_mapping** 填官方 id→poyo 名(前端预填)
5. **定价**给两模型各配按次价

---

## 附:关键代码锚点

- 同步生图入口:`relay/image_handler.go:23`
- 适配器接口:`relay/channel/adapter.go:15`(`Adaptor`)/ `:34`(`TaskAdaptor`)
- 适配器工厂:`relay/relay_adaptor.go:54`(`GetAdaptor`)
- base64 组装范例:`relay/channel/jimeng/image.go:32`
- URL→base64:`service/image.go:69`(`GetImageFromUrl`)
- 计费:`service/text_quota.go`(`PostTextConsumeQuota` / 按次分支 `:289`)
- 视频归档模板:`service/storage/archive.go:110`(`ArchiveVideoToR2`)
- 日志模型:`model/log.go:40`(`Other`)/ `:62`(`formatUserLogs`)
- 前端日志渲染:`web/src/hooks/usage-logs/useUsageLogsData.jsx` / `components/table/usage-logs/UsageLogsTable.jsx:88`
- 模型 pattern 匹配:`setting/model_setting/global.go:311`(`matchPatterns`)
- 渠道 model_mapping:`model/channel.go:40` / `relay/helper/model_mapped.go:16`
</content>
</invoke>
<invoke name="Read">
<parameter name="file_path">/Users/lijianchao/isboyjc/remote/amux-api/constant/api_type.go