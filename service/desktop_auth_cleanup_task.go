package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	desktopAuthCleanupInterval = 5 * time.Minute
)

var desktopAuthCleanupOnce sync.Once

func StartDesktopAuthCleanupTask() {
	desktopAuthCleanupOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}

		gopool.Go(func() {
			logger.LogInfo(context.Background(), "desktop auth session cleanup task started")

			ticker := time.NewTicker(desktopAuthCleanupInterval)
			defer ticker.Stop()

			for range ticker.C {
				err := model.CleanupExpiredDesktopSessions()
				if err != nil {
					common.SysError("failed to cleanup expired desktop auth sessions: " + err.Error())
				}
			}
		})
	})
}
