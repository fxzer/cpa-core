package management

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"gopkg.in/yaml.v3"
)

func TestAuthenticateManagementKey_LocalhostIPBan_BlocksCorrectKeyDuringBan(t *testing.T) {
	h := &Handler{
		cfg:            &config.Config{},
		failedAttempts: make(map[string]*attemptInfo),
		envSecret:      "test-secret",
	}

	for i := 0; i < 5; i++ {
		allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "wrong-secret")
		if allowed {
			t.Fatalf("expected auth to be denied at attempt %d", i+1)
		}
		if statusCode != http.StatusUnauthorized || errMsg != "invalid management key" {
			t.Fatalf("unexpected auth failure at attempt %d: status=%d msg=%q", i+1, statusCode, errMsg)
		}
	}

	allowed, statusCode, errMsg := h.AuthenticateManagementKey("127.0.0.1", true, "test-secret")
	if allowed {
		t.Fatalf("expected correct key to be denied while banned")
	}
	if statusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden status while banned, got %d", statusCode)
	}
	if !strings.HasPrefix(errMsg, "IP banned due to too many failed attempts. Try again in") {
		t.Fatalf("unexpected banned message: %q", errMsg)
	}
}

func TestPersistNotifiesConfigUpdateHook(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"
	cfg := &config.Config{Port: 8317}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err = os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	h := NewHandler(cfg, configPath, nil)
	cfg.Port = 9321

	gotCh := make(chan *config.Config, 1)
	h.SetConfigUpdateHook(func(updated *config.Config) {
		gotCh <- updated
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if !h.persist(c) {
		t.Fatal("expected persist to succeed")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var got *config.Config
	select {
	case got = <-gotCh:
	case <-time.After(time.Second):
		t.Fatal("expected config update hook to run")
	}
	if got == cfg {
		t.Fatal("expected hook config to be a snapshot, not the live pointer")
	}
	if got.Port != 9321 {
		t.Fatalf("hook config port = %d, want 9321", got.Port)
	}
}
