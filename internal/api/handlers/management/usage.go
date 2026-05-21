package management

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
)

type usageQueueRecord []byte

func (r usageQueueRecord) MarshalJSON() ([]byte, error) {
	if json.Valid(r) {
		return append([]byte(nil), r...), nil
	}
	return json.Marshal(string(r))
}

// GetUsageQueue pops queued usage records from the usage queue.
func (h *Handler) GetUsageQueue(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}

	count, errCount := parseUsageQueueCount(c.Query("count"))
	if errCount != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": errCount.Error()})
		return
	}

	items := redisqueue.PopOldest(count)
	records := make([]usageQueueRecord, 0, len(items))
	for _, item := range items {
		records = append(records, usageQueueRecord(append([]byte(nil), item...)))
	}

	c.JSON(http.StatusOK, records)
}

func parseUsageQueueCount(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 1, nil
	}
	count, errCount := strconv.Atoi(value)
	if errCount != nil || count <= 0 {
		return 0, errors.New("count must be a positive integer")
	}
	return count, nil
}

// GetUsage returns aggregated usage statistics from the usage queue.
// Supports time range filtering: 7h, 24h, 7d, 30d, all
func (h *Handler) GetUsage(c *gin.Context) {
	if h == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "handler unavailable"})
		return
	}

	timeRange := strings.TrimSpace(c.Query("time_range"))
	startTime := firstNonEmpty(c.Query("start_time"), c.Query("start"))
	endTime := firstNonEmpty(c.Query("end_time"), c.Query("end"))
	limitStr := strings.TrimSpace(c.Query("limit"))
	var limit int
	var err error

	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit parameter"})
			return
		}
	}

	allItems := redisqueue.ArchivedUsageRecords()
	if len(allItems) == 0 {
		allItems = redisqueue.PeekAll()
	}
	if len(allItems) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"requests": []interface{}{},
			"stats": gin.H{
				"total_tokens":   0,
				"total_requests": 0,
				"success_count":  0,
				"failure_count":  0,
				"total_cost":     0.0,
			},
		})
		return
	}

	var filteredItems [][]byte
	now := time.Now()
	start, startOK := parseUsageTime(startTime)
	end, endOK := parseUsageTime(endTime)

	for _, item := range allItems {
		record, ok := decodeUsageRecord(item)
		if !ok {
			continue
		}

		timestampStr, _ := record["timestamp"].(string)
		timestamp, err := time.Parse(time.RFC3339Nano, timestampStr)
		if err != nil {
			continue
		}

		include := true

		if timeRange != "" {
			var duration time.Duration
			switch timeRange {
			case "7h":
				duration = 7 * time.Hour
			case "24h":
				duration = 24 * time.Hour
			case "7d":
				duration = 7 * 24 * time.Hour
			case "30d":
				duration = 30 * 24 * time.Hour
			case "all":
				duration = 365 * 24 * time.Hour
			default:
				duration = 24 * time.Hour
			}
			cutoff := now.Add(-duration)
			include = timestamp.After(cutoff)
		}

		if startTime != "" {
			if startOK {
				include = include && timestamp.After(start)
			} else {
				include = false
			}
		}

		if endTime != "" {
			if endOK {
				include = include && timestamp.Before(end)
			} else {
				include = false
			}
		}

		if include {
			filteredItems = append(filteredItems, item)
		}
	}

	if limit > 0 && len(filteredItems) > limit {
		filteredItems = filteredItems[len(filteredItems)-limit:]
	}

	stats := calculateUsageStats(filteredItems)
	records := make([]usageQueueRecord, 0, len(filteredItems))
	for _, item := range filteredItems {
		records = append(records, usageQueueRecord(append([]byte(nil), item...)))
	}

	c.JSON(http.StatusOK, gin.H{
		"requests": records,
		"stats":    stats,
	})
}

type usageStats struct {
	TotalTokens   int64   `json:"total_tokens"`
	TotalRequests int64   `json:"total_requests"`
	SuccessCount  int64   `json:"success_count"`
	FailureCount  int64   `json:"failure_count"`
	TotalCost     float64 `json:"total_cost"`
}

func calculateUsageStats(items [][]byte) usageStats {
	stats := usageStats{}

	for _, item := range items {
		record, ok := decodeUsageRecord(item)
		if !ok {
			continue
		}

		tokens := int64(0)
		if tokensMap, ok := record["tokens"].(map[string]interface{}); ok {
			if totalTokens, ok := tokensMap["total_tokens"].(float64); ok {
				tokens = int64(totalTokens)
			} else {
				if inputTokens, ok := tokensMap["input_tokens"].(float64); ok {
					tokens += int64(inputTokens)
				}
				if outputTokens, ok := tokensMap["output_tokens"].(float64); ok {
					tokens += int64(outputTokens)
				}
			}
		}

		stats.TotalTokens += tokens
		stats.TotalRequests++

		failed, _ := record["failed"].(bool)
		if failed {
			stats.FailureCount++
		} else {
			stats.SuccessCount++
		}

		if cost, ok := record["cost"].(float64); ok {
			stats.TotalCost += cost
		}
	}

	return stats
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func parseUsageTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return parsed, err == nil
}

func decodeUsageRecord(item []byte) (map[string]interface{}, bool) {
	trimmed := bytes.TrimSpace(item)
	if len(trimmed) == 0 {
		return nil, false
	}

	var record map[string]interface{}
	if err := json.Unmarshal(trimmed, &record); err == nil {
		return record, true
	}

	var encoded string
	if err := json.Unmarshal(trimmed, &encoded); err != nil {
		encoded = string(trimmed)
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, false
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false
	}
	if err := json.Unmarshal(decoded, &record); err != nil {
		return nil, false
	}
	return record, true
}
