# 用户分组与渠道分组系统重构方案

> 文档版本：v1.2
> 起草日期：2026-05-12
> 最后更新：2026-05-12（第三轮深度核查后补漏）
> 负责人：sejian@amux.chat
> 状态：方案已评审 / 未开工

---

## 1. 背景与目标

### 1.1 业务背景

平台基于 new-api 二开，定位为 AI API 中转 + 分销。目前用户分级使用 `default` / `vip` 两个用户分组，并通过累计充值阈值自动升级。同时为不同用户分组配置可见的"渠道分组"和差异化倍率。

### 1.2 现状的限制

随着分销业务展开，当前模型撑不住以下场景：

1. **精准的企业客户定价**：需要给某个企业客户单独定制一组渠道分组 + 倍率（如 enterprise_a 走 OpenAI 7 折），但不影响 default/vip 用户。
2. **限定用户的渠道访问**：需要给特定用户只开放某几个渠道分组的权限。
3. **私有渠道分组**：给管理员、朋友、内部账号开几个对外完全不可见的渠道分组。
4. **管理后台体验差**：当前所有分组配置都是嵌套 JSON map，靠后台 JSON 编辑器维护，新增/删除分组流程繁琐且无 FK 约束保护。

### 1.3 重构目标

- 将"分组"从分散在多个 JSON setting 中的字符串约定，**提升为正式的 DB 实体**。
- 支持**任意数量**的用户分组、渠道分组、用户分组 × 渠道分组矩阵。
- 提供后台 CRUD 界面替代 JSON 编辑器。
- 不破坏现有计费/鉴权链路，**保留** `user.group` / `channel.group` 字段形态（字符串 code），**最小化**主干代码改动面。
- 顺手修复盘点过程中发现的隐藏 bug（subscription UpgradeGroup 校验错位）。
- 为未来扩展（per-user override、按模型粒度覆盖、分销代理树、过期/审计等）保留口子，本期不实现。

---

## 2. 当前系统现状

### 2.1 字段层（保留不动）

| 字段 | 文件 | 类型 | 说明 |
|---|---|---|---|
| `User.Group` | `model/user.go:43` | `varchar(64)`，默认 `'default'` | 单值字符串，存用户所属用户分组 code |
| `Channel.Group` | `model/channel.go:38` | `varchar(64)` | 逗号分隔字符串，存渠道所属的渠道分组 code 列表 |
| `Token.Group` | `model/token.go:29` | `varchar(64)` | 单值字符串，存 token 选定的渠道分组 code |
| `Log.Group` | `model/log.go:36` | `varchar(64)`，带索引 | 历史日志的渠道分组 code 快照 |

### 2.2 配置层（4 个分散的 JSON 块，本次重构主要标的）

| 配置块 | 文件 | 类型 | 内容 |
|---|---|---|---|
| `userUsableGroups` | `setting/user_usable_group.go:10` | `map[string]string` | 用户分组列表 + 描述 |
| `groupRatioMap` | `setting/ratio_setting/group_ratio.go:12` | `map[string]float64` | 渠道分组默认倍率 |
| `groupGroupRatioMap` | `setting/ratio_setting/group_ratio.go:20` | `map[string]map[string]float64` | 用户分组 × 渠道分组倍率覆盖 |
| `groupSpecialUsableGroup` | `setting/ratio_setting/group_ratio.go:28` | `map[string]map[string]string` | 用户分组可见渠道分组（含 `+:` / `-:` mini-DSL）|
| `userUpgradeSetting` | `setting/operation_setting/user_upgrade_setting.go:23` | `[]UserUpgradeRule` | 自动升级规则 |
| `TopupGroupRatio` | `common/topup-ratio.go:15` | JSON map | 充值返利倍率（按用户分组） |

### 2.3 计费 / 鉴权 / 路由的核心路径

| 路径 | 文件:行号 | 当前做什么 |
|---|---|---|
| 预扣费 | `service/quota.go:109,120` | 读 `GetGroupRatio` + `GetGroupGroupRatio` 决定预扣额 |
| 结算 | `service/task_billing.go:283-287` | 读 `GetGroupGroupRatio` 或 fallback `GetGroupRatio`（在途任务走 `BillingContext` 快照） |
| Relay 路径倍率 | `relay/helper/price.go:38` `HandleGroupRatio` | relay 主路径统一计算 group ratio 入口 |
| Token 鉴权校验 | `middleware/auth.go:401` `ContainsGroupRatio` | 校验 tokenGroup 是否为合法渠道分组（每次 API 请求都过） |
| 挑渠道 | `middleware/distributor.go:86-162` | 用 `user.group` / token 上的 group 走 `group2model2channels` 缓存 |
| 自动升级 | `model/topup.go:609-658` | 充值后按 `UserUpgradeRule` 链式升级 |
| Subscription 升降级 | `model/subscription.go:170,263-264,419-510,558,677-678,897` | 订阅生效/过期时切换 `user.Group` |
| 模型广场 | `controller/pricing.go:14-87` | **硬编码** 返回 `default_group_ratio` 和 `vip_group_ratio` 两个字段 |

### 2.4 现状的核心问题

1. **分组是"幽灵实体"**：在数据库中没有正式表，仅以字符串 code 散落在 user/channel/setting 中。删一个 JSON key，user 表里的字符串变成孤儿引用。
2. **N × M 倍率矩阵是嵌套 JSON map**：不可索引、不可审计、不能做 SQL JOIN 报表、前端只能用 JSON editor 维护。
3. **用户分组与渠道分组共用同一个 string namespace**：两者本是不同概念却共享 key 空间。这导致 `controller/subscription.go:147-149` 的 UpgradeGroup（应该是用户分组 code）被用 `GetGroupRatioCopy()`（渠道分组 map）做校验 —— 当前能"工作"是因为 `default`/`vip` 在两边同名，一旦解耦就会暴露 bug。
4. **可见性靠 mini-DSL**：`+:append_1` / `-:remove_1` 这种约定难以维护，且无法表达"私有渠道分组"语义。
5. **自动升级规则与用户分组定义分离**：UpgradeRule 是独立 JSON 数组、靠字符串拼接引用 from_group/to_group，删一个分组时无法被发现引用关系。
6. **充值返利倍率（TopupGroupRatio）是第 3 套"按用户分组的配置"**，与 GroupRatio、UpgradeRule 平行存在。

---

## 3. 设计方案

### 3.1 设计原则

1. **字段形态不变，真相源换位**：`user.group` / `channel.group` 字段保持字符串 code 形态，背后新增 DB 表作为元数据真相源。绝大部分高频代码（distributor、ability、cache、auth 鉴权）零改动。
2. **软关联，不加 FK**：考虑到 user.group 字段是软迁移、channel.group 是逗号分隔字符串，使用 service 层"删除时检查"代替数据库 FK，三库兼容性更好。
3. **统一入口**：所有读取倍率/可见性的代码必须走 `service.GetUserGroupRatio` / `service.GetUserUsableGroups` 这两个函数（签名保持不变），底层切换数据源。
4. **缓存内存化**：分组配置是低频写、高频读，启动时全量加载到内存 + RWMutex，多实例靠现有 `SyncFrequency` 定时拉取保证一致性。
5. **本期不做**：per-user 单点覆盖、模型粒度的倍率、分销代理树、过期/审计字段。这些都是后向兼容的扩展，可以将来加列或加表。

### 3.2 表结构（共 3 张表）

```sql
-- 表 1：用户分组
user_groups
  id                    BIGINT       PRIMARY KEY,        -- GORM 自增
  code                  VARCHAR(64)  NOT NULL UNIQUE,    -- "default" / "vip" / "enterprise_a"
  name                  VARCHAR(128) NOT NULL,           -- 显示名
  description           TEXT,
  is_system             BOOLEAN      DEFAULT FALSE,      -- 系统内置不允许删除
  visibility            VARCHAR(16)  DEFAULT 'public',   -- public | internal
  upgrade_to_code       VARCHAR(64),                     -- 自动升级目标分组 code（软引用）
  upgrade_threshold     DECIMAL(20,4),                   -- 累计充值阈值
  auto_upgrade_enabled  BOOLEAN      DEFAULT FALSE,
  topup_ratio           DECIMAL(20,8) DEFAULT 1,         -- 替代 TopupGroupRatio map
  sort_order            INT          DEFAULT 0,
  created_at, updated_at, deleted_at

-- 表 2：渠道分组
channel_groups
  id              BIGINT       PRIMARY KEY,
  code            VARCHAR(64)  NOT NULL UNIQUE,
  name            VARCHAR(128) NOT NULL,
  description     TEXT,
  default_ratio   DECIMAL(20,8) DEFAULT 1,               -- 替代 GroupRatio map
  is_auto         BOOLEAN      DEFAULT FALSE,            -- 替代 setting/auto_group.go 的 autoGroups []string
  sort_order      INT          DEFAULT 0,
  created_at, updated_at, deleted_at

-- 表 3：用户分组 × 渠道分组（白名单 + 倍率覆盖）
user_group_channel_groups
  user_group_id    BIGINT NOT NULL,
  channel_group_id BIGINT NOT NULL,
  ratio            DECIMAL(20,8),                        -- NULL = 用 channel_groups.default_ratio
  created_at, updated_at,
  PRIMARY KEY (user_group_id, channel_group_id),
  INDEX idx_user_group (user_group_id),
  INDEX idx_channel_group (channel_group_id)
```

**三库兼容要点**：
- 主键 `BIGINT` 自增，由 GORM 适配 SQLite/MySQL/PostgreSQL 差异。
- 数值类型统一 `DECIMAL(20,8)`，三库通用。
- 不使用 `JSONB`、PG 专有操作符。
- 列名避开 reserved word（不用 `group`、`key`）。
- 字符串列指定明确 `VARCHAR(N)`，不依赖 dialect 默认。

### 3.3 字段语义

#### user_groups.visibility

- `public`：在模型广场倍率对比中公开展示，可作为注册默认分组、可作为升级目标
- `internal`：仅管理员可分配，模型广场完全不展示分组名称和倍率
- 用例：`default` / `vip` 设 `public`；`enterprise_a` / `friend` / `admin_internal` 设 `internal`

#### user_groups.upgrade_to_code + upgrade_threshold + auto_upgrade_enabled

合并自原 `UserUpgradeRule`。字段直接落在 user_groups 表上，删除某个 user_group 时可立即查出 `WHERE upgrade_to_code = ?` 反向引用，避免悬空指针。

#### user_groups.topup_ratio

合并自原 `TopupGroupRatio` 配置。同一张表承载用户分组的所有相关属性（升级链、充值返利、可见性）。

#### channel_groups.default_ratio

替代原 `GroupRatio` map。当 `user_group_channel_groups.ratio` 为 NULL 时使用此默认值。

#### channel_groups.is_auto

替代原 `setting/auto_group.go` 的 `autoGroups []string`。标记该渠道分组是否参与自动路由（即 `token.Group="auto"` 时系统会从所有 `is_auto=true` 的渠道分组中挑选）。

**为什么合并而不保留独立配置**：
- 避免出现"channel_group 被删了但 AutoGroups 还引用该 code"导致的运行时静默失败（`service/group.go:48` 走 `setting.GetAutoGroups()` 拿到不存在的 code，`channel_select.go` 找不到任何可用渠道但不会报清晰错误）
- 单一真相源，删除 channel_group 时自动连带清理
- 配置面板更直观（在 channel_group 编辑表单里勾选"参与自动路由"即可）

#### user_group_channel_groups（junction 表）

- **行存在 = 该用户分组可见此渠道分组**（白名单模型）
- `ratio` NULL = 用 `channel_groups.default_ratio`；非 NULL = 该 (user_group, channel_group) 对的特定倍率
- **不需要 `visible` 列**：行存在与否本身就是可见性信号
- **不需要 `channel_groups.visibility` 列**：私有渠道分组通过"junction 表里只对特定 user_group 挂行"实现，对其他用户分组天然不可见

### 3.4 解析逻辑

```text
查询：user(group=G) 能看到哪些渠道分组、各自倍率多少

可见性：
  对每个 channel_group C：
    junction(G, C) 存在 → 可见
    junction(G, C) 不存在 → 不可见

倍率：
  对可见的 (G, C)：
    ratio = junction.ratio ?? channel_groups[C].default_ratio
```

伪代码：

```go
// service/group_registry.go
func (r *Registry) GetVisibleChannelGroups(userGroupCode string) map[string]float64 {
    result := map[string]float64{}
    for _, cgCode := range r.channelGroupsVisibleTo(userGroupCode) {
        result[cgCode] = r.resolveRatio(userGroupCode, cgCode)
    }
    return result
}

func (r *Registry) GetRatio(userGroupCode, channelGroupCode string) float64 {
    if channelGroupCode == AutoGroupCode { return 1.0 }  // 特殊 code
    if ratio, ok := r.matrix[userGroupCode][channelGroupCode]; ok {
        return ratio
    }
    if cg, ok := r.channelGroups[channelGroupCode]; ok {
        return cg.DefaultRatio
    }
    return 1.0  // fallback：找不到时不让系统瘫痪
}
```

### 3.5 保留的特殊 code：`"auto"`

- `controller/user.go:220` 用户注册时默认 token 设 `Group: "auto"`
- `service/quota.go:116` / `middleware/distributor.go:121` 识别 `"auto"` 为"由系统从用户可用渠道分组里自动挑一个"
- **`"auto"` 不是某个具体的渠道分组**，是路由保留字
- **group_registry 必须显式跳过 `"auto"`**，不要尝试查表，避免误报"channel_group not found"

```go
const AutoGroupCode = "auto"

func (r *Registry) GetChannelGroup(code string) *ChannelGroup {
    if code == AutoGroupCode { return nil }
    return r.channelGroups[code]
}
```

---

## 4. 改动地图

### 4.1 完全不动的代码（高频路径，性能保住）

| 类别 | 文件:行号 | 不动原因 |
|---|---|---|
| Group 字段本身 | `model/user.go:43`, `model/channel.go:38`, `model/token.go:29`, `model/log.go:36` | 字符串 code 形态保留 |
| Distributor 主路径 | `middleware/distributor.go:86-162` | 仅消费字符串 |
| Channel 缓存 | `model/channel_cache.go:45` (`group2model2channels`) | 字符串 key 结构不变 |
| Ability 表 | `model/ability.go:41-46,148,220` | split channel.group 字符串逻辑不变 |
| Channel 选路 | `service/channel_select.go:83-118` (`CacheGetRandomSatisfiedChannel`) | 走 group2model2channels |
| Channel 满足判断 | `model/channel_satisfy.go` (`IsChannelEnabledForGroupModel`) | 字符串匹配 |
| Channel 搜索 SQL | `model/channel.go:292-338` | 三库 LIKE/CONCAT 不变 |
| Token 鉴权读 user.Group | `middleware/auth.go:158,395-396` | 字符串透传 |
| Context Keys | `constant/context_key.go:17,51,52` | 仅 key 名 |
| Token/User/Channel 缓存对象 | `model/token_cache.go`, `user_cache.go` | Group 字段已序列化进缓存 |
| Log 表 group 字段 + 查询 | `model/log.go:36,305-307,389-391,445-448` | 历史快照保留 |
| 模型健康度按 group 聚合 | `model/model_health.go:91` | SQL GROUP BY 用字段值，不受表结构影响 |
| 模型 enable_groups 反向引用 | `model/pricing.go:35,61`, `model/model_meta.go:50`, `controller/model_meta.go:283,332,379` | 从 channels 表 distinct 衍生 |
| Subscription UpgradeGroup/PrevUserGroup 字段 | `model/subscription.go:170,263-264,419-510,558,677-678,897` | 字段不动（仅改校验逻辑，见 5.1） |
| Token 创建/编辑链路 | `controller/token.go`, `EditTokenModal.jsx` | `/api/user/self/groups` 返回格式不变 |
| Playground 链路 | `controller/playground.go:67-68,112-113`, `SettingsPanel.jsx` | group 字符串透传 |
| OAuth 注册 | `oauth/*.go`, `controller/oauth.go` | 走 GORM `default:'default'` 标签 |
| 限流 | `middleware/model-rate-limit.go:181-183` | 只读 context |
| Task.Group / 任务结算 group 透传 | `model/task.go:199`, `service/task_billing.go:60`, `controller/channel-test.go:497` | 字符串透传 |
| 前端 enable_groups 消费 | `web/src/.../model-pricing/*` 约 10 处 jsx | 后端字段不变 |
| i18n 文案 | `web/src/i18n/locales/*.json` | 词条不变 |

### 4.2 核心改动点（按链路分组）

#### 4.2.1 倍率/可见性数据源（统一改成查 group_registry）

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `service/group.go:10-37` `GetUserUsableGroups` | 读 `userUsableGroups` + `GroupSpecialUsableGroup` + 解析 `+:` / `-:` DSL | 改为查 user_group_channel_groups JOIN channel_groups（带缓存）。**签名不变** |
| `service/group.go:45-54` `GetUserAutoGroup` | 走 `GetUserUsableGroups` | 透传，自动跟随 |
| `service/group.go:59-65` `GetUserGroupRatio` | 读 `GetGroupGroupRatio` / `GetGroupRatio` | 走 group_registry。**签名不变** |
| `service/quota.go:109` `GetGroupRatio` | 预扣费 | 改为 `service.GetUserGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)` 统一入口 |
| `service/quota.go:120` `GetGroupGroupRatio` | 预扣费 | 同上 |
| `service/task_billing.go:283-287` | 结算 fallback | 改为 `service.GetUserGroupRatio(...)` 统一入口 |
| `relay/helper/price.go:38` `HandleGroupRatio` | relay 主路径计算 group ratio | 内部从 ratio_setting 改为 service.GetUserGroupRatio |
| `controller/pricing.go:14-87` | 硬编码返回 default_group_ratio / vip_group_ratio | 重写为返回所有 visibility=public 用户分组的 tier_ratios |
| `controller/group.go:16,47,56` | 遍历 `GetGroupRatioCopy()` 拿渠道分组全集 | 改为 `groupregistry.ListChannelGroups()` |
| `controller/model_health.go:36-37` | 同上 | 同上 |

#### 4.2.2 鉴权高频路径

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `middleware/auth.go:401` `ContainsGroupRatio` | 校验 tokenGroup 是否合法 | 改为 `groupregistry.ChannelGroupExists(code)`，**必须命中内存缓存** |

#### 4.2.3 升级与订阅

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `model/topup.go:609-658` `CheckAndUpgradeUserGroup` | 遍历 `operation_setting.GetUpgradeRules()` 链式匹配 | 改为查 user_groups 表读 upgrade_to_code / upgrade_threshold / auto_upgrade_enabled，链式递归保留 |
| `controller/subscription.go:147-149,210-212` UpgradeGroup 校验 | 用 `GetGroupRatioCopy()`（渠道分组 map）校验 user_group code | **修 bug**：改为 `groupregistry.UserGroupExists(code)` |
| `model/subscription.go` 内的 UpgradeGroup / PrevUserGroup 字段使用 | 字符串透传写入 user.Group | 不动 |

#### 4.2.4 设置写入端（过渡期双写）

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `controller/option.go:199` `case "GroupRatio"` | 解析 JSON 校验 | 双写阶段：既写 setting JSON 又同步到 channel_groups 表；切换阶段后改为返回 410 Gone 并引导到新 API |
| `model/option.go:509,513,145,146` | OptionMap 同步 | 同上 |

#### 4.2.5 充值返利合并（TopupGroupRatio → user_groups.topup_ratio）

数据迁移到 `user_groups.topup_ratio` 字段后，**3 个调用方都要改**：

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `common/topup-ratio.go:15,32` `GetTopupGroupRatio` | 内存 map 查 user_group code → 充值倍率 | 改为 `groupregistry.GetTopupRatio(userGroupCode)`，函数签名保留为向下兼容 |
| `controller/topup.go:149` | 主充值流程读 GetTopupGroupRatio 算返利 | 透明跟随上面 |
| `controller/topup_stripe.go:448` | Stripe 充值流程 | 透明跟随上面 |
| `controller/topup_waffo.go:82` | Waffo 充值流程 | 透明跟随上面 |
| `model/option.go:116` OptionMap["TopupGroupRatio"] 初始化 | OptionMap 同步 | 双写过渡，4 周后删 |

#### 4.2.6 AutoGroups 合并到 channel_groups.is_auto

| 文件:行号 | 当前做什么 | 改动 |
|---|---|---|
| `setting/auto_group.go:7-35` `autoGroups []string` + `GetAutoGroups` | 独立 setting 维护参与自动路由的渠道分组 code 列表 | 数据合并进 `channel_groups.is_auto`，`GetAutoGroups()` 改为查 `WHERE is_auto = true` |
| `service/group.go:48` `GetUserAutoGroup` 走 `setting.GetAutoGroups()` | 遍历 AutoGroups 过滤用户可见 | 透明跟随：底层换数据源，逻辑不变 |
| `service/channel_select.go:90` 检查 AutoGroups 是否为空 | 决定是否走 auto 路由 | 透明跟随 |
| 旧 settings API `controller/option.go` 中的 `AutoGroups` 写入 | 写 JSON setting | 双写过渡 |

### 4.3 新建模块

```
service/group_registry.go         # 三表内存缓存 + 查询入口
controller/group_admin.go         # 后台 CRUD API
model/user_group.go               # UserGroup GORM model
model/channel_group.go            # ChannelGroup GORM model
model/user_group_channel_group.go # 矩阵 GORM model
model/migrate_groups.go           # 启动时回填脚本（幂等）
```

#### group_registry 接口设计

```go
package service

type GroupRegistry interface {
    // 查询
    ListUserGroups() []*UserGroup
    ListChannelGroups() []*ChannelGroup
    GetUserGroup(code string) *UserGroup
    GetChannelGroup(code string) *ChannelGroup
    UserGroupExists(code string) bool
    ChannelGroupExists(code string) bool
    GetRatio(userGroupCode, channelGroupCode string) float64
    GetVisibleChannelGroups(userGroupCode string) map[string]float64
    GetTopupRatio(userGroupCode string) float64

    // 写入（含失效）
    CreateUserGroup(...) error
    UpdateUserGroup(...) error
    DeleteUserGroup(code string) error  // 先做引用检查
    CreateChannelGroup(...) error
    UpdateChannelGroup(...) error
    DeleteChannelGroup(code string) error
    SetMatrix(userGroupCode, channelGroupCode string, ratio *float64) error
    UnsetMatrix(userGroupCode, channelGroupCode string) error

    // 缓存
    Reload() error  // 全量重载，由定时任务或写入后调用
}
```

### 4.4 新增 Admin REST API

```
GET    /api/admin/user-groups
POST   /api/admin/user-groups
PUT    /api/admin/user-groups/:id
DELETE /api/admin/user-groups/:id           # 校验：是否有 user.group 引用、是否被升级链引用

GET    /api/admin/channel-groups
POST   /api/admin/channel-groups
PUT    /api/admin/channel-groups/:id
DELETE /api/admin/channel-groups/:id        # 校验：是否被 channel.group 字符串引用

GET    /api/admin/user-groups/:id/channel-matrix    # 该用户分组下所有渠道分组的可见性 + 倍率
PUT    /api/admin/user-groups/:id/channel-matrix    # 批量更新矩阵
```

返回 / 提交格式保持 RESTful，前端使用现有的 fetch 工具链。

### 4.5 前端改动

#### 删除 / 重做

- `web/src/pages/Setting/Ratio/GroupRatioSettings.jsx` —— 原 JSON editor 三件套废弃，保留只读视图作为 debug 工具

#### 新建 3 个 CRUD 页

- `web/src/pages/Group/UserGroups.jsx` —— 用户分组管理（含 visibility、升级规则、topup_ratio）
- `web/src/pages/Group/ChannelGroups.jsx` —— 渠道分组管理
- `web/src/pages/Group/Matrix.jsx`（或嵌入用户分组详情）—— 矩阵编辑

#### 改下拉数据源（字段提交形态不变）

- `EditChannelModal.jsx` 渠道分组多选 → 新 API 拉列表，提交仍为逗号字符串
- `EditUserModal.jsx` 用户分组单选 → 新 API 拉列表，提交仍为 code 字符串
- `playground/SettingsPanel.jsx` 间接受影响（消费 /api/pricing）

#### 模型广场 pricing 改造

- `controller/pricing.go` 不再返回 `default_group_ratio` / `vip_group_ratio`
- 新增返回 `tier_ratios: map[user_group_code]map[channel_group_code]float64`（**仅 visibility=public 的用户分组**）
- 前端 `web/src/hooks/model-pricing/useModelPricingData.jsx:51-54,253-254` 消费 `tier_ratios`，倍率对比表从"固定 2 列"改为"动态 N 列"

---

## 5. 隐藏问题与顺手修复

### 5.1 Subscription UpgradeGroup 校验错位 BUG

**问题**：`controller/subscription.go:147-149,210-212` 用 `ratio_setting.GetGroupRatioCopy()`（渠道分组的倍率 map）校验 `req.Plan.UpgradeGroup`（应为用户分组 code）。

**当前能"工作"的原因**：default、vip 在两个 namespace 同名，校验碰巧通过。

**重构后修复**：用 `groupregistry.UserGroupExists(code)` 校验。

**历史脏数据处理**：发布前跑审计脚本

```sql
SELECT id, name, upgrade_group FROM subscription_plans
WHERE upgrade_group != ''
  AND upgrade_group NOT IN (SELECT code FROM user_groups);
```

由管理员人工清理。**不主动改数据库**。

### 5.2 TopupGroupRatio 合并

`common/topup-ratio.go` 的充值返利倍率本质是"按用户分组的属性"，独立维护违反单一真相源原则。合并为 `user_groups.topup_ratio` 字段。

### 5.3 GroupSpecialUsableGroup mini-DSL 废弃

原 `+:append_1` / `-:remove_1` / 无前缀直接添加 的语义糖在新模型中完全不需要：

- "添加可见" → junction 表里加一行
- "移除可见" → junction 表里不加这一行
- 不再需要"基础列表 + 增量"两段式表达

DSL 解析代码（`service/group.go:17-29`）随 GetUserUsableGroups 重写一并删除。

### 5.4 `GetUserUsableGroups` 的 baseline 语义清洁化（关键行为变化）

#### 当前代码的"namespace 冲突硬抗"

`service/group.go:10-37` 的 `GetUserUsableGroups` 函数实际语义是混乱的：

```go
groupsCopy := setting.GetUserUsableGroupsCopy()  // {"default":"...","vip":"..."}
// 上面这个 baseline map 装的是【用户分组 code】，
// 但函数返回值被 middleware/auth.go:396 当作【渠道分组 code 列表】用来校验 tokenGroup
```

也就是说，**因为 "default" / "vip" 在用户分组 namespace 和渠道分组 namespace 同名**，当前代码靠这种名字碰撞"硬抗"才能工作。

**已确认的越权路径**（不算 bug，是当前的预期行为，但容易被忽视）：

- `default` 用户分组的用户，可以创建 `token.Group="vip"` 的 token
- `middleware/auth.go:396` 检查通过：`GetUserUsableGroups("default")` 返回 `{"default":"...","vip":"...",...}` ← `"vip"` 在 baseline 里
- `middleware/auth.go:401` 检查通过：`ContainsGroupRatio("vip")` 返回 true（"vip" 是 GroupRatio 的合法 key）
- 走 `service.GetUserGroupRatio("default", "vip")` 计费 → 拿 `GroupRatio["vip"]` 默认倍率（不是免费，按 vip 渠道分组的费率收）

实质：当前系统中**所有用户都可以使用 `userUsableGroups` baseline 里列出的任何渠道分组**（按对应费率），不论自己处于哪个用户分组。

#### 重构的行为保留策略

重构后 `GetUserUsableGroups` 严格语义为"**该用户分组可见的渠道分组列表**"，由 junction 表精确控制。

为保证**线上行为不变、不破坏现有 token**，回填脚本必须显式做：

```
for each user_group_code in 老 userUsableGroups baseline:
    for each channel_group_code in 老 userUsableGroups baseline:
        # 每个用户分组都默认能看到所有 baseline 渠道分组
        if not exists junction(user_group_code, channel_group_code):
            INSERT INTO user_group_channel_groups (...)

# 然后再叠加 GroupSpecialUsableGroup 的 +:/-:/ 增减语义
```

这样回填后：
- default 用户分组的可见渠道分组 ⊇ {default, vip}（来自 baseline）
- vip 用户分组的可见渠道分组 ⊇ {default, vip} + special 添加 - special 移除

**现有所有 token 的 token.Group 校验路径不受影响**。

#### 重构后管理员的新能力

新 CRUD 矩阵编辑页里，管理员**可以主动收紧权限**：把 default 用户分组对 vip 渠道分组的 junction 行删掉，达成"严格的分级隔离"。这是原来 setting JSON 做不到的（DSL 只能减自定义分组、不能减 baseline）。

但**回填阶段保持现状**，不主动收紧，避免发布时影响线上用户。

### 5.5 Subscription PrevUserGroup 降级兜底

#### 当前缺陷

`model/subscription.go:899` 订阅到期时：

```go
if upgradeGroup == "" || prevGroup == "" {
    return  // 直接返回，不做任何降级
}
```

如果 `PrevUserGroup` 为空，或者指向的用户分组被管理员删了（软关联，删除后字符串还在但查不到对应用户分组），用户**永远卡在升级后的分组**，不会自动降回去。

#### 重构时修复

`group_registry.GetUserGroup(code)` 找不到时返回 nil，subscription 降级逻辑改为：

```go
if prevGroup == "" || !groupregistry.UserGroupExists(prevGroup) {
    prevGroup = "default"  // 兜底到 default
    common.SysLog(fmt.Sprintf("订阅到期，PrevUserGroup=%q 不存在，降级到 default", sub.PrevUserGroup))
}
user.Group = prevGroup
```

同时，**删除 user_group 时反向引用检查**要扩展到 subscription 表：

```sql
SELECT COUNT(*) FROM user_subscriptions
WHERE upgrade_group = ? OR prev_user_group = ?
```

如有引用，拒绝删除并提示管理员先处理（同 5.1 节 UpgradeGroup 校验风格）。

### 5.6 新建分组的 code 格式校验

#### 当前缺陷

`user.group` / `channel.group` / `token.group` 字段在创建/更新时**没有任何格式校验**：

- 可以创建 `code = "  vip  "`（带空格），路由时不匹配
- 可以创建 `code = "中文"` 或带特殊字符 `code = "vip/test"`
- 数据库定义是 `varchar(64)` 但应用层无长度限制（依赖 DB 截断或报错）

整个项目除了 `model/subscription.go:439,897` 在订阅降级时调了 `strings.TrimSpace()`，没有任何统一的规范化逻辑。

#### 重构时引入（仅对新建分组）

新 admin REST API 创建 user_group / channel_group 时，对 `code` 字段强制校验：

```go
var codePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

func validateGroupCode(code string) error {
    if len(code) == 0 || len(code) > 64 {
        return errors.New("code 长度必须 1-64 字符")
    }
    if !codePattern.MatchString(code) {
        return errors.New("code 只能包含小写字母、数字、下划线、连字符")
    }
    if code == "auto" {
        return errors.New("auto 是保留字，不能用作 code")
    }
    return nil
}
```

**不对已有的旧 code 强制改造**：回填脚本读到的脏数据（如带空格、大写、中文）原样写入新表，避免破坏线上引用。可以在后台 UI 上对这些脏 code 加红色警告标记，让管理员有意识地清理。

### 5.7 Token.Group 后端无校验（已知问题，本次不修）

`controller/token.go:222,300` 创建/更新 token 时**不校验 token.Group 是否在 user 的可见渠道分组列表中**。API 用户可以绕过前端下拉，把 token.Group 设为任意字符串。

防护完全依赖运行时 `middleware/auth.go:396-401` 的两道校验。这两道校验在重构后改为查新表，**防护能力一致**，没有变化。

**本次重构不修这个 token 创建时的校验缺失**：
- 修了可能破坏现存 API 用户的脚本（他们可能依赖能任意填 group 然后等运行时拦截的行为）
- 风险高、收益小，运行时拦截已足够防止真正的越权
- 留作未来安全加固独立 PR

仅在文档中记录此现状，让后续维护者知晓。

---

## 6. 数据迁移与回填

### 6.1 Migration

在 `model/main.go` 的 AutoMigrate 链尾追加：

```go
err = DB.AutoMigrate(
    &UserGroup{},
    &ChannelGroup{},
    &UserGroupChannelGroup{},
)
```

三库兼容由 GORM 自动适配 BIGINT 自增。

### 6.2 回填脚本（幂等）

启动时调用 `model.BackfillGroupsFromSetting()`：

```go
func BackfillGroupsFromSetting() error {
    if alreadyBackfilled() {
        return nil  // 通过 setting 标志位判断，避免每次重启都回填
    }

    DB.Transaction(func(tx *gorm.DB) error {
        // Step 1: 回填 user_groups
        for code, desc := range setting.GetUserUsableGroupsCopy() {
            tx.Create(&UserGroup{
                Code:        code,
                Name:        desc,
                IsSystem:    code == "default",
                Visibility:  defaultVisibilityFor(code),    // default/vip → public，其他 → public（保守）
                TopupRatio:  topupRatioFor(code),           // 从 common.TopupGroupRatio 读
            })
        }

        // Step 2: 回填 channel_groups（来源是并集）
        // 数据源：当前生产环境的实际配置（option 表中的 GroupRatio / GroupGroupRatio /
        // GroupSpecialUsableGroup），不参考源码中的 default 示例值。
        codes := collectAllChannelGroupCodes()
        // = keys(GroupRatio map)
        // ∪ split tokens from channels.group
        // ∪ values in GroupGroupRatio inner maps
        // ∪ codes referenced in GroupSpecialUsableGroup（去 `+:` / `-:` 前缀）
        // ∪ "default" 兜底
        // 跳过 "auto" 特殊保留字
        for _, code := range codes {
            ratio := getGroupRatioOrDefault(code, 1.0)
            tx.Create(&ChannelGroup{
                Code:         code,
                Name:         code,
                DefaultRatio: ratio,
            })
        }

        // Step 3: 回填 user_group_channel_groups（最复杂，必须保留 baseline 语义）
        //
        // 3a. baseline 全连接：每个 user_group 默认可见 setting.GetUserUsableGroupsCopy()
        //     列出的所有 code（旧代码把这套 map 当渠道分组用，靠 namespace 冲突硬抗，
        //     见 5.4 节）。这一步**必须做**，否则现有 token 大批失效。
        //
        // 3b. baseline 之外的 GroupRatio 中存在但不在 userUsableGroups 中的 channel_group：
        //     这部分原本对所有 user_group 不可见（除非 GroupSpecialUsableGroup 显式加），
        //     回填时不做全连接。
        //
        // 3c. 应用 GroupSpecialUsableGroup 的 +:/-:/ DSL：
        //     - 无前缀 / +: → 在矩阵中加 (user_group, channel_group) 行
        //     - -: → 从矩阵中删除该 (user_group, channel_group) 行（覆盖 3a 的全连接）
        //
        // 3d. 倍率覆盖：GroupGroupRatio[user_group][channel_group] 写入 junction.ratio
        baselineCodes := keysOf(setting.GetUserUsableGroupsCopy())  // 老 baseline 集合

        for _, userGroup := range tx.AllUserGroups() {
            visibleSet := map[string]struct{}{}

            // 3a: baseline 全连接
            for _, code := range baselineCodes {
                visibleSet[code] = struct{}{}
            }

            // 3c: 叠加 GroupSpecialUsableGroup
            for specialEntry := range ratio_setting.GetGroupRatioSetting().
                GroupSpecialUsableGroup.GetOrDefault(userGroup.Code, nil) {

                if strings.HasPrefix(specialEntry, "-:") {
                    code := strings.TrimPrefix(specialEntry, "-:")
                    delete(visibleSet, code)
                } else {
                    code := strings.TrimPrefix(specialEntry, "+:")
                    visibleSet[code] = struct{}{}
                }
            }

            // 写入 junction，附加倍率
            for cgCode := range visibleSet {
                ratioOverride := lookupGroupGroupRatio(userGroup.Code, cgCode)
                cgID, ok := lookupChannelGroupID(cgCode)
                if !ok {
                    common.SysLog(fmt.Sprintf("WARN: 回填跳过未知 channel_group code=%s", cgCode))
                    continue
                }
                tx.Create(&UserGroupChannelGroup{
                    UserGroupID:    userGroup.ID,
                    ChannelGroupID: cgID,
                    Ratio:          ratioOverride,  // 可能为 nil
                })
            }
        }

        // Step 4: 升级规则从 UserUpgradeSetting 写回 user_groups 表
        for _, rule := range operation_setting.GetUpgradeRules() {
            tx.Model(&UserGroup{}).Where("code = ?", rule.FromGroup).Updates(map[string]any{
                "upgrade_to_code":      rule.ToGroup,
                "upgrade_threshold":    rule.Threshold,
                "auto_upgrade_enabled": operation_setting.IsAutoUpgradeEnabled(),
            })
        }

        // Step 5: AutoGroups 写入 channel_groups.is_auto
        for _, code := range setting.GetAutoGroups() {
            tx.Model(&ChannelGroup{}).Where("code = ?", code).Update("is_auto", true)
        }

        // Step 6: TopupGroupRatio 写入 user_groups.topup_ratio
        for code, ratio := range common.GetTopupGroupRatioCopy() {
            tx.Model(&UserGroup{}).Where("code = ?", code).Update("topup_ratio", ratio)
        }

        return nil
    })

    markBackfilled()
    return nil
}
```

### 6.3 兜底机制

1. **default 用户分组必须存在**：若 user_groups 表为空（全新部署或被人为清空），migration 后立即插入 `{code: "default", name: "默认分组", is_system: true, visibility: "public", topup_ratio: 1}`。
2. **default 渠道分组必须存在**：同上，插入 `{code: "default", name: "默认", default_ratio: 1}`。
3. **runtime fallback**：`group_registry.GetRatio(userGroupCode, channelGroupCode)` 若两个 code 都查不到，返回 1.0 并打 ERROR log，不让系统瘫痪。

### 6.4 双写机制（过渡期）

发布后 4 周内，旧 settings API 与新 admin API 双写同步：

- 调用旧 API 写 `GroupRatio` → 同步写 `channel_groups.default_ratio`
- 调用旧 API 写 `GroupGroupRatio` → 同步写 `user_group_channel_groups.ratio`
- 调用旧 API 写 `UserUsableGroup` → 同步写 `user_groups`
- 调用新 admin API 写新表 → 反向同步回 setting JSON

目的：rollback 时（开启 `USE_LEGACY_GROUP_CONFIG=true`）数据仍然正确。4 周稳定后下一版本删除双写代码。

---

## 7. 缓存策略

### 7.1 单实例

- 进程启动时**同步**加载 3 张表全量数据到 `service/group_registry` 的内存结构
- 加载失败 panic（拒绝启动），避免冷数据放流量
- 写操作（admin CRUD）后立即失效 + 重载

### 7.2 多实例

- 沿用现有 `channel_cache` 的 `SyncFrequency` 定时拉取模式（默认 60s）
- 实例 A 写完后，实例 B 最多 60s 后同步到
- **不引入 Redis Pub/Sub**：多一套机制多一个故障点，60s 不一致窗口对管理员操作可接受
- 多实例场景由运维通过 env 配置 `SYNC_FREQUENCY` 调整

### 7.3 性能要求

- `groupregistry.GetRatio()` / `GetUserGroupRatio()`：纯内存读，目标 < 100ns
- `groupregistry.ChannelGroupExists()`：每次 API 请求都过，目标 < 100ns
- 启动加载 3 张表全量：目标 < 2s

---

## 8. 发布策略（分阶段灰度）

### 阶段 0：发布前准备（2-3 天）

- [ ] 生产数据库导出快照，副本库跑完整 migration + 回填脚本
- [ ] 回填 diff 报告：每个用户分组在回填前 vs 回填后的"可见渠道列表"做对比，人工 review
- [ ] **Token baseline 冲突审计**（关键，对应 5.4 节）：
  ```sql
  -- 找出所有"用户分组与 token.group 不同"的 token，检查回填后是否依然可用
  SELECT t.id, t.name, u.id AS user_id, u.group AS user_group, t.group AS token_group
  FROM tokens t JOIN users u ON t.user_id = u.id
  WHERE t.group != u.group AND t.group != 'auto' AND t.status = 1;
  ```
  对每条结果，确认其 `(user_group, token_group)` 在回填后的 junction 表中存在；不存在的 token 提前通知用户。
- [ ] **Subscription UpgradeGroup 脏数据审计**（对应 5.1 节 bug）：
  ```sql
  SELECT id, name, upgrade_group FROM subscription_plans
  WHERE upgrade_group != '' AND upgrade_group NOT IN (
    SELECT DISTINCT `group` FROM users  -- 老 namespace 借用
  );
  ```
- [ ] **Subscription PrevUserGroup 脏数据审计**（对应 5.5 节）：
  ```sql
  SELECT id, user_id, prev_user_group FROM user_subscriptions
  WHERE prev_user_group != '' AND prev_user_group NOT IN (
    SELECT code FROM user_groups  -- 回填后查
  );
  ```
- [ ] 预扣费 vs 结算一致性测试：构造各种 (user_group, channel_group) 组合，验证两路径返回完全相同倍率
- [ ] 冷启动性能测试基线
- [ ] AutoGroups 回填后的自动路由验证：模拟 `token.group="auto"` 请求，确认走的渠道分组集合与重构前完全一致

### 阶段 1：灰度发布（带 feature flag，1-3 天）

- 加 env `USE_LEGACY_GROUP_CONFIG=true`（默认）：所有读取走旧路径，但**双写新表**
- 观察 1-3 天：新表数据正确性、双写无错误日志
- 期间随时可回滚（直接走旧代码）

### 阶段 2：切换读路径（滚动发布）

- 改 env 为 `USE_LEGACY_GROUP_CONFIG=false`，**滚动发布**
- 一个实例一个实例切，每切完观察 5-10 分钟错误率/计费日志
- 出问题立即把这个实例切回 `true` 再排查
- 所有实例切完后观察 24h

### 阶段 3：UI 切换

- 开放新 admin CRUD 页给管理员
- 旧 setting JSON editor 页保留只读模式，标记"已废弃"
- 管理员开始用新 UI 操作

### 阶段 4：清理（4 周后下一版本）

- 删除 `USE_LEGACY_GROUP_CONFIG` env
- 删除双写代码
- 删除旧 setting JSON editor 页
- 删除 `setting/user_usable_group.go`、`group_ratio.go` 中废弃函数
- 删除 `setting/operation_setting/user_upgrade_setting.go`（升级规则已并入 user_groups 表）
- 保留 setting JSON 的回填能力作为 disaster recovery 工具（脱离主代码路径）

---

## 9. 回滚预案

| 阶段 | 回滚动作 | RTO |
|---|---|---|
| 阶段 0（未发布）| 不发布 | 0 |
| 阶段 1（灰度，flag 开着）| 改 env=true 重启，或回滚镜像 | < 5 min |
| 阶段 2（已切新读路径）| 改 env=true 重启 —— 双写仍生效，旧 setting JSON 数据新鲜 | < 10 min |
| 阶段 3（UI 切换后）| 同上，管理员回旧 UI 也行 | < 10 min |
| 阶段 4（清理后）| **无法平滑回滚**，从备份恢复 + 反向迁移 | 数小时 |

阶段 1-3 都能 5-10 分钟回滚。建议阶段 3 稳定 2-4 周才进阶段 4。

---

## 10. 发布前 Checklist

```
□ migration 在 SQLite / MySQL / PostgreSQL 三库都跑通
□ 回填脚本幂等性测试（跑 2 次结果一致）
□ 回填脚本在生产数据副本上跑过，diff 已 review
□ 单元测试覆盖：
  □ +: / -: DSL 所有组合
  □ 特殊 code "auto" 处理
  □ 空配置兜底（user_groups / channel_groups 表为空）
  □ 循环升级链（default → vip → svip）
  □ baseline 全连接策略（5.4 节）：default 用户的 token.group=vip 在回填后仍能通过鉴权
  □ Subscription PrevUserGroup 兜底（5.5 节）：PrevUserGroup 不存在时降到 default
  □ Code 格式校验（5.6 节）：拒绝带空格 / 大写 / 特殊字符 / "auto" 保留字 / 超过 64 字符
□ 集成测试：
  □ 预扣费 vs 结算倍率完全一致
  □ 在途任务（BillingContext 已快照）结算不受影响
  □ Subscription 升降级链路
  □ Token 鉴权（包括 group="auto" 的 token）
  □ 模型广场只展示 visibility=public 的用户分组
  □ 充值返利倍率（TopupGroupRatio 合并后）3 个 controller 路径返回值一致
  □ AutoGroups 合并后 token.group="auto" 路由结果与重构前完全一致
□ 性能测试：
  □ 进程启动到 ready 时间增加 < 2s
  □ 单次 API 请求加入 group_registry 缓存查询后 P99 延迟增加 < 1ms
□ 双写机制验证：env=true 时新表与旧 setting JSON 始终一致
□ 多实例缓存：60s 内同步一致（如有多副本部署）
□ Subscription UpgradeGroup 脏数据已清理（5.1 节）
□ Subscription PrevUserGroup 脏数据已清理（5.5 节）
□ Token baseline 冲突审计已完成（5.4 节）—— 受影响的 token 列表已通知用户或处理完毕
□ user_groups 表必有 code='default' 行
□ channel_groups 表必有 code='default' 行
□ channel_groups 表至少有 1 行 is_auto=true（否则 token.group="auto" 完全失效）
□ group_registry 找不到 user.group 时有 fallback + ERROR log
□ Subscription 降级 PrevUserGroup 不存在时 fallback 到 default + WARN log
□ 前端 SPA bundle hash 已变更，CDN 缓存可正常失效
□ CLAUDE.md、CHANGELOG、运维 runbook 已更新
```

---

## 11. 工作量评估

| 模块 | 工作量 | 备注 |
|---|---|---|
| schema + GORM model + migration + 回填 | 2 天 | 含 `+:` / `-:` DSL 解析、baseline 全连接、topup_ratio / AutoGroups 合并、特殊 code 处理 |
| group_registry 缓存层 | 1 天 | 高频路径性能要求高，必须做好失效机制 |
| 6 处计费/鉴权链路改造 | 2 天 | quota.go / task_billing.go / HandleGroupRatio / auth.go ContainsGroupRatio / pricing.go / model_health.go |
| service/group.go + topup.go + group_admin.go 重写 | 1.5 天 | |
| Subscription bug 修复（UpgradeGroup 校验 + PrevUserGroup 兜底）| 0.5 天 | 5.1 + 5.5 一起 |
| TopupGroupRatio 3 个 controller 调用方切换 | 0.5 天 | 5.x |
| AutoGroups 合并：channel_groups.is_auto + 2 个读取点切换 + 写入端双写 | 0.5 天 | |
| Code 格式校验（仅新建分组） | 0.3 天 | regex + 单元测试 |
| option.go 写入端双写 | 0.5 天 | |
| 新增 admin REST API | 1 天 | 含删除时引用检查（含 5.5 节 subscription 反向引用） |
| 前端 CRUD 3 页 + 矩阵编辑 | 2-3 天 | 矩阵编辑稍复杂 |
| 前端 pricing 改造 | 0.5 天 | tier_ratios 动态列 |
| 联调 + 回归测试 + token baseline 冲突审计脚本 | 2-3 天 | 重点测：预扣费/结算一致性、在途任务、自动升级、订阅升降级、token 鉴权、auto 路由、token baseline 兼容 |
| **总计** | **14-17 天** | 一个人三周左右 |

---

## 12. 未来扩展空间（本期不做）

| 扩展 | 数据库变更 | 触发场景 |
|---|---|---|
| **per-user 单点覆盖** | 新增 `user_channel_group_override` 表 | 单个用户需要特殊折扣或临时屏蔽某分组 |
| **按模型粒度的倍率覆盖** | 在 `user_group_channel_groups` 加 `model VARCHAR(64)` 字段，主键扩展为三元组 | 某分组里仅特定模型打折 |
| **临时活动 / 折扣过期** | 在 `user_group_channel_groups` 加 `expires_at TIMESTAMP` | 限时活动 |
| **分销代理树** | 在 `user_groups` 加 `parent_code` 字段，自指引用 | 多级分销 |
| **审计日志** | 新增 `group_audit_log` 表 | 谁在什么时候改了什么倍率 |
| **批量操作 API** | 新增专用 endpoint | 一次性给 100 个企业客户更新倍率 |

所有扩展都是后向兼容的"加列 / 加表"操作，不影响已设计的 3 张表。

---

## 13. 已决问题（2026-05-12 确认）

### 13.1 回填数据源

**决策**：回填脚本完全基于**当前生产环境的实际配置**（运行时从 `option` 表加载到内存的 `userUsableGroups` / `groupRatioMap` / `groupGroupRatioMap` / `groupSpecialUsableGroup`）。

不参考源码中的 `defaultGroupRatio` / `defaultGroupGroupRatio` / `defaultGroupSpecialUsableGroup` 等示例占位值——生产环境已运行良久，开源项目自带的演示占位符（`edit_this`、`append_1`、`vip_special_group_1` 等）早已被覆盖为真实业务值。

**回填脚本不需要任何"占位符过滤名单"逻辑**，纯做配置转换。

### 13.2 管理员账号的初始分组

**决策**：回填脚本对管理员账号**不做任何特殊处理**。

- 不自动创建 `admin_internal` 用户分组
- 不迁移现有 admin 用户的 `user.group` 字段
- 由管理员在重构上线后通过新 CRUD 页**手动创建** `admin_internal`（visibility=internal）并自行决定何时切换自己的分组

理由：迁移期"无意外"原则。管理员对自己什么时候被切到内部分组应有完全控制权。

### 13.3 删除 user_group 的流程（有用户引用时）

**决策**：可选目标的批量迁移。

具体行为：
- 删除按钮点击后，后端先 `SELECT COUNT(*) FROM users WHERE group = ?`
- 若 count > 0，前端弹出确认窗：
  - 显示"还有 N 个用户在此分组下"
  - 提供下拉菜单"将这 N 个用户批量迁移到：[选择目标用户分组]"，下拉项为所有非待删除的 user_groups
  - 二次确认后，事务内执行 `UPDATE users SET group = ? WHERE group = ?` + 删除 user_group 行
- 若 count = 0，直接确认删除

同时**反向引用检查**也要做：删除前 `SELECT COUNT(*) FROM user_groups WHERE upgrade_to_code = ?` —— 若有分组的升级目标指向待删除分组，要求管理员先改升级配置才能删。

### 13.4 channel_group 删除流程

**决策**（顺势对齐 user_group 的处理风格）：

- 删除前查 `SELECT COUNT(*) FROM channels WHERE group LIKE %code%`（注意三库兼容的 LIKE 写法）
- 若有引用，弹窗提示"还有 N 个渠道引用此分组"，提供：
  - 选项 A：从这 N 个渠道的 group 字符串中移除该 code（保留其他 group 引用不变）
  - 选项 B：拒绝删除，让管理员手动处理
- 推荐默认走选项 A，但需要二次强确认

### 13.5 CLAUDE.md 增加约束规则

**决策**：在项目根 `CLAUDE.md` 新增一条 Rule（建议作为 Rule 8）：

```markdown
### Rule 8: 分组倍率/可见性查询入口统一

所有读取"用户分组×渠道分组"倍率或可见性的代码，必须走以下统一入口：

- `service.GetUserGroupRatio(userGroupCode, channelGroupCode string) float64`
- `service.GetUserUsableGroups(userGroupCode string) map[string]string`
- `service.GetUserAutoGroup(userGroupCode string) []string`

禁止直接调用以下旧函数（已废弃，会在后续版本删除）：
- `ratio_setting.GetGroupRatio`
- `ratio_setting.GetGroupGroupRatio`
- `ratio_setting.ContainsGroupRatio`
- `ratio_setting.GetGroupRatioCopy`
- `setting.GetUserUsableGroupsCopy`

这些旧函数在过渡期会被保留并打 `// Deprecated` 注释，IDE 会黄字提醒。新代码一律走 service 层入口，由 service 层统一从 `group_registry` 内存缓存读取，保证：
1. 预扣费与结算使用相同数据源（避免计费偏差）
2. 数据源未来切换（如改为 Redis 缓存或外部配置中心）只需改一处
3. 单实例/多实例缓存一致性由 service 层兜底
```

附加措施：在 `ratio_setting` 包内所有相关函数顶部加 `// Deprecated: use service.GetUserGroupRatio instead`，让 IDE 在调用处显示黄字警告。

### 13.6 i18n 文案补全（实施时 TODO，非决策点）

新 CRUD 页的中英文翻译词条，实施阶段在前端 i18n 文件中补充：
- `web/src/i18n/locales/zh-CN.json`（基准）
- `web/src/i18n/locales/zh-TW.json`
- `web/src/i18n/locales/en.json`
- `web/src/i18n/locales/fr.json`
- `web/src/i18n/locales/ru.json`
- `web/src/i18n/locales/ja.json`
- `web/src/i18n/locales/vi.json`

按现有项目约定（key 是中文源串）一次性添加，使用 `bun run i18n:extract` / `i18n:sync` / `i18n:lint` 工具链辅助。

---

## 附录 A：审计过程中确认的代码位置一览

按文件分组的所有受影响 / 已确认不受影响的位置见正文第 4 节。每一行都已通过实际代码扫描验证，未凭印象。

## 附录 B：本方案演进历程

- v0.1：初稿提出 4 张表（含 user_channel_group_override），三层叠加设计
- v0.2：经评审认为过度设计，简化为 1 张 override 表 + 现有 user_group 体系扩展
- v0.3：评审者指出"地基薄弱"问题，决定升级为 4 张表（user_groups / channel_groups / junction / override）
- v0.4：评审者指出 override 与 junction `visible` 字段冗余，移除该字段
- v0.5：经穷尽审计补充 6 处遗漏的核心改动点（quota.go、HandleGroupRatio、auth.go、option.go、subscription.go bug、TopupGroupRatio 合并）
- v1.0：去掉 override 表（本期不做），加入 user_groups.visibility / topup_ratio 字段，完整发布策略与回滚预案
- v1.1：5 个未决问题全部敲定并落入 13 节；明确回填脚本不需要占位符过滤逻辑（生产数据已清理）；CLAUDE.md 新增 Rule 8 约束统一查询入口；删除流程改为"可选目标的批量迁移"
- **v1.2（当前）**：第三轮深度核查后补漏：
  - 5.4 节：明确 `GetUserUsableGroups` 当前的 namespace 冲突"硬抗"行为，回填脚本增加 baseline 全连接策略，保证现有 token 不破坏
  - 5.5 节：新增 Subscription `PrevUserGroup` 兜底逻辑（之前 PrevUserGroup 不存在时用户会卡死在升级分组）
  - 5.6 节：新增"新建分组的 code 格式校验"
  - 5.7 节：记录 Token.Group 后端无校验的已知问题，本次不修
  - 3.2 节：`channel_groups` 增加 `is_auto BOOLEAN` 列，合并 `setting/auto_group.go` 的 autoGroups
  - 4.2.5 节：明确 TopupGroupRatio 的 3 个调用方（topup.go / topup_stripe.go / topup_waffo.go）都要切换
  - 4.2.6 节：新增 AutoGroups 合并的改动点
  - 8 阶段 0：新增 token baseline 冲突审计、subscription PrevUserGroup 脏数据审计
  - 10 节 Checklist：新增 8 项对应新改动
  - 工作量从 12-14 天调整为 14-17 天
