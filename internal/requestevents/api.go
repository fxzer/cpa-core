package requestevents

type RequestEventItem struct {
	ID                   string `json:"id"`
	RequestID            string `json:"request_id,omitempty"`
	Timestamp            string `json:"timestamp"`
	TimestampMS          int64  `json:"timestamp_ms"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model"`
	Alias                string `json:"alias,omitempty"`
	Endpoint             string `json:"endpoint,omitempty"`
	Method               string `json:"method,omitempty"`
	Path                 string `json:"path,omitempty"`
	AuthType             string `json:"auth_type,omitempty"`
	AuthIndex            string `json:"auth_index,omitempty"`
	Source               string `json:"source,omitempty"`
	SourceHash           string `json:"source_hash,omitempty"`
	APIKeyHash           string `json:"api_key_hash,omitempty"`
	AccountSnapshot      string `json:"account_snapshot,omitempty"`
	AuthLabelSnapshot    string `json:"auth_label_snapshot,omitempty"`
	AuthFileSnapshot     string `json:"auth_file_snapshot,omitempty"`
	AuthProviderSnapshot string `json:"auth_provider_snapshot,omitempty"`
	AuthSnapshotAtMS     int64  `json:"auth_snapshot_at_ms,omitempty"`
	LatencyMS            *int64 `json:"latency_ms,omitempty"`
	Failed               bool   `json:"failed"`
	FailBody             string `json:"fail_body,omitempty"`
	FailStatusCode       int    `json:"fail_status_code,omitempty"`
	Tokens               Tokens `json:"tokens"`
	RequestBody          string `json:"request_body,omitempty"`
	ResponseBody         string `json:"response_body,omitempty"`
}

type EventSummary struct {
	TotalRequests int64 `json:"total_requests"`
	SuccessCount  int64 `json:"success_count"`
	FailureCount  int64 `json:"failure_count"`
	TotalTokens   int64 `json:"total_tokens"`
}

type ListResponse struct {
	Items   []RequestEventItem `json:"items"`
	Summary EventSummary       `json:"summary"`
}

// PagedListResponse 是分页列表响应。Summary 是整个时间窗的全局聚合
// （与过滤条件无关，供页头统计使用），Total 是过滤后总行数。
type PagedListResponse struct {
	Items    []RequestEventItem `json:"items"`
	Summary  EventSummary       `json:"summary"`
	Total    int64              `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
}

// TimeBucketItem 对应前端趋势图/热力图的一个时间桶。
type TimeBucketItem struct {
	BucketMS int64 `json:"bucket_ms"`
	Total    int64 `json:"total"`
	Success  int64 `json:"success"`
	Failure  int64 `json:"failure"`
	Tokens   int64 `json:"tokens"`
}

// AggregateResponse 是聚合查询响应：全局计数 + 时间维度分布。
// 供页头统计与服务健康热力图使用，避免把全量行拉到前端聚合。
type AggregateResponse struct {
	TotalRequests  int64            `json:"total_requests"`
	SuccessCount   int64            `json:"success_count"`
	FailureCount   int64            `json:"failure_count"`
	TotalTokens    int64            `json:"total_tokens"`
	RequestsByDay  []TimeBucketItem `json:"requests_by_day"`
	RequestsByHour []TimeBucketItem `json:"requests_by_hour"`
	TokensByDay    []TimeBucketItem `json:"tokens_by_day"`
	TokensByHour   []TimeBucketItem `json:"tokens_by_hour"`
}

func ToRequestEventItem(event Event) RequestEventItem {
	id := event.EventHash
	if id == "" {
		id = event.RequestID
	}
	return RequestEventItem{
		ID:                   id,
		RequestID:            event.RequestID,
		Timestamp:            event.Timestamp,
		TimestampMS:          event.TimestampMS,
		Provider:             event.Provider,
		Model:                event.Model,
		Alias:                event.Alias,
		Endpoint:             event.Endpoint,
		Method:               event.Method,
		Path:                 event.Path,
		AuthType:             event.AuthType,
		AuthIndex:            event.AuthIndex,
		Source:               event.Source,
		SourceHash:           event.SourceHash,
		APIKeyHash:           event.APIKeyHash,
		AccountSnapshot:      event.AccountSnapshot,
		AuthLabelSnapshot:    event.AuthLabelSnapshot,
		AuthFileSnapshot:     event.AuthFileSnapshot,
		AuthProviderSnapshot: event.AuthProviderSnapshot,
		AuthSnapshotAtMS:     event.AuthSnapshotAtMS,
		LatencyMS:            event.LatencyMS,
		Failed:               event.Failed,
		FailBody:             event.FailBody,
		FailStatusCode:       event.FailStatusCode,
		RequestBody:          event.RequestBody,
		ResponseBody:         event.ResponseBody,
		Tokens: Tokens{
			InputTokens:     event.InputTokens,
			OutputTokens:    event.OutputTokens,
			ReasoningTokens: event.ReasoningTokens,
			CachedTokens:    event.CachedTokens,
			CacheTokens:     event.CacheTokens,
			TotalTokens:     event.TotalTokens,
		},
	}
}

func BuildListResponse(events []Event) ListResponse {
	items := make([]RequestEventItem, 0, len(events))
	summary := EventSummary{}
	for _, event := range events {
		items = append(items, ToRequestEventItem(event))
		summary.TotalRequests++
		if event.Failed {
			summary.FailureCount++
		} else {
			summary.SuccessCount++
		}
		summary.TotalTokens += event.TotalTokens
	}
	return ListResponse{Items: items, Summary: summary}
}

// BuildPagedListResponse 用分页查询结果 + 聚合结果构造响应。
// summary 用全局聚合值（不是仅当页），保证页头统计在分页下仍正确。
func BuildPagedListResponse(paged PagedEvents, agg AggregateResult, page, pageSize int) PagedListResponse {
	items := make([]RequestEventItem, 0, len(paged.Items))
	for _, event := range paged.Items {
		items = append(items, ToRequestEventItem(event))
	}
	return PagedListResponse{
		Items: items,
		Summary: EventSummary{
			TotalRequests: agg.TotalRequests,
			SuccessCount:  agg.SuccessCount,
			FailureCount:  agg.FailureCount,
			TotalTokens:   agg.TotalTokens,
		},
		Total:    paged.Total,
		Page:     page,
		PageSize: pageSize,
	}
}

func BuildAggregateResponse(agg AggregateResult) AggregateResponse {
	toItems := func(buckets []TimeBucket) []TimeBucketItem {
		items := make([]TimeBucketItem, 0, len(buckets))
		for _, b := range buckets {
			items = append(items, TimeBucketItem{
				BucketMS: b.BucketMS,
				Total:    b.Total,
				Success:  b.Success,
				Failure:  b.Failure,
				Tokens:   b.Tokens,
			})
		}
		return items
	}
	return AggregateResponse{
		TotalRequests:  agg.TotalRequests,
		SuccessCount:   agg.SuccessCount,
		FailureCount:   agg.FailureCount,
		TotalTokens:    agg.TotalTokens,
		RequestsByDay:  toItems(agg.ByDay),
		RequestsByHour: toItems(agg.ByHour),
		// tokens_by_day / tokens_by_hour 复用同一批桶的 Tokens 字段，
		// 前端按需取 Total 或 Tokens。
		TokensByDay:  toItems(agg.ByDay),
		TokensByHour: toItems(agg.ByHour),
	}
}
