package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

// 盲盒每日开奖定时任务。
//
// 与订阅额度重置任务同构：仅主节点运行（避免多实例重复发奖），sync.Once 只启动一次，
// atomic.Bool 防止上一轮未跑完重入。每分钟 tick 一次，检查「最近一个已越过的开奖边界」
// 对应的周期是否已结算，未结算则结算之。结算幂等（draw_date 唯一约束兜底）。
//
// 功能关闭时每次 tick 直接返回，任务完全休眠，不产生任何副作用。

const blindBoxTickInterval = 1 * time.Minute

var (
	blindBoxDrawOnce    sync.Once
	blindBoxDrawRunning atomic.Bool
)

func StartBlindBoxDrawTask() {
	blindBoxDrawOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("blindbox draw task started: tick=%s", blindBoxTickInterval))
			ticker := time.NewTicker(blindBoxTickInterval)
			defer ticker.Stop()

			runBlindBoxDrawOnce()
			for range ticker.C {
				runBlindBoxDrawOnce()
			}
		})
	})
}

func runBlindBoxDrawOnce() {
	if !blindBoxDrawRunning.CompareAndSwap(false, true) {
		return
	}
	defer blindBoxDrawRunning.Store(false)

	setting := operation_setting.GetBlindBoxSetting()
	if !setting.Enabled {
		return
	}
	hour, minute, ok := setting.ParseDrawTime()
	if !ok {
		return
	}

	now := time.Now()

	// 过期清扫与结算解耦，先跑：判定标准是 expire_at 而非"下一期有没有开成"。
	// 这样即便某期结算失败或主节点停过机，用户看到的状态也能自愈，不会出现
	// "显示待开启但点不动"。
	if n, err := model.ExpireBlindBoxWinners(now.Unix()); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("blindbox expire winners failed: %v", err))
	} else if n > 0 {
		logger.LogInfo(context.Background(), fmt.Sprintf("blindbox expired %d unopened winners", n))
	}

	// 周期 = [上一个开奖时点, 本次开奖时点)，领取截止 = 下一个开奖时点。
	// 三个边界都走日历运算，跨夏令时也严格等于"两次开奖之间"。
	boundary := model.MostRecentDrawBoundary(now, hour, minute)
	drawDate := boundary.Format("2006-01-02")
	periodEnd := boundary.Unix()
	periodStart := model.PrevDrawBoundary(boundary, hour, minute).Unix()
	expireAt := model.NextDrawBoundary(boundary, hour, minute).Unix()

	if err := model.SettleBlindBoxDraw(drawDate, periodStart, periodEnd, expireAt); err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("blindbox draw settle failed for %s: %v", drawDate, err))
	}
}
