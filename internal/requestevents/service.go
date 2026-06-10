package requestevents

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/logging"
)

type Service struct {
	cfg    config.UsageConfig
	dbPath string
	store  *Store
	writer *Writer
}

var (
	globalMu sync.RWMutex
	global   *Service
)

func DefaultConfig() config.UsageConfig {
	enabled := true
	return config.UsageConfig{
		Enabled:       &enabled,
		RetentionDays: 90,
		Writer: config.UsageWriterConfig{
			ChannelSize:    4096,
			BatchSize:      100,
			FlushIntervalMS: 500,
			OnOverflowDrop: true,
		},
	}
}

func ResolveDBPath(cfg *config.Config, usageCfg config.UsageConfig) string {
	if path := filepath.Clean(usageCfg.DBPath); path != "" && path != "." {
		return path
	}
	base := logging.ResolveLogDirectory(cfg)
	if base == "" {
		base = cfg.AuthDir
	}
	if base == "" {
		base = "."
	}
	return filepath.Join(base, "usage", "usage.sqlite")
}

func Start(ctx context.Context, cfg *config.Config) (*Service, error) {
	Stop()
	usageCfg := cfg.Usage
	if usageCfg.Enabled == nil || *usageCfg.Enabled {
		if usageCfg.RetentionDays == 0 && usageCfg.DBPath == "" && usageCfg.Writer.ChannelSize == 0 {
			usageCfg = DefaultConfig()
		}
		if usageCfg.Enabled == nil {
			enabled := true
			usageCfg.Enabled = &enabled
		}
	} else {
		return nil, nil
	}

	dbPath := ResolveDBPath(cfg, usageCfg)
	store, err := Open(dbPath)
	if err != nil {
		return nil, err
	}

	writerCfg := WriterConfig{
		ChannelSize:    usageCfg.Writer.ChannelSize,
		BatchSize:      usageCfg.Writer.BatchSize,
		FlushInterval:  usageCfg.Writer.FlushInterval(),
		OnOverflowDrop: usageCfg.Writer.OnOverflowDrop,
	}
	writer := NewWriter(store, writerCfg)
	writer.Start(ctx)

	service := &Service{
		cfg:    usageCfg,
		dbPath: dbPath,
		store:  store,
		writer: writer,
	}

	globalMu.Lock()
	global = service
	globalMu.Unlock()
	return service, nil
}

func Stop() {
	globalMu.Lock()
	service := global
	global = nil
	globalMu.Unlock()
	if service == nil {
		return
	}
	if service.writer != nil {
		service.writer.Stop()
	}
	if service.store != nil {
		_ = service.store.Close()
	}
}

func Global() *Service {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

func (s *Service) Store() *Store {
	if s == nil {
		return nil
	}
	return s.store
}

func (s *Service) Writer() *Writer {
	if s == nil {
		return nil
	}
	return s.writer
}

func (s *Service) DBPath() string {
	if s == nil {
		return ""
	}
	return s.dbPath
}

func Emit(raw []byte) {
	service := Global()
	if service == nil || service.writer == nil {
		return
	}
	service.writer.Emit(raw)
}
