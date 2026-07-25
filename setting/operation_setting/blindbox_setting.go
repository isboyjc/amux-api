package operation_setting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// 盲盒抽奖功能配置。
//
// 玩法：用户在「一个开奖周期」内累计消耗达到门槛（默认 $1）即获得抽奖资格，
// 消耗越多中奖概率越大。次日开奖时点由定时任务统一结算，按消耗加权无放回
// 抽出中奖者；中奖后奖金不会自动到账，用户需在下一次开奖前主动「开盲盒」领取，
// 超时未开则作废。
//
// 一个周期 = [本次开奖时点, 次日同一开奖时点)。DrawTime 可配置（HH:MM，服务器时区）。
//
// 与签到共用后台运营设置页；本模块与签到完全独立，可单独开关，关闭后所有相关
// 定时任务、接口、前端入口全部休眠，不影响任何其他功能。

const (
	// MaxBlindBoxPrizes 固定奖池最多可配置的奖项数量。
	MaxBlindBoxPrizes = 10

	// BlindBoxPoolModeFixed 固定奖池：后台逐个设定奖项及其中奖额度。
	BlindBoxPoolModeFixed = "fixed"
	// BlindBoxPoolModePercent 百分比奖池：奖池 = 参与者总消耗 × 比例，均分为 N 个等额奖项。
	BlindBoxPoolModePercent = "percent"
)

// BlindBoxPrize 固定奖池中的单个奖项。
type BlindBoxPrize struct {
	Name  string `json:"name"`  // 奖项名称（展示用）
	Quota int    `json:"quota"` // 中奖额度（quota 单位，500000 ≈ $1）
}

// BlindBoxSetting 盲盒抽奖配置。切片/结构体字段由 config 框架自动 JSON 序列化落库。
type BlindBoxSetting struct {
	Enabled        bool            `json:"enabled"`         // 功能总开关，默认关闭
	DrawTime       string          `json:"draw_time"`       // 每日开奖时点 "HH:MM"（服务器时区）
	ThresholdQuota int             `json:"threshold_quota"` // 参与门槛（quota），达到即有资格
	PoolMode       string          `json:"pool_mode"`       // fixed | percent
	Prizes         []BlindBoxPrize `json:"prizes"`          // 固定模式奖池，最多 MaxBlindBoxPrizes 个
	PercentRate    float64         `json:"percent_rate"`    // 百分比模式：奖池占总消耗比例，0~1
	PercentCount   int             `json:"percent_count"`   // 百分比模式：奖项数量，1~MaxBlindBoxPrizes
}

var blindBoxSetting = BlindBoxSetting{
	Enabled:        false,
	DrawTime:       "00:00",
	ThresholdQuota: 500000, // 1 * QuotaPerUnit，init 中按实际 QuotaPerUnit 校正
	PoolMode:       BlindBoxPoolModePercent,
	Prizes:         []BlindBoxPrize{},
	PercentRate:    0.1,
	PercentCount:   3,
}

func init() {
	// 默认门槛跟随实际的 QuotaPerUnit（$1）。DB 中已有配置会在 LoadFromDB 时覆盖。
	blindBoxSetting.ThresholdQuota = int(common.QuotaPerUnit)
	config.GlobalConfig.Register("blindbox_setting", &blindBoxSetting)
}

// GetBlindBoxSetting 返回配置实例（指针，热更新原地生效）。
func GetBlindBoxSetting() *BlindBoxSetting {
	return &blindBoxSetting
}

// ParseDrawTime 解析 DrawTime（"HH:MM"）为小时、分钟。非法则返回 ok=false。
func (s *BlindBoxSetting) ParseDrawTime() (hour, minute int, ok bool) {
	h, m, err := parseHHMM(s.DrawTime)
	if err != nil {
		return 0, 0, false
	}
	return h, m, true
}

// GetPrizes 返回固定奖池奖项（防御式：nil 时返回空切片）。
func (s *BlindBoxSetting) GetPrizes() []BlindBoxPrize {
	if s.Prizes == nil {
		return []BlindBoxPrize{}
	}
	return s.Prizes
}

// parseHHMM 解析 "HH:MM"，校验范围 00:00~23:59。
func parseHHMM(v string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(v), ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("时间格式必须为 HH:MM")
	}
	_, err = fmt.Sscanf(strings.TrimSpace(v), "%d:%d", &hour, &minute)
	if err != nil {
		return 0, 0, errors.New("时间格式必须为 HH:MM")
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, errors.New("开奖时间必须在 00:00~23:59 之间")
	}
	return hour, minute, nil
}

// ValidateBlindBoxDrawTime 校验开奖时间字符串。option 写入前调用。
func ValidateBlindBoxDrawTime(v string) error {
	if _, _, err := parseHHMM(v); err != nil {
		return err
	}
	return nil
}

// ValidateBlindBoxPoolMode 校验奖池模式。
func ValidateBlindBoxPoolMode(v string) error {
	switch strings.TrimSpace(v) {
	case BlindBoxPoolModeFixed, BlindBoxPoolModePercent:
		return nil
	default:
		return errors.New("奖池模式仅支持 fixed 或 percent")
	}
}

// ValidateBlindBoxPrizes 校验固定奖池 JSON 数组字符串。option 写入前调用，
// 错误消息直接返回给管理员，需可读。
//
// 校验项：合法 JSON 数组、数量 ≤ MaxBlindBoxPrizes、名称非空、额度 > 0。
func ValidateBlindBoxPrizes(jsonStr string) error {
	s := strings.TrimSpace(jsonStr)
	if s == "" {
		return nil // 等价空数组
	}
	var prizes []BlindBoxPrize
	if err := common.UnmarshalJsonStr(s, &prizes); err != nil {
		return fmt.Errorf("奖项 JSON 解析失败：%v", err)
	}
	if len(prizes) > MaxBlindBoxPrizes {
		return fmt.Errorf("最多支持 %d 个奖项", MaxBlindBoxPrizes)
	}
	for i, p := range prizes {
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("第 %d 个奖项名称不能为空", i+1)
		}
		if p.Quota <= 0 {
			return fmt.Errorf("第 %d 个奖项的中奖额度必须大于 0", i+1)
		}
	}
	return nil
}
