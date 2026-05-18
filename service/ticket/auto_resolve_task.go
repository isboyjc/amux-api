package ticket

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/bytedance/gopkg/util/gopool"
)

// 自动 resolve 巡检节奏：每小时跑一次，对照 TicketSetting.AutoResolveDays
// 把超期没人回复的工单标 resolved。频率 1h 是因为该决定只依赖天级窗口，
// 没必要更密。runOnce 内自带 CAS 防并发。
const ticketAutoResolveTick = 1 * time.Hour

var (
	ticketAutoResolveOnce    sync.Once
	ticketAutoResolveRunning atomic.Bool
)

// StartTicketAutoResolveTask 在 main.go 启动时调用，仅 master 节点跑。
// 内部首次立即跑一次，之后按 ticker 节奏运行。
func StartTicketAutoResolveTask() {
	ticketAutoResolveOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(),
				fmt.Sprintf("ticket auto-resolve task started: tick=%s", ticketAutoResolveTick))
			ticker := time.NewTicker(ticketAutoResolveTick)
			defer ticker.Stop()
			runAutoResolveSafe()
			for range ticker.C {
				runAutoResolveSafe()
			}
		})
	})
}

func runAutoResolveSafe() {
	if !ticketAutoResolveRunning.CompareAndSwap(false, true) {
		return
	}
	defer ticketAutoResolveRunning.Store(false)
	RunAutoResolveOnce()
}
