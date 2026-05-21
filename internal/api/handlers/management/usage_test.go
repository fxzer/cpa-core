package management

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/redisqueue"
)

func TestGetUsageQueuePopsRequestedRecords(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withManagementUsageQueue(t, func() {
		redisqueue.Enqueue([]byte(`{"id":1}`))
		redisqueue.Enqueue([]byte(`{"id":2}`))
		redisqueue.Enqueue([]byte(`{"id":3}`))

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		ginCtx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage-queue?count=2", nil)

		h := &Handler{}
		h.GetUsageQueue(ginCtx)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}

		var payload []json.RawMessage
		if errUnmarshal := json.Unmarshal(rec.Body.Bytes(), &payload); errUnmarshal != nil {
			t.Fatalf("unmarshal response: %v", errUnmarshal)
		}
		if len(payload) != 2 {
			t.Fatalf("response records = %d, want 2", len(payload))
		}
		requireRecordID(t, payload[0], 1)
		requireRecordID(t, payload[1], 2)

		remaining := redisqueue.PopOldest(10)
		if len(remaining) != 1 || string(remaining[0]) != `{"id":3}` {
			t.Fatalf("remaining queue = %q, want third item only", remaining)
		}
	})
}

func TestGetUsageQueueInvalidCountDoesNotPop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withManagementUsageQueue(t, func() {
		redisqueue.Enqueue([]byte(`{"id":1}`))

		rec := httptest.NewRecorder()
		ginCtx, _ := gin.CreateTestContext(rec)
		ginCtx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage-queue?count=0", nil)

		h := &Handler{}
		h.GetUsageQueue(ginCtx)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}

		remaining := redisqueue.PopOldest(10)
		if len(remaining) != 1 || string(remaining[0]) != `{"id":1}` {
			t.Fatalf("remaining queue = %q, want original item", remaining)
		}
	})
}

func TestGetUsageReadsArchivedRecordsAfterMemoryQueueCleared(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withManagementUsageQueue(t, func() {
		withUsageArchive(t, func() {
			redisqueue.Enqueue([]byte(`{"timestamp":"2026-05-17T09:10:53.000Z","endpoint":"POST /v1/chat/completions","model":"deepseek-v4-flash","tokens":{"input_tokens":9,"output_tokens":64,"total_tokens":73},"failed":false}`))
			redisqueue.SetEnabled(false)
			redisqueue.SetEnabled(true)

			rec := httptest.NewRecorder()
			ginCtx, _ := gin.CreateTestContext(rec)
			ginCtx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?start=2026-05-17T00:00:00.000Z&end=2026-05-18T00:00:00.000Z", nil)

			h := &Handler{}
			h.GetUsage(ginCtx)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var payload struct {
				Requests []json.RawMessage `json:"requests"`
				Stats    struct {
					TotalRequests int `json:"total_requests"`
					SuccessCount  int `json:"success_count"`
					FailureCount  int `json:"failure_count"`
					TotalTokens   int `json:"total_tokens"`
				} `json:"stats"`
			}
			if errUnmarshal := json.Unmarshal(rec.Body.Bytes(), &payload); errUnmarshal != nil {
				t.Fatalf("unmarshal response: %v body=%s", errUnmarshal, rec.Body.String())
			}
			if len(payload.Requests) != 1 {
				t.Fatalf("requests = %d, want 1 body=%s", len(payload.Requests), rec.Body.String())
			}
			if payload.Stats.TotalRequests != 1 || payload.Stats.SuccessCount != 1 || payload.Stats.FailureCount != 0 || payload.Stats.TotalTokens != 73 {
				t.Fatalf("unexpected stats: %+v", payload.Stats)
			}
		})
	})
}

func TestGetUsageFiltersArchivedRecordsWithFrontendRangeParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withManagementUsageQueue(t, func() {
		withUsageArchive(t, func() {
			redisqueue.Enqueue([]byte(`{"timestamp":"2026-05-17T08:59:59.000Z","endpoint":"POST /old","model":"old","tokens":{"total_tokens":10},"failed":false}`))
			redisqueue.Enqueue([]byte(`{"timestamp":"2026-05-17T09:10:53.000Z","endpoint":"POST /v1/chat/completions","model":"deepseek-v4-flash","tokens":{"total_tokens":73},"failed":false}`))
			redisqueue.Enqueue([]byte(`{"timestamp":"2026-05-17T10:00:01.000Z","endpoint":"POST /future","model":"future","tokens":{"total_tokens":20},"failed":true}`))

			rec := httptest.NewRecorder()
			ginCtx, _ := gin.CreateTestContext(rec)
			ginCtx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/usage?start=2026-05-17T09:00:00.000Z&end=2026-05-17T10:00:00.000Z", nil)

			h := &Handler{}
			h.GetUsage(ginCtx)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var payload struct {
				Requests []json.RawMessage `json:"requests"`
				Stats    struct {
					TotalRequests int `json:"total_requests"`
					TotalTokens   int `json:"total_tokens"`
				} `json:"stats"`
			}
			if errUnmarshal := json.Unmarshal(rec.Body.Bytes(), &payload); errUnmarshal != nil {
				t.Fatalf("unmarshal response: %v body=%s", errUnmarshal, rec.Body.String())
			}
			if len(payload.Requests) != 1 || payload.Stats.TotalRequests != 1 || payload.Stats.TotalTokens != 73 {
				t.Fatalf("unexpected filtered payload: %+v body=%s", payload, rec.Body.String())
			}
		})
	})
}

func withManagementUsageQueue(t *testing.T, fn func()) {
	t.Helper()

	prevQueueEnabled := redisqueue.Enabled()
	redisqueue.SetEnabled(false)
	redisqueue.SetEnabled(true)

	defer func() {
		redisqueue.SetEnabled(false)
		redisqueue.SetEnabled(prevQueueEnabled)
	}()

	fn()
}

func withUsageArchive(t *testing.T, fn func()) {
	t.Helper()

	prevArchivePath := redisqueue.UsageArchivePath()
	redisqueue.SetUsageArchivePath(filepath.Join(t.TempDir(), "usage-events.jsonl"))
	defer func() {
		redisqueue.SetUsageArchivePath(prevArchivePath)
	}()

	fn()
}

func requireRecordID(t *testing.T, raw json.RawMessage, want int) {
	t.Helper()

	var payload struct {
		ID int `json:"id"`
	}
	if errUnmarshal := json.Unmarshal(raw, &payload); errUnmarshal != nil {
		t.Fatalf("unmarshal record: %v", errUnmarshal)
	}
	if payload.ID != want {
		t.Fatalf("record id = %d, want %d", payload.ID, want)
	}
}
