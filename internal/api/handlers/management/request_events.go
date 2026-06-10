package management

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/fxzer/cpa-core/v7/internal/requestevents"
)

const maxUsageImportBytes = 64 * 1024 * 1024

func (h *Handler) GetRequestEvents(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}

	query := requestevents.ListQuery{
		StartMS: parseEventTimeQuery(c.Query("start"), c.Query("start_time")),
		EndMS:   parseEventTimeQuery(c.Query("end"), c.Query("end_time")),
		Limit:   parsePositiveIntDefault(c.Query("limit"), 50000),
	}
	events, err := store.ListEvents(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, requestevents.BuildListResponse(events))
}

func (h *Handler) GetRequestEventsStatus(c *gin.Context) {
	service := requestevents.Global()
	if service == nil || service.Store() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	events, deadLetters, err := service.Store().Counts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	status := gin.H{
		"db_path":      service.DBPath(),
		"event_count":  events,
		"dead_letters": deadLetters,
	}
	if writer := service.Writer(); writer != nil {
		writerStatus := writer.Status()
		status["writer"] = writerStatus
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) ExportRequestEvents(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	data, err := store.ExportJSONL(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Content-Disposition", `attachment; filename="request-events.jsonl"`)
	c.Data(http.StatusOK, "application/x-ndjson", data)
}

func (h *Handler) ImportRequestEvents(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxUsageImportBytes)
	data, err := io.ReadAll(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	events, failed, err := requestevents.ParseImportJSONL(data)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "failed": failed})
		return
	}
	result, err := store.InsertEvents(c.Request.Context(), events)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"added":   result.Inserted,
		"skipped": result.Skipped,
		"total":   len(events),
		"failed":  failed,
	})
}

func (h *Handler) DeleteRequestEvents(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	deleted, err := store.DeleteEvents(c.Request.Context(), req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func requestEventsStore() *requestevents.Store {
	service := requestevents.Global()
	if service == nil {
		return nil
	}
	return service.Store()
}

func parsePositiveIntDefault(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseEventTimeQuery(values ...string) int64 {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed.UnixMilli()
		}
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UnixMilli()
		}
	}
	return 0
}
