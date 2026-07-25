package auth

import (
	"net/http"
	"testing"
)

// 这些错误消息全部来自线上 2026-07-24 usage_events 日志的 fail_body 原文。
// 它们共同的特征是：与具体 provider 无关，是客户端请求本身有问题，
// 换任何 provider / API key 重试都不会成功，所以 isRequestInvalidError 应该返回 true，
// 让 conductor 立即返回错误而不是把所有凭证轮询一遍。
func TestIsRequestInvalidError_ClientShapeFailuresFromLogs(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		message string
	}{
		// freemodel gpt-5.5 / CPA ThinkingError：level 非法且附带 valid levels 列表
		{"freemodel level xhigh not supported", http.StatusBadRequest, `level "xhigh" not supported, valid levels: low, medium, high`},
		// ark doubao / syscxp deepseek: InvalidParameter
		{"ark InvalidParameter reasoning_effort", http.StatusBadRequest, `{"error":{"code":"InvalidParameter","message":"Invalid reasoning_effort: xhigh Request id: 021784855368789bd223177cc7b14","param":"reasoning_effort"}}`},
		{"syscxp InvalidParameter", http.StatusBadRequest, `{"error":{"code":"InvalidParameter","message":"Invalid reasoning_effort: xhigh"}}`},
		{"bare InvalidParameter code", http.StatusBadRequest, `{"error":{"code":"InvalidParameter","message":"bad request"}}`},
		{"invalid reasoning_effort plain text", http.StatusBadRequest, `Invalid reasoning_effort: xhigh`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := &retryAfterStatusError{status: tc.status, message: tc.message}
			if !isRequestInvalidError(err) {
				t.Errorf("isRequestInvalidError() = false, want true\nstatus=%d message=%s", tc.status, tc.message)
			}
		})
	}
}

// 反例：以下错误必须继续 failover，不能被误判为客户端请求错误。
func TestIsRequestInvalidError_StillRetriable(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		message string
	}{
		// model-support 类错误必须走 failover（已有 isModelSupportError 守卫）
		{"model not supported", http.StatusBadRequest, `{"error":{"code":"model_not_supported","message":"The requested model is not supported"}}`},
		// 能力类 "not supported" 没有 valid levels：可能换 provider 就能成功，必须继续 failover
		{"tools not supported on provider", http.StatusBadRequest, `tools are not supported for this endpoint`},
		{"temperature not supported bare", http.StatusBadRequest, `temperature value 3.5 not supported`},
		// 真正的 upstream 5xx / 网络错误必须重试
		{"siliconflow 500 context canceled", http.StatusInternalServerError, `Post "https://api.siliconflow.com/v1/chat/completions": context canceled`},
		{"generic 500", http.StatusInternalServerError, `internal server error`},
		// 429 必须重试（配额冷却机制处理）
		{"ark 429 set limit exceeded", http.StatusTooManyRequests, `{"error":{"code":"SetLimitExceeded","message":"Your account has reached the set inference limit"}}`},
		// 401 必须重试（OAuth token 过期由自动刷新处理，单 key 失效不该中断工作流）
		{"tokengen invalid token 401", http.StatusUnauthorized, `{"error":{"code":"","message":"Invalid token (request id: 202607240107395110408848268d9d6RQYVxst6)","type":"new_api_error"}}`},
		// nil 不是请求错误
		{"nil error", 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.message != "" {
				err = &retryAfterStatusError{status: tc.status, message: tc.message}
			}
			if isRequestInvalidError(err) {
				t.Errorf("isRequestInvalidError() = true, want false (this error should still failover)\nstatus=%d message=%s", tc.status, tc.message)
			}
		})
	}
}
