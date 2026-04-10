package model

import (
	"fmt"
	"sync"
	"time"
)

type ModelHealthBucket struct {
	ModelName    string `json:"model_name" gorm:"column:model_name"`
	Group        string `json:"group" gorm:"column:group"`
	TimeBucket   int64  `json:"time_bucket" gorm:"column:time_bucket"`
	SuccessCount int64  `json:"success_count" gorm:"column:success_count"`
	ErrorCount   int64  `json:"error_count" gorm:"column:error_count"`
}

type ModelHealthResponse struct {
	StartTime   int64                              `json:"start_time"`
	EndTime     int64                              `json:"end_time"`
	BucketSize  int                                `json:"bucket_size"`
	BucketCount int                                `json:"bucket_count"`
	Data        map[string]map[string][]HealthCell `json:"data"`
}

type HealthCell struct {
	TimeBucket   int64 `json:"t"`
	SuccessCount int64 `json:"s"`
	ErrorCount   int64 `json:"e"`
}

var (
	healthCache24h          *ModelHealthResponse
	healthCache24hTimestamp time.Time
	healthCache7d           *ModelHealthResponse
	healthCache7dTimestamp  time.Time
	healthCacheLock         sync.RWMutex
	healthCacheTTL          = 1 * time.Minute
)

func GetModelHealthData(timeRange string) (*ModelHealthResponse, error) {
	healthCacheLock.RLock()
	cached, ts := getHealthCache(timeRange)
	if cached != nil && time.Since(ts) < healthCacheTTL {
		healthCacheLock.RUnlock()
		return cached, nil
	}
	healthCacheLock.RUnlock()

	healthCacheLock.Lock()
	defer healthCacheLock.Unlock()

	// Double-check after acquiring write lock
	cached, ts = getHealthCache(timeRange)
	if cached != nil && time.Since(ts) < healthCacheTTL {
		return cached, nil
	}

	var hours int
	var bucketMinutes int
	switch timeRange {
	case "7d":
		hours = 168
		bucketMinutes = 240
	default:
		hours = 24
		bucketMinutes = 30
	}

	bucketSeconds := int64(bucketMinutes * 60)
	now := time.Now().Unix()
	bucketCount := (hours * 60) / bucketMinutes

	// Anchor from current time bucket backwards so the last cell always covers "now"
	endBucket := now - (now % bucketSeconds)
	startBucket := endBucket - int64(bucketCount-1)*bucketSeconds
	// SQL range covers from startBucket to now (inclusive)
	startTime := startBucket
	endTime := now

	buckets, err := queryHealthBuckets(startTime, endTime, bucketSeconds)
	if err != nil {
		return nil, err
	}

	response := buildHealthResponse(buckets, startBucket, endBucket, int(bucketSeconds), bucketCount)

	setHealthCache(timeRange, response)
	return response, nil
}

func queryHealthBuckets(startTime, endTime, bucketSeconds int64) ([]ModelHealthBucket, error) {
	var results []ModelHealthBucket

	timeBucketExpr := fmt.Sprintf("(created_at - (created_at %% %d))", bucketSeconds)

	err := LOG_DB.Table("logs").
		Select(fmt.Sprintf(
			"model_name, %s, %s AS time_bucket, "+
				"SUM(CASE WHEN type = %d THEN 1 ELSE 0 END) AS success_count, "+
				"SUM(CASE WHEN type = %d THEN 1 ELSE 0 END) AS error_count",
			logGroupCol, timeBucketExpr,
			LogTypeConsume, LogTypeError,
		)).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Group(fmt.Sprintf("model_name, %s, %s", logGroupCol, timeBucketExpr)).
		Order(fmt.Sprintf("model_name, %s", timeBucketExpr)).
		Find(&results).Error

	return results, err
}

func buildHealthResponse(buckets []ModelHealthBucket, startTime, endTime int64, bucketSize, bucketCount int) *ModelHealthResponse {
	data := make(map[string]map[string][]HealthCell)

	for _, b := range buckets {
		if _, ok := data[b.ModelName]; !ok {
			data[b.ModelName] = make(map[string][]HealthCell)
		}
		data[b.ModelName][b.Group] = append(data[b.ModelName][b.Group], HealthCell{
			TimeBucket:   b.TimeBucket,
			SuccessCount: b.SuccessCount,
			ErrorCount:   b.ErrorCount,
		})
	}

	return &ModelHealthResponse{
		StartTime:   startTime,
		EndTime:     endTime,
		BucketSize:  bucketSize,
		BucketCount: bucketCount,
		Data:        data,
	}
}

// FilterHealthByGroups returns a copy of the response with only the specified groups.
func FilterHealthByGroups(resp *ModelHealthResponse, groups map[string]string) *ModelHealthResponse {
	if resp == nil {
		return nil
	}
	filtered := &ModelHealthResponse{
		StartTime:   resp.StartTime,
		EndTime:     resp.EndTime,
		BucketSize:  resp.BucketSize,
		BucketCount: resp.BucketCount,
		Data:        make(map[string]map[string][]HealthCell),
	}
	for modelName, groupMap := range resp.Data {
		for group, cells := range groupMap {
			if _, ok := groups[group]; ok {
				if _, exists := filtered.Data[modelName]; !exists {
					filtered.Data[modelName] = make(map[string][]HealthCell)
				}
				filtered.Data[modelName][group] = cells
			}
		}
	}
	return filtered
}

func getHealthCache(timeRange string) (*ModelHealthResponse, time.Time) {
	if timeRange == "7d" {
		return healthCache7d, healthCache7dTimestamp
	}
	return healthCache24h, healthCache24hTimestamp
}

func setHealthCache(timeRange string, resp *ModelHealthResponse) {
	if timeRange == "7d" {
		healthCache7d = resp
		healthCache7dTimestamp = time.Now()
	} else {
		healthCache24h = resp
		healthCache24hTimestamp = time.Now()
	}
}
