package auth

import (
	"context"
	"net/http"
	"testing"

	"github.com/fxzer/cpa-core/v7/internal/registry"
	"github.com/fxzer/cpa-core/v7/sdk/cliproxy/executor"
)

// registerPriorityAuth 是个小工具：注册一个带优先级的 auth 并登记模型支持。
func registerPriorityAuth(t *testing.T, m *Manager, id, provider, model string, priority int) *Auth {
	t.Helper()
	auth := &Auth{
		ID:       id,
		Provider: provider,
		Attributes: map[string]string{
			"priority": itoa(priority),
		},
	}
	reg := registry.GetGlobalRegistry()
	reg.RegisterClient(auth.ID, provider, []*registry.ModelInfo{{ID: model}})
	t.Cleanup(func() { reg.UnregisterClient(auth.ID) })
	if _, err := m.Register(context.Background(), auth); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	return auth
}

func itoa(n int) string {
	// 避免在测试里再 import strconv 仅为一次转换
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestManager_PriorityDegradation_FallsBackToLowerPriorityWhenHighTierExhausts
// 复现 fxzer 报告的场景：高优先级 provider 全挂时，必须降级到低优先级 provider，
// 而不是因 maxRetryCredentials 提前耗尽而中断 Agent 工作流。
//
// 配置：maxRetryCredentials=1（每桶最多试 1 个）
//   - 高优先级 auth-high-A (priority=10)：失败 (500)
//   - 高优先级 auth-high-B (priority=10)：失败 (500)
//   - 低优先级 auth-low (priority=1)：成功
//
// 期望：请求成功，且 low 被实际调用过。
// 旧行为（bug）：试 1 个 high 失败 → maxRetryCredentials=1 触发全局上限 → 直接返回错误，
// low 根本没机会被试到 → Agent 中断。
func TestManager_PriorityDegradation_FallsBackToLowerPriorityWhenHighTierExhausts(t *testing.T) {
	m := NewManager(nil, nil, nil)
	// maxRetryCredentials=1：关键在于"每个优先级桶最多试 1 个"，而不是"全局最多 1 个"
	m.SetRetryConfig(0, 0, 1)

	const provider = "claude"
	const model = "test-model"

	executorMock := &authFallbackExecutor{
		id: provider,
		executeErrors: map[string]error{
			"auth-high-A": &Error{HTTPStatus: http.StatusInternalServerError, Message: "high tier down"},
			"auth-high-B": &Error{HTTPStatus: http.StatusInternalServerError, Message: "high tier down"},
			// auth-low 没有错误 → 成功
		},
	}
	m.RegisterExecutor(executorMock)

	registerPriorityAuth(t, m, "auth-high-A", provider, model, 10)
	registerPriorityAuth(t, m, "auth-high-B", provider, model, 10)
	low := registerPriorityAuth(t, m, "auth-low", provider, model, 1)

	resp, err := m.Execute(context.Background(), []string{provider}, executor.Request{Model: model}, executor.Options{})
	if err != nil {
		t.Fatalf("expected degradation to low-priority auth to succeed, got error: %v", err)
	}
	if string(resp.Payload) != low.ID {
		t.Fatalf("expected response from %s, got %q", low.ID, string(resp.Payload))
	}

	calls := executorMock.ExecuteCalls()
	if !containsString(calls, low.ID) {
		t.Fatalf("expected low-priority auth %q to be attempted, calls were: %v", low.ID, calls)
	}
}

// TestManager_PriorityDegradation_AllTiersFailStillRespectsLimit
// 反例：当所有优先级都失败时，仍然必须返回错误（不能因为分桶就变成无限重试）。
// 配置：maxRetryCredentials=2，每个桶 2 个 auth 全失败 → 最终应返回错误。
func TestManager_PriorityDegradation_AllTiersFailStillRespectsLimit(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.SetRetryConfig(0, 0, 2)

	const provider = "claude"
	const model = "test-model"

	executorMock := &authFallbackExecutor{
		id: provider,
		executeErrors: map[string]error{
			"high-1": &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
			"high-2": &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
			"low-1":  &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
			"low-2":  &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
		},
	}
	m.RegisterExecutor(executorMock)

	registerPriorityAuth(t, m, "high-1", provider, model, 10)
	registerPriorityAuth(t, m, "high-2", provider, model, 10)
	registerPriorityAuth(t, m, "low-1", provider, model, 1)
	registerPriorityAuth(t, m, "low-2", provider, model, 1)

	_, err := m.Execute(context.Background(), []string{provider}, executor.Request{Model: model}, executor.Options{})
	if err == nil {
		t.Fatalf("expected error when all tiers fail, got nil")
	}

	// 所有 auth 都应该被试过（每个桶 2 个，2 个桶 = 4 次）
	calls := executorMock.ExecuteCalls()
	if len(calls) != 4 {
		t.Fatalf("expected all 4 auths to be attempted (2 per tier × 2 tiers), got %d calls: %v", len(calls), calls)
	}
}

// TestManager_PriorityDegradation_ZeroLimitMeansUnlimitedPerTier
// 边界：maxRetryCredentials=0 表示不限制（旧行为兼容）。
// 高优先级有 3 个全失败、低优先级 1 个成功 → 必须试遍高优 3 个后降级到低优成功。
func TestManager_PriorityDegradation_ZeroLimitMeansUnlimitedPerTier(t *testing.T) {
	m := NewManager(nil, nil, nil)
	m.SetRetryConfig(0, 0, 0)

	const provider = "claude"
	const model = "test-model"

	executorMock := &authFallbackExecutor{
		id: provider,
		executeErrors: map[string]error{
			"high-1": &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
			"high-2": &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
			"high-3": &Error{HTTPStatus: http.StatusInternalServerError, Message: "down"},
		},
	}
	m.RegisterExecutor(executorMock)

	registerPriorityAuth(t, m, "high-1", provider, model, 10)
	registerPriorityAuth(t, m, "high-2", provider, model, 10)
	registerPriorityAuth(t, m, "high-3", provider, model, 10)
	low := registerPriorityAuth(t, m, "low", provider, model, 1)

	resp, err := m.Execute(context.Background(), []string{provider}, executor.Request{Model: model}, executor.Options{})
	if err != nil {
		t.Fatalf("expected unlimited retry to reach low-priority auth, got error: %v", err)
	}
	if string(resp.Payload) != low.ID {
		t.Fatalf("expected response from low auth, got %q", string(resp.Payload))
	}
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
