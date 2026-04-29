# 上游 Cherry-Pick 方案

**目标**：从 `upstream/main` 选择性合并高价值改动到 `develop`，避免全量 merge 的高冲突成本。

**关键边界**：上游 commit `a42b39760` (2026-04-28) 把 `web/src/` 重命名为 `web/classic/src/`。

| 时间分界 | 路径状态 | Cherry-pick 难度 |
|---|---|---|
| 在 `a42b39760` **之前** | `web/src/...` 与我们一致 | ✅ 路径直接兼容 |
| 在 `a42b39760` **之后** | 上游已 `web/classic/src/...` | ⚠️ 需手工 path-remap 或仅取后端部分 |

幸运的是，**90% 的高价值修复都在 rename 之前**。

---

## 推进方式

每一波建议在独立分支做，跑一次 `go build ./...` + 抽查影响功能后 PR 合回 develop。

```bash
git checkout develop
git pull origin develop
git checkout -b cherry/wave-1-security
# 逐个 cherry-pick，遇到冲突就停下解决
git cherry-pick <hash>
go build ./...
# 通过后继续下一个；全部通过后 push + PR
```

每完成一波就更新本文档底部的 **"已 cherry-pick 记录"** 表。

---

## Wave 1 — 安全 / 正确性修复（推荐立刻做）

**特点**：纯后端、路径兼容、风险极低、收益明确。

| 优先级 | Commit | 说明 | 预计冲突 | 文件改动 |
|---|---|---|---|---|
| 🔴 高 | `e2807c5f9` | SSRF 防护增强（识别保留地址、链路本地等） | 无 | `common/ssrf_protection.go` 单文件 |
| 🔴 高 | `925342622` | 禁用/删除/升降级用户时清掉 user + token cache | 可能小冲突（我方可能改过 ManageUser） | `controller/user.go`、`model/user_cache.go` |
| 🔴 高 | `095e1920f` | 渠道刷新模型时先加载 model_mapping | 无 | `model/channel.go` |
| 🔴 高 | `4e93148d9` | Config map 字段 unmarshal 修复 | 无 | `model/option.go` |
| 🔴 高 | `bee339d27` | sync 延迟期 ratio/price 仍序列化（fallback 不丢） | 无 | `setting/ratio_setting/...` |
| 🔴 高 | `8ca103342` | `Message.Reasoning/ReasoningContent` 改 `*string` | 可能影响 dto 调用方 | `dto/openai_request.go` 等 + 配套 service |
| 🔴 高 | `8ca103342` 的兄弟 `6f57dcd2f` | Delete `dto/message_reasoning_test.go` | 无 | 测试删除 |
| 🟠 中高 | `b2e62a44e` | 充值搜索 30 天窗口 + COUNT hard limit + sanitizeLikePattern | ⚠️ **会冲突** — 我方 `model/topup.go` 有 `TopupListItem` + 多字段搜索扩展 | `model/topup.go`、`controller/topup.go` |
| 🟠 中高 | `a7c38ec85` | TopUp 加 `PaymentProvider` 字段防跨网关回调 | ⚠️ **大概率冲突** — 跟我方 topup 改造重叠 | `model/topup.go`、所有 `controller/topup_*.go` |

**说明**：
- 后 2 项必然冲突（我们之前合并时已经手工处理过）。可以参考之前的合并经验：
  - `b2e62a44e`：保留我方 `TopupListItem` + 多字段管理员搜索；只把 `sanitizeLikePattern` + 30 天窗口 + COUNT limit 这部分逻辑加进来。
  - `a7c38ec85` + Waffo `f995a868e`：把 `PaymentProvider` 字段加上，配合更新 Stripe/Creem/Waffo/Pancake 等回调里的 `PaymentProvider` 校验逻辑。
- 验证：`go build ./...`、登录后台手动充值测试 1 笔、禁用一个用户看 token 是否立即失效。

---

## Wave 2 — 渠道兼容性修复（高价值，纯后端）

**特点**：直接修 LLM 协议兼容性，影响实际可用性。

| 优先级 | Commit | 说明 | 预计冲突 |
|---|---|---|---|
| 🔴 高 | `45cc95a25` | Gemini ToolConfig 加 `IncludeServerSideToolInvocations` | 无 |
| 🔴 高 | `db89b57e1` | 工具调用 arguments 兼容直接 JSON 对象（不止字符串） | 无 |
| 🔴 高 | `899338674` + `435d7ae0d` | DeepSeek V4 reasoning 后缀 marker 处理 | 无 |
| 🟠 中 | `47d7bca26` | claude-opus-4-7 模型常量 / ratio | 可能冲突（我方可能加了别的模型） |
| 🟠 中 | `df6d86289` | gpt-5.5 completion ratio 修正 | 可能冲突 |
| 🟠 中 | `69ba18d39` | image N 倍率仅作用于 image 模型 | 无 |
| 🟢 低 | `097a50ebd` + `355307223` | affinity 提示文案 | 可能 i18n 冲突 |
| 🟢 低 | `62d4b63fc` | native messages 模型匹配配置 | 可能冲突 |
| 🟢 低 | `86cfb3920` | Ali Anthropic messages 匹配 | 看 PR 里具体涉及文件 |

---

## Wave 3 — Codex / 渠道维护改进（中价值）

| 优先级 | Commit | 说明 | 预计冲突 |
|---|---|---|---|
| 🟠 中 | `e729b2219` | 自动禁用 codex 渠道恢复测试时刷新凭据 | 无 |
| 🟠 中 | `5f67d2a28` | codex auto test 用 stream | 无 |
| 🟠 中 | `4c21c4c43` | "获取模型"显示已下架但本地仍有的模型 | 看是否触前端 |
| 🟠 中 | `f424f906d` | 同步上游 pricing endpoint | 看具体改动 |
| 🟢 低 | `d586a567e` | codex usage modal 折叠 raw JSON | **触前端** — 路径在 `web/src/...` 我们应该兼容 |

---

## Wave 4 — Token / 用户管理小功能（中价值，触前端）

| 优先级 | Commit | 说明 | 预计冲突 |
|---|---|---|---|
| 🟠 中 | `1d83b5472` | passkey 修改需二次验证 | ⚠️ 触 `web/src/components/settings/PersonalSetting.jsx`，看我方有没改过 |
| 🟠 中 | `02aacb38a` | User 加 `LastLoginAt` + 列表显示 | ⚠️ 触 `model/user.go` + `web/src/components/table/users/UsersColumnDefs.jsx`；注意上游同 commit 还加了 `CreatedAt`，**我们只要 LastLoginAt** |
| 🟠 中 | `b60bc94f9` | tokens 表显示 last_used_at 列 | ⚠️ 触 `web/src/components/table/tokens/TokensColumnDefs.jsx` |
| 🟢 低 | `2d4bdd297` + `600ae8599` | 管理员充值账单显示 user_id | 可能冲突 |
| 🟢 低 | `81ddf6e72` + `0feb6f2c3` + `49474520e` + `2431efc01` | token 长 key 兼容 + 跨 DB 迁移测试 | 测试文件多，看是否纯增量 |

**`02aacb38a` 处理建议**：cherry-pick 后手动删掉 `User` 结构体里的 `CreatedAt` 字段（保留 `LastLoginAt`），保持我们既有的 `CreatedTime` 不变。

---

## Wave 5 — 充值日志审计 / 安全（中价值）

| 优先级 | Commit | 说明 | 预计冲突 |
|---|---|---|---|
| 🟠 中 | `209d90e86` | 充值日志加 admin-only 审计区 | 跟我方 topup 改造可能重叠 |
| 🟠 中 | `c31343ac7` | 用户可见管理日志隐藏管理员身份 | 待评估 |
| 🟠 中 | `6ff8c7ab0` | 旧日志展开提示 | 待评估 |
| 🟠 中 | `209645e26` | NODE_NAME 写入审计日志 | 无 |
| 🟢 低 | `6afaa58d2` | RechargeCard 漏 import Tag 修复 | 触前端 |

---

## Wave 6 — 依赖升级（无脑做）

| Commit | 说明 |
|---|---|
| `6c69d60fb` / `dd57eeb51` / `e2e479c11` | `pgx/v5` 5.7.1 → 5.9.2（含安全修复） |
| `346de0268` / `01c2e909a` | electron `@xmldom/xmldom` 0.8.12 → 0.8.13 |

直接 cherry-pick 或者手工编辑 `go.mod` / `electron/package.json` 都可以。

---

## Wave 7 — 分级计费 (Tiered Billing)（重大功能，需专项）

⚠️ **这是单独项目，不要混在以上 wave 里**。

涉及 commit（15+ 个）：
- `91ed4e196` `f0589cc47` `f6c0852da` `5b03b39db` `c5405b2a1` `6e3ef48c9`
- `44fc10ba9` `d66311e98` `3a2138ba6` `0220df842`
- `1fe9f6f98` `5c4ed5be9` `3e5f2ee1d` `8eeae0073` `9f8a4ec05`
- `eab478bdc` `e3d64cb76` `f2f3410dc` `63ce2db98`

新增 `pkg/billingexpr/`（设计文档 `expr.md` 说明清楚），扩展 `model/pricing.go`（`BillingMode` / `BillingExpr` 字段），前端有 `TieredPricingEditor`、`DynamicPricingBreakdown`、`render.jsx` 大改。

**建议**：当**专项 epic 处理**：
1. 先决定要不要这个能力（如果你只用统一倍率就跳过）
2. 要的话单独开分支 `feature/tiered-billing`，把整组 commit 一次性 squash merge 进来，单独 review + 测试

---

## 跳过列表（明确不要）

| Commit | 原因 |
|---|---|
| `a42b39760` | 整套新前端 web/default/，单独迁移项目，不要 cherry-pick |
| `e0b6eb3a5` `22ae14f0d` `f98254482` `438410708` `75af3db11` `db48108d2` `22ef5b2f8` `28f7e9eb2` `fc377dae3` `d385d7abf` `3b592895c` | 纯 web/default/ UI，不影响我方 |
| `df14a0bf1` `fbca2561e` | 上游 CI workflow（推 Docker Hub） |
| `c609cb13b` | 上游 README logo 改 |
| `d75a04679` | 上游 docker-compose 改默认 redis 密码（我方 Dokploy 配置无关） |

---

## 推荐执行顺序

```
Wave 6 (依赖升级)  ← 5 分钟，最无风险，先做
Wave 1 (安全修复)  ← 收益最大，做完跑一次冒烟测试
Wave 2 (渠道兼容) ← 直接影响 LLM 调用质量
Wave 3 (Codex)
Wave 4 (用户/Token)
Wave 5 (审计日志)
Wave 7 (分级计费) ← 评估清楚再开专项
```

每波之间留缓冲，跑一次：
```bash
go build ./...
cd web && bun run build  # 如果触到前端
```

部署到测试环境观察一段时间再合下一波，避免一次合太多出问题难定位。

---

## 已 Cherry-Pick 记录

> 每完成一个就追加一行，记录 commit hash + 落地到 develop 的 commit hash + 日期。
> 下次评估上游新 commit 时，只看「最后 cherry-pick 的上游 hash 之后」的内容。

**当前 upstream/main 顶端**：`3b592895c`（2026-04-29 fetch 时）
**Cherry-pick 分支**：`cherry-pick/upstream-essentials`

### 2026-04-29 第一批（29 commits）

**Wave 6 — 依赖升级**

| 上游 hash | 说明 | 处理方式 |
|---|---|---|
| `dd57eeb51` + `6c69d60fb` | pgx/v5 5.7.1 → 5.9.2 | 手工 `go get`（中间版本冲突） |
| `346de0268` | xmldom 0.8.12 → 0.8.13 | cherry-pick 干净 |

**Wave 1 — 安全/正确性**

| 上游 hash | 说明 |
|---|---|
| `e2807c5f9` | SSRF 防护增强 |
| `095e1920f` | 渠道刷新模型时先加载 model_mapping |
| `4e93148d9` | Config map 字段 unmarshal 修复 |
| `925342622` | 禁用用户时清缓存 |
| `8ca103342` | Reasoning/ReasoningContent 改 *string |
| `6f57dcd2f` | 删除测试文件 followup |
| `ce66bb93f`(对应 `b2e62a44e`) | topup 搜索 DoS 加固（手工合并冲突，保留我方 TopupListItem 结构） |

**Wave 2 — 渠道兼容**

| 上游 hash | 说明 |
|---|---|
| `45cc95a25` | Gemini ToolConfig.IncludeServerSideToolInvocations |
| `db89b57e1` | 工具调用 raw JSON arguments 兼容 |
| `435d7ae0d` | DeepSeek V4 reasoning 后缀处理 |
| `47d7bca26` | claude-opus-4-7 支持 |
| `6c922ffc0`(对应 `69ba18d39`) | image N 倍率仅作用于 image 模型 |
| `bd4b104fe`(对应 `df6d86289`) | gpt-5.5 completion ratio 修正 |
| `5fe9d808e`(对应 `355307223`) | affinity 重试提示 |
| `102bd82b5`(对应 `62d4b63fc`) | native messages 模型匹配配置 |

**Wave 3 — Codex**

| 上游 hash | 说明 |
|---|---|
| `e729b2219` | 自动禁用 codex 渠道恢复时刷新凭据 |
| `5f67d2a28` | codex auto test 用 stream 模式 |
| `d586a567e` | codex usage modal 折叠 raw JSON |
| `4c21c4c43` | 显示已下架但本地仍有的模型 |

**Wave 4 — 用户/Token UI**

| 上游 hash | 说明 |
|---|---|
| `1d83b5472` | passkey 修改需二次验证 |
| `02aacb38a` | User 加 LastLoginAt（**手工跳过 CreatedAt**，保留我方 CreatedTime） |
| `b60bc94f9` | tokens 表显示 last_used_at 列 |
| `2431efc01` `81ddf6e72` `0feb6f2c3` `49474520e` | 旧 token 长 key 兼容 + 跨 DB 迁移测试 |

### 已跳过 / 推迟

| 上游 hash | 原因 |
|---|---|
| `bee339d27` | 依赖 tiered_billing（Wave 7 范围） |
| `f424f906d` | 同上 |
| `a7c38ec85` (PaymentProvider) | 涉及 12 个文件互动且依赖上游中间状态，无法直接 cherry-pick；**待单独 epic 手工实现核心字段 + 跨网关校验** |
| Wave 5 全部（`209d90e86` `c31343ac7` `6ff8c7ab0` `209645e26` `2d4bdd297` `600ae8599`） | 主要为 i18n 翻译键扩展 + topup.go 二次改造，与我方 RecordTopupLog 已有审计能力高度重叠，i18n 冲突量级大；本期跳过 |

### 待单独立项

- **PaymentProvider 跨网关回调校验**：手工添加 `PaymentProvider` 字段到 TopUp / SubscriptionOrder + 在每个回调入口校验。安全价值高，建议下一轮独立做。
- **Wave 7 阶梯计费 (Tiered Billing)**：`pkg/billingexpr/` 完整体系，独立 epic。

**最后 cherry-pick 的 upstream hash**：以本批为基线，下次只看 `2026-04-29` 之后的新 upstream commit。

---

## 一些通用经验

1. **cherry-pick 失败时**先看冲突文件性质：
   - 我方有定制 → 手工 merge，保留我方语义 + 增量加上游补丁
   - 我方没动 → 一般 `git checkout --theirs` 就好

2. **跨多 commit 的特性**（如 PaymentProvider 涉及 a7c38ec85 + Waffo + Pancake）一次性全部 cherry-pick，避免半截状态编译不过。

3. **触 i18n 的 commit**（locale json）小心处理，手工 merge 而不是 cherry-pick 整文件。我们 7 个语言包，每次 merge 都要 union。

4. **业务决策保留我方**：
   - Stripe 充值额度计算（`Amount * QuotaPerUnit`）
   - Stripe 设置面板（保留所有高级开关）
   - docker-compose、CI workflow（amux-api 自有部署）
   - User.CreatedTime（不切到 CreatedAt）
