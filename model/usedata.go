package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id               int    `json:"id"`
	UserID           int    `json:"user_id" gorm:"index"`
	Username         string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName        string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	TokenUsed        int    `json:"token_used" gorm:"default:0"`
	PromptTokens     int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens int    `json:"completion_tokens" gorm:"default:0"`
	CacheReadTokens  int    `json:"cache_read_tokens" gorm:"default:0"`
	CacheWriteTokens int    `json:"cache_write_tokens" gorm:"default:0"`
	Count            int    `json:"count" gorm:"default:0"`
	Quota            int    `json:"quota" gorm:"default:0"`
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(userId int, username string, modelName string, quota int, createdAt int64,
	promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int) {
	key := fmt.Sprintf("%d-%s-%s-%d", userId, username, modelName, createdAt)
	tokenUsed := promptTokens + completionTokens
	quotaData, ok := CacheQuotaData[key]
	if ok {
		quotaData.Count += 1
		quotaData.Quota += quota
		quotaData.TokenUsed += tokenUsed
		quotaData.PromptTokens += promptTokens
		quotaData.CompletionTokens += completionTokens
		quotaData.CacheReadTokens += cacheReadTokens
		quotaData.CacheWriteTokens += cacheWriteTokens
	} else {
		quotaData = &QuotaData{
			UserID:           userId,
			Username:         username,
			ModelName:        modelName,
			CreatedAt:        createdAt,
			Count:            1,
			Quota:            quota,
			TokenUsed:        tokenUsed,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			CacheReadTokens:  cacheReadTokens,
			CacheWriteTokens: cacheWriteTokens,
		}
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(userId int, username string, modelName string, quota int, createdAt int64,
	promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int) {
	// 只精确到小时
	createdAt = createdAt - (createdAt % 3600)

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(userId, username, modelName, quota, createdAt,
		promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt).First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData.UserID, quotaData.Username, quotaData.ModelName,
				quotaData.Count, quotaData.Quota, quotaData.CreatedAt,
				quotaData.TokenUsed, quotaData.PromptTokens, quotaData.CompletionTokens,
				quotaData.CacheReadTokens, quotaData.CacheWriteTokens)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(userId int, username string, modelName string, count int, quota int, createdAt int64,
	tokenUsed, promptTokens, completionTokens, cacheReadTokens, cacheWriteTokens int) {
	err := DB.Table("quota_data").Where("user_id = ? and username = ? and model_name = ? and created_at = ?",
		userId, username, modelName, createdAt).Updates(map[string]interface{}{
		"count":              gorm.Expr("count + ?", count),
		"quota":              gorm.Expr("quota + ?", quota),
		"token_used":         gorm.Expr("token_used + ?", tokenUsed),
		"prompt_tokens":      gorm.Expr("prompt_tokens + ?", promptTokens),
		"completion_tokens":  gorm.Expr("completion_tokens + ?", completionTokens),
		"cache_read_tokens":  gorm.Expr("cache_read_tokens + ?", cacheReadTokens),
		"cache_write_tokens": gorm.Expr("cache_write_tokens + ?", cacheWriteTokens),
	}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, " +
			"sum(count) as count, " +
			"sum(quota) as quota, " +
			"sum(token_used) as token_used, " +
			"sum(prompt_tokens) as prompt_tokens, " +
			"sum(completion_tokens) as completion_tokens, " +
			"sum(cache_read_tokens) as cache_read_tokens, " +
			"sum(cache_write_tokens) as cache_write_tokens").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select("model_name, created_at, " +
			"sum(count) as count, " +
			"sum(quota) as quota, " +
			"sum(token_used) as token_used, " +
			"sum(prompt_tokens) as prompt_tokens, " +
			"sum(completion_tokens) as completion_tokens, " +
			"sum(cache_read_tokens) as cache_read_tokens, " +
			"sum(cache_write_tokens) as cache_write_tokens").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

// MigrateQuotaDataTokenSplit 一次性迁移：从 logs 表回填 quota_data 的 token 拆分列
// 通过 Option 表记录是否已执行，避免重复执行
func MigrateQuotaDataTokenSplit() {
	migrationKey := "quota_data_token_split_migration_v1"
	var opt Option
	if err := DB.Where(commonKeyCol+" = ?", migrationKey).First(&opt).Error; err == nil {
		if opt.Value == "done" {
			return
		}
	}

	const batchSize = 500
	lastId := 0
	migrated := 0
	skipped := 0

	common.SysLog("MigrateQuotaDataTokenSplit: starting backfill from logs...")

	for {
		var rows []QuotaData
		err := DB.Table("quota_data").
			Where("id > ? AND prompt_tokens = 0 AND completion_tokens = 0 AND cache_read_tokens = 0 AND cache_write_tokens = 0 AND token_used > 0", lastId).
			Order("id ASC").
			Limit(batchSize).
			Find(&rows).Error
		if err != nil {
			common.SysLog(fmt.Sprintf("MigrateQuotaDataTokenSplit: query quota_data error: %s", err))
			return
		}
		if len(rows) == 0 {
			break
		}

		for _, row := range rows {
			lastId = row.Id
			hourStart := row.CreatedAt
			hourEnd := row.CreatedAt + 3600

			var logs []Log
			err := LOG_DB.Table("logs").
				Select("prompt_tokens, completion_tokens, other").
				Where("user_id = ? AND username = ? AND model_name = ? AND type = ? AND created_at >= ? AND created_at < ?",
					row.UserID, row.Username, row.ModelName, LogTypeConsume, hourStart, hourEnd).
				Find(&logs).Error
			if err != nil || len(logs) == 0 {
				skipped++
				continue
			}

			var sumPrompt, sumCompletion, sumCacheRead, sumCacheWrite int
			for _, l := range logs {
				sumPrompt += l.PromptTokens
				sumCompletion += l.CompletionTokens
				otherMap := map[string]interface{}{}
				_ = common.UnmarshalJsonStr(l.Other, &otherMap)
				if v, ok := otherMap["cache_tokens"]; ok {
					if fv, ok := v.(float64); ok {
						sumCacheRead += int(fv)
					}
				}
				if v, ok := otherMap["cache_write_tokens"]; ok {
					if fv, ok := v.(float64); ok {
						sumCacheWrite += int(fv)
					}
				}
			}

			result := DB.Table("quota_data").
				Where("id = ? AND prompt_tokens = 0", row.Id).
				Updates(map[string]interface{}{
					"prompt_tokens":      sumPrompt,
					"completion_tokens":  sumCompletion,
					"cache_read_tokens":  sumCacheRead,
					"cache_write_tokens": sumCacheWrite,
				})
			if result.Error != nil {
				common.SysLog(fmt.Sprintf("MigrateQuotaDataTokenSplit: update id=%d error: %s", row.Id, result.Error))
				continue
			}
			if result.RowsAffected > 0 {
				migrated++
			}
		}

		common.SysLog(fmt.Sprintf("MigrateQuotaDataTokenSplit: progress - migrated %d, skipped %d, last_id %d", migrated, skipped, lastId))
	}

	if err := UpdateOption(migrationKey, "done"); err != nil {
		common.SysLog(fmt.Sprintf("MigrateQuotaDataTokenSplit: failed to save completion marker: %s", err))
	}
	common.SysLog(fmt.Sprintf("MigrateQuotaDataTokenSplit: completed, migrated %d rows, skipped %d rows (no matching logs)", migrated, skipped))
}
