package management

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fxzer/cpa-core/v7/internal/requestevents"
	"github.com/gin-gonic/gin"
)

const maxUsageImportBytes = 64 * 1024 * 1024

// 允许的每页条数。前端可传 page_size 在此集合内取值，其余值会被 clamp 到默认。
var allowedPageSizes = map[int]struct{}{10: {}, 20: {}, 50: {}, 100: {}}

const defaultRequestEventsPageSize = 10
const defaultRequestEventsLimit = 50000

func (h *Handler) GetRequestEvents(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}

	startMS := parseEventTimeQuery(c.Query("start"), c.Query("start_time"))
	endMS := parseEventTimeQuery(c.Query("end"), c.Query("end_time"))

	// 旧调用方只传 limit（如导出拉全量），此时退回非分页全量行为，保持兼容。
	if _, hasPaging := c.GetQuery("page"); !hasPaging && c.Query("limit") != "" {
		query := requestevents.ListQuery{
			StartMS: startMS,
			EndMS:   endMS,
			Limit:   parsePositiveIntDefault(c.Query("limit"), defaultRequestEventsLimit),
		}
		events, err := store.ListEvents(c.Request.Context(), query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, requestevents.BuildListResponse(events))
		return
	}

	pageSize := parsePositiveIntDefault(c.Query("page_size"), defaultRequestEventsPageSize)
	if _, ok := allowedPageSizes[pageSize]; !ok {
		pageSize = defaultRequestEventsPageSize
	}
	page := parsePositiveIntDefault(c.Query("page"), 1)
	if page < 1 {
		page = 1
	}

	query := requestevents.ListQuery{
		StartMS:    startMS,
		EndMS:      endMS,
		Limit:      pageSize,
		Offset:     (page - 1) * pageSize,
		Model:      strings.TrimSpace(c.Query("model")),
		Provider:   strings.TrimSpace(c.Query("provider")),
		SourceHash: strings.TrimSpace(c.Query("source_hash")),
		APIKeyHash: strings.TrimSpace(c.Query("api_key_hash")),
		Search:     strings.TrimSpace(c.Query("search")),
	}
	if result := strings.TrimSpace(c.Query("result")); result == "success" {
		f := false
		query.Failed = &f
	} else if result == "failure" {
		t := true
		query.Failed = &t
	}

	paged, err := store.ListEventsPaged(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// summary 反映整个时间窗的全局聚合（与过滤条件无关），页头统计用全局值。
	agg, err := store.Aggregate(c.Request.Context(), requestevents.ListQuery{StartMS: startMS, EndMS: endMS})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, requestevents.BuildPagedListResponse(paged, agg, page, pageSize))
}

// GetRequestEventsAggregate 返回整个时间窗的轻量聚合，供页头统计与热力图使用。
func (h *Handler) GetRequestEventsAggregate(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	query := requestevents.ListQuery{
		StartMS: parseEventTimeQuery(c.Query("start"), c.Query("start_time")),
		EndMS:   parseEventTimeQuery(c.Query("end"), c.Query("end_time")),
	}
	agg, err := store.Aggregate(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, requestevents.BuildAggregateResponse(agg))
}

// GetRequestEventsDistinct 返回指定列的去重非空值，供前端过滤下拉选项。
func (h *Handler) GetRequestEventsDistinct(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	column := strings.TrimSpace(c.Query("column"))
	values, err := store.DistinctValues(c.Request.Context(), column)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"values": values})
}

func (h *Handler) GetRequestEvent(c *gin.Context) {
	store := requestEventsStore()
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "request event persistence is unavailable"})
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing event id"})
		return
	}
	event, err := store.GetEventByHash(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, requestevents.ToRequestEventItem(event))
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
