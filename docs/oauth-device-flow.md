# OAuth 应用授权（Device Authorization Grant）

外部应用（桌面客户端、CLI、Web 应用等）接入 amux-api 获取**用户级别**访问令牌（OAT）的端到端流程。本文同时面向：

- **平台管理员**：如何注册一个外部 OAuth 应用
- **集成方开发者**：如何在你的应用里发起授权、拿到 access token、调用平台接口

---

## 1. 概述

amux-api 实现了**简化版** RFC 8628 OAuth Device Authorization Grant：

- 由外部应用主动发起授权请求，平台返回一个短期 session
- 用户在浏览器里访问授权页、登录、批准或拒绝
- 应用轮询服务端，被批准后**一次性**领取一把 OAT（OAuth Access Token）
- 后续调用平台所有受 `UserAuth` 保护的 API，把 OAT 放在 `Authorization` 头里即可

整个流程**不要求外部应用持有 client_secret**——这正是 device flow 适合桌面/CLI 客户端的原因（这类无法保密的客户端不应分发 secret）。`client_secret` 字段保留只为了未来上 Authorization Code Flow / Confidential Client 时复用。

### 名词速查

| 名词 | 含义 |
| --- | --- |
| `client_id` | 公开的应用标识，例如 `amux-desktop`。所有 device flow 请求都带它 |
| `client_secret` | 仅在创建/轮换时返回一次的明文密钥。Device Flow 当前**不要求**携带，未来其它 flow 复用 |
| `session_id` | 单次授权流程的标识，由**集成方生成的** UUID v4，5 分钟有效 |
| OAT | OAuth Access Token，前缀 `amux_api_oat_`，用户授权后签发给应用 |
| PAT | Personal Access Token，前缀 `amux_api_pat_`，用户在后台手动创建给自己用 |

OAT 和 PAT 共享 `user_access_tokens` 表 + 同一套校验逻辑，区别仅在 `source` 字段。

---

## 2. 整体时序

```
┌──────────┐                ┌──────────┐                ┌──────────┐
│ 外部应用 │                │ amux-api │                │   用户   │
│ (Client) │                │ (Server) │                │ (Browser)│
└────┬─────┘                └────┬─────┘                └────┬─────┘
     │                           │                           │
     │ 1. 生成 session_id (UUID) │                           │
     │                           │                           │
     │ 2. POST /oauth/device/authorize                       │
     │    {session_id, client_id}│                           │
     │ ─────────────────────────►│                           │
     │ ◄───────── 200 ───────────│                           │
     │                           │                           │
     │ 3. 引导用户打开 URL：     │                           │
     │    /oauth/authorize?session_id=...                    │
     │ ───────────────────────────────────────────────────► │
     │                           │                           │
     │ 4. 轮询 GET /oauth/device/check?session_id=...        │
     │ ─────────────────────────►│                           │
     │ ◄────── pending ──────────│                           │
     │ (loop 每 5s)              │                           │
     │                           │ 5. 用户登录 + 同意/拒绝   │
     │                           │ ◄─────────────────────── │
     │                           │ POST /oauth/device/confirm│
     │                           │ (UserAuth)                │
     │                           │                           │
     │                           │ 6. 服务端签发 OAT，写进   │
     │                           │    session.access_token   │
     │                           │                           │
     │ 7. 下一次轮询             │                           │
     │ ─────────────────────────►│                           │
     │ ◄── 200 {access_token} ──│  (原子消费，session 标 used)│
     │                           │                           │
     │ 8. 用 OAT 调业务接口      │                           │
     │ Authorization: Bearer ... │                           │
     │ ─────────────────────────►│                           │
```

session 5 分钟过期；超时未获批准时轮询返回 `expired`。

---

## 3. 管理员：注册一个 OAuth 应用

集成方需要先拿到一个 `client_id`。这是一次性配置，由 amux-api **root** 用户在后台完成。

### 3.1 入口

控制台 → **系统设置** tab → **OAuth 应用** 子 tab。

> 鉴权用 `RootAuth`：因为注册新 client 等于在平台上发布一个能拿任意用户 OAT 的应用，权限提升风险高，必须只对 root 开放。

### 3.2 创建表单字段

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| 应用名称 | 是 | 用户授权页会显示这个名字 |
| client_id | 否 | 留空自动从名字生成（如 `notion-ai-3jK9pQ`）。建议填一个可读 slug，例如 `acme-cli` |
| Logo URL | 否 | 用户授权页头部展示。填一个 https 公开图 URL |
| 主页 URL | 否 | 仅用于审计参考 |
| 联系邮箱 | 否 | 同上 |
| 描述 | 否 | 一句话告诉用户你的 app 是干什么的，授权页会展示 |
| 标记为已认证 | 否 | 勾选后授权页显示蓝色"已认证"徽章；只有 root 信任的应用才勾 |

提交后会**仅这一次**返回明文 `client_secret`（即使将来不强制携带，建议保存以备日后启用其它 flow）。

### 3.3 后续操作

- **轮换 secret**：泄漏或周期性更换时调用，旧 secret 立即失效
- **禁用**：`status=2` 之后该 `client_id` 不能再发起新授权（已签发的 OAT 仍可用，需到"管理员 → access tokens 治理"单独撤销）
- **不可删**：内置 `amux-desktop` 受保护，删除会被拒绝

---

## 4. 集成方：端到端拿到 access token

下面以伪代码描述四步流程，所有路径相对 amux-api host（例如 `https://api.amux.ai`）。

### Step 1：生成 session_id

由**客户端本地生成** UUID v4 字符串：

```python
import uuid
session_id = str(uuid.uuid4())  # "550e8400-e29b-41d4-a716-446655440000"
```

> session_id 必须是 8-4-4-4-12 格式的小写 UUID v4，服务端会用正则校验。

### Step 2：创建授权 session

```http
POST /api/oauth/device/authorize
Content-Type: application/json

{
  "session_id": "<UUID>",
  "client_id":  "<your-client-id>"
}
```

成功返回：

```json
{ "success": true, "data": null }
```

可能的失败：

| 场景 | 返回 |
| --- | --- |
| `client_id` 留空或缺失 | 服务端会兜底到内置 `amux-desktop`（兼容老 Amux Desktop） |
| `client_id` 在 `oauth_clients` 表里查不到 / 已禁用 | `MsgInvalidParams`（不会泄露具体原因） |
| `session_id` 格式非 UUID | `MsgInvalidParams` |
| 同 session_id 已存在 | `MsgDesktopAuthSessionExists` |
| 全平台并发 pending session 超过 1000 | `MsgRetryLater`（防滥用兜底） |
| 限流：CriticalRateLimit（默认 20 次 / 20 分钟，按 IP） | 429 |

### Step 3：把用户引导到授权页

打开浏览器访问：

```
https://<amux-api-host>/oauth/authorize?session_id=<UUID>
```

这是平台前端的页面，会：

1. 检查用户是否登录，未登录跳到 `/login?callback=...`，登录完自动回来
2. 调 `GET /api/oauth/device/info?session_id=...` 拉取 session 状态 + 你的 app 元信息（logo / 名字 / verified 标）
3. 渲染 "**{app_name} 请求接入你的账号**" 同意页
4. 用户点"确认授权"或"拒绝"

授权页会显示授权后应用能做什么：

- 获取你的账户基本信息
- 查看可用模型列表与分组信息
- 使用你的额度进行 API 调用

> 不需要在 URL 上带 redirect_uri / state / 等其它参数——session_id 已经把后续身份绑定在服务端。

### Step 4：轮询拿 token

集成端在 Step 2 完成之后立刻开始轮询：

```http
GET /api/oauth/device/check?session_id=<UUID>
```

返回三种状态：

| 状态 | 含义 | 处理 |
| --- | --- | --- |
| `{"status":"pending"}` | 用户还没操作 | 等 5 秒再轮 |
| `{"status":"authorized","user_id":<int>,"access_token":"amux_api_oat_..."}` | 用户已批准 | **立即保存 access_token**，停止轮询 |
| `{"status":"expired"}` | session 过期 / 已被消费 / 用户拒绝 | 抛错给用户，让 ta 重新发起授权 |

**关键：**

- 该端点用专用宽松限流 `OAuthPollRateLimit`（默认 120 次/60s/IP），可以放心 5 秒轮一次
- `authorized` 是一次性消费——服务端检测到首次成功 `check` 就把 session 改成 `used`、清空 token、再返回。下一次同 session_id 轮询会返回 `expired`。所以**集成端拿到 token 必须立刻持久化**
- session TTL 5 分钟，到期没拿到就视为失败

### 完整伪代码示例

```python
import requests, uuid, webbrowser, time

API_BASE = "https://api.amux.ai"
CLIENT_ID = "acme-cli"

def authorize_user():
    session_id = str(uuid.uuid4())

    # Step 2: 创建 session
    r = requests.post(f"{API_BASE}/api/oauth/device/authorize", json={
        "session_id": session_id,
        "client_id":  CLIENT_ID,
    })
    r.raise_for_status()

    # Step 3: 打开浏览器
    webbrowser.open(f"{API_BASE}/oauth/authorize?session_id={session_id}")
    print("请在浏览器中完成授权…")

    # Step 4: 轮询
    deadline = time.time() + 300   # 5 分钟超时
    while time.time() < deadline:
        time.sleep(5)
        r = requests.get(
            f"{API_BASE}/api/oauth/device/check",
            params={"session_id": session_id},
        )
        data = r.json()
        if data.get("status") == "authorized":
            return data["access_token"]
        if data.get("status") == "expired":
            raise RuntimeError("授权已过期或被拒绝")

    raise RuntimeError("授权超时")

token = authorize_user()
# 持久化 token，下次启动不需要重新授权
save_token(token)
```

---

## 5. 用 access token 调平台 API

把 OAT 放在 `Authorization` 头里即可：

```http
GET /api/user/self
Authorization: Bearer amux_api_oat_<40-char-base62>
```

> `Bearer ` 前缀是可选的，纯 token 也能识别。建议加上 `Bearer` 以符合规范。

OAT 校验路径（`middleware.UserAuth`）：

1. 提取 `Authorization` 头
2. 按前缀分流到 `ValidateUserAccessToken`（amux_api_pat_/amux_api_oat_）或老路径（旧 32 字符 token 兜底）
3. 用 `token_prefix`（前 16 字符）走索引查 `user_access_tokens` 表
4. 在内存里 `subtle.ConstantTimeCompare` SHA256 hash，防 timing attack
5. 检查 `status=active && expires_at > now`
6. 异步节流更新 `last_used_at` / `last_used_ip`（60 秒内不重复写库）
7. 加载对应用户、把 user_id / role / status 注入 gin.Context

之后这个请求**等同于该用户在浏览器登录**——所有 `UserAuth` 路由都可用。`AdminAuth`/`RootAuth` 路由仍按用户实际角色判断。

### 测试 token 是否有效

最简单：

```bash
curl -H "Authorization: Bearer amux_api_oat_..." \
     https://api.amux.ai/api/user/self
```

返回 200 + 用户基本信息即表示 OAT 有效。

---

## 6. Token 生命周期

```
┌─────────┐  user revoke /        ┌──────────┐
│ active  │─ rotate / pwd reset ─►│ revoked  │ (status=2，前端不展示)
└────┬────┘                       └──────────┘
     │ expires_at <= now
     │ 或 last_used_at < now-90d (空闲过期)
     ▼
┌─────────┐                       ┌──────────┐
│ expired │  user revoke ────────►│ revoked  │
└─────────┘                       └──────────┘
```

| 状态 | 含义 | UI 展示 | 是否能调 API |
| --- | --- | --- | --- |
| 1 active | 生效中 | ✅ | ✅ |
| 3 expired | 已过期（硬过期 / 90 天空闲） | ⚠️ 灰标 | ❌ |
| 2 revoked | 已撤销 | 🚫 不展示 | ❌ |

### 触发撤销的场景

- 用户在 **个人设置 → 安全设置 → 已授权应用** 主动点击"解除授权"
- 用户重置密码（自动撤销该用户**全部** active token，包括旧 legacy）
- 管理员从治理面板强制撤销
- 用户调"刷新"按钮——旧 token 撤销 + 同步签发新 token（PAT 才有此功能；OAT 没有，需要重新走授权）

### 默认空闲过期

- 90 天没用 → 状态切到 `expired`
- 由 `StartUserAccessTokenJanitor` 每 24 小时跑一次清理（启动时立即跑一次）

---

## 7. 错误处理速查

### Authorize 阶段

| 失败 | 客户端行为 |
| --- | --- |
| `client_id` 无效 / 已禁用 | 提示用户"应用不可用"，让运营联系 amux-api root 排查 |
| 限流 429 | 退避重试；通常说明该 IP 频繁创建 session（debug 期间手动重试） |
| 网络错误 | 重试一次；连续失败提示用户检查网络 |

### 用户授权页

| 行为 | 集成端表现 |
| --- | --- |
| 用户点"拒绝" | 服务端 ExpireDesktopSession，下一次 check 返回 `expired` |
| 用户关闭浏览器 / 不操作 | 5 分钟后 session 过期，check 返回 `expired` |
| 用户点"确认授权" | 服务端签发 OAT 写进 session.access_token |

### Check 阶段

| 返回 | 含义 |
| --- | --- |
| `pending` | 继续轮询 |
| `authorized` | **保存 token，停轮询** |
| `expired` | 提示用户重新授权 |

### 调 API 阶段

| HTTP 状态 / message | 处理 |
| --- | --- |
| 401 `auth.access_token_invalid` | OAT 已被撤销 / 过期 → 引导用户重新授权 |
| 401 `auth.user_info_invalid` | 用户被平台禁用 / 删除 → 联系平台 |
| 5xx | 服务端故障，退避重试 |

---

## 8. 安全考量

1. **OAT 是 user-equivalent 凭证**，泄漏等于他人能以该用户身份调任意 `UserAuth` 接口。集成端必须：
   - 只存在受保护位置（OS Keychain、加密文件等）
   - 不打日志、不带进 git、不放 build artifact
   - 用 HTTPS
2. **没有 refresh token**——不刷新概念，OAT 直接长期有效（最长 90 天空闲），过期后用户重新跑一次 device flow 即可
3. **session_id 是公开的**，不能作为身份凭证使用——它只在 device flow 5 分钟窗口内绑定 session 元信息
4. **client_secret 当前不强制**——但保留字段，未来上 Authorization Code Flow 时启用
5. **session 过期后立即清理 access_token 字段**——`ConsumeDesktopSession` 原子消费 + 清空 plaintext，service 层 `CleanupExpiredDesktopSessions` 兜底清陈旧记录
6. **重置密码自动撤销所有 OAT/PAT**——给被盗号用户兜底
7. 频繁失败的轮询不会泄露具体原因（拒/超时 / session 不存在统一返回 `expired`），防 session_id 探测

---

## 9. FAQ

**Q：为什么不需要 redirect_uri？**

A：Device Flow 设计就是给"无法接收回调的客户端"用的（CLI、电视盒子、桌面客户端）。集成端通过自己生成的 session_id 在轮询里"拉"到 token，不依赖浏览器回调。

**Q：用户能同时给同一个应用授权多次吗？**

A：可以。每次授权独立签发一把 OAT，写入 `user_access_tokens` 一行。用户能在"已授权应用"列表里看到多条同 `client_id` 的记录、独立撤销。

**Q：为什么有的 token 在我后台是 `amux_api_pat_`，授权来的是 `amux_api_oat_`？**

A：前缀只是审计区分：

- `amux_api_pat_` = 用户在 amux-api 后台手动创建的 Personal Access Token
- `amux_api_oat_` = OAuth Device Flow 由集成方触发签发的 OAuth Access Token

校验路径完全一致，权限范围也一致（当前都是 `full`）。

**Q：集成方能自己撤销自己签发的 OAT 吗？**

A：暂不支持。当前撤销路径仅限：用户自己 / 用户重置密码 / 管理员强制 / 空闲过期。如有需要可以加 `DELETE /api/oauth/device/revoke`，提供 OAT 自己撤销自己。

**Q：能拿 refresh token 吗？**

A：当前不发。OAT 默认 90 天空闲过期、无硬过期；正常使用的 token 会因为 `last_used_at` 不断推后而长期有效。如果你的应用一定要走 access+refresh 模式，告诉平台 root，可以加 Authorization Code Flow。

**Q：本地开发怎么调？**

A：

1. amux-api root 在本地实例的"系统设置 → OAuth 应用"创建一个测试 client_id（例如 `dev-cli`）
2. 跑你的集成代码，把 API_BASE 指向本地 `http://localhost:3000`
3. session_id / OAT 用 sqlite 客户端打开 `desktop_auth_sessions` / `user_access_tokens` 可以直接看到状态

---

## 10. 端点速查

### 集成方调用

| Method | Path | 鉴权 | 限流 |
| --- | --- | --- | --- |
| POST | `/api/oauth/device/authorize` | 公开 | CriticalRateLimit (20/20m) |
| GET | `/api/oauth/device/info` | 公开 | CriticalRateLimit |
| POST | `/api/oauth/device/confirm` | UserAuth（用户登录会话） | — |
| GET | `/api/oauth/device/check` | 公开 | OAuthPollRateLimit (120/60s) |

### 用户调用

| Method | Path | 用途 |
| --- | --- | --- |
| GET | `/api/user/access_tokens` | 列我所有 PAT + 已授权应用 |
| POST | `/api/user/access_tokens` | 创建一个 PAT |
| DELETE | `/api/user/access_tokens/:id` | 撤销 / 解除授权 |
| POST | `/api/user/access_tokens/:id/rotate` | 刷新 PAT（OAT 不支持） |

### 管理员调用（root 限定）

| Method | Path | 用途 |
| --- | --- | --- |
| GET / POST / PATCH / DELETE | `/api/admin/oauth/clients[/:id]` | OAuth 应用注册管理 |
| POST | `/api/admin/oauth/clients/:id/rotate` | 轮换该应用的 client_secret |

### 管理员调用（普通管理员）

| Method | Path | 用途 |
| --- | --- | --- |
| GET | `/api/admin/access_tokens` | 治理面板：按用户/状态/来源查所有 token |
| DELETE | `/api/admin/access_tokens/:id` | 强制撤销某个 token（治理滥用） |

> 注：管理 access_token 是治理性操作（撤销已签发的滥用 token），普通管理员可做。注册 OAuth 应用是签发新身份能力，必须 root 才能做。详见 `router/api-router.go` 注释。
