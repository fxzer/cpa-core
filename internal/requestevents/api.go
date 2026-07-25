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
