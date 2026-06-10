package config

import "time"

// UsageConfig controls SQLite-backed request event persistence.
type UsageConfig struct {
	// Enabled toggles persistence. Nil or true means enabled (default).
	Enabled *bool `yaml:"enabled" json:"enabled"`
	// DBPath is the SQLite database path. Empty uses <log-dir>/usage/usage.sqlite.
	DBPath string `yaml:"db-path" json:"db-path"`
	// RetentionDays controls automatic pruning. 0 disables pruning.
	RetentionDays int `yaml:"retention-days" json:"retention-days"`
	Writer        UsageWriterConfig `yaml:"writer" json:"writer"`
}

type UsageWriterConfig struct {
	ChannelSize     int  `yaml:"channel-size" json:"channel-size"`
	BatchSize       int  `yaml:"batch-size" json:"batch-size"`
	FlushIntervalMS int  `yaml:"flush-interval-ms" json:"flush-interval-ms"`
	OnOverflowDrop  bool `yaml:"on-overflow-drop" json:"on-overflow-drop"`
}

func (c UsageWriterConfig) FlushInterval() time.Duration {
	if c.FlushIntervalMS <= 0 {
		return 500 * time.Millisecond
	}
	return time.Duration(c.FlushIntervalMS) * time.Millisecond
}

func (c UsageConfig) PersistenceEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}
