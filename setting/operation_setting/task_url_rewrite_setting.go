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

package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// TaskURLRewriteRule 定义一条 URL 前缀替换规则。
//   From  命中这个前缀的 URL 才替换（严格 HasPrefix）
//   To    替换为这个前缀
// 例：上游视频 URL 是 https://resource.zerocut.cn/zerocut/foo.mp4，管理员
// 在后台配 From="https://resource.zerocut.cn/zerocut/"、To="https://r.amux.ai/zc/"，
// 则落库和对外返回都会变成 https://r.amux.ai/zc/foo.mp4，不暴露上游域名。
type TaskURLRewriteRule struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TaskURLRewriteSetting 是"任务结果 URL 脱敏/反向代理"的管理员可编辑配置。
//
// 应用点：adapter 解析上游任务结果拿到 URL → 写入 task.PrivateData.ResultURL
// 之前统一过一次 Apply；因此：
//   - 操练场 /pg/video/generations 响应里的 metadata.url 已是脱敏后的
//   - 管理员任务日志的 result_url 字段也是脱敏后的
//   - /v1/videos/:task_id/content 代理同样指向脱敏 URL
// 不用在多处打补丁。
type TaskURLRewriteSetting struct {
	// 总开关——关掉后所有规则失效，用来紧急回退。
	Enabled bool                 `json:"enabled"`
	Rules   []TaskURLRewriteRule `json:"rules"`
}

// taskURLRewriteSetting 的默认值：功能默认关闭，规则列表为空。管理员
// 在后台显式打开并配置后才会生效——避免给现有部署带来任何隐式行为变化。
var taskURLRewriteSetting = TaskURLRewriteSetting{
	Enabled: false,
	Rules:   []TaskURLRewriteRule{},
}

func init() {
	config.GlobalConfig.Register("task_url_rewrite_setting", &taskURLRewriteSetting)
}

func GetTaskURLRewriteSetting() *TaskURLRewriteSetting {
	return &taskURLRewriteSetting
}

// ApplyTaskURLRewrite 对一个 URL 按当前已启用的规则做前缀替换。
//
// 语义：
//   - 空 URL / 非 http(s) 协议（如 data: / asset://）直接返回原值，不参与匹配
//   - 多条规则按"最长匹配优先"命中（避免前缀重叠时顺序依赖）
//   - 未命中任何规则返回原值
//   - 功能总开关关闭时直接返回原值
//
// 调用侧只需把"要落库/返回给用户的 URL"喂进来即可，幂等安全——重复调用
// 也不会二次替换（因为 To 前缀和 From 前缀形态不同）。
func ApplyTaskURLRewrite(url string) string {
	if url == "" {
		return url
	}
	if !taskURLRewriteSetting.Enabled {
		return url
	}
	// 只处理 http/https，跳过 data:/asset:// 这种内联/引用协议
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return url
	}
	var bestIdx = -1
	var bestLen = 0
	for i, rule := range taskURLRewriteSetting.Rules {
		if rule.From == "" {
			continue
		}
		if strings.HasPrefix(url, rule.From) && len(rule.From) > bestLen {
			bestIdx = i
			bestLen = len(rule.From)
		}
	}
	if bestIdx < 0 {
		return url
	}
	rule := taskURLRewriteSetting.Rules[bestIdx]
	return rule.To + strings.TrimPrefix(url, rule.From)
}
