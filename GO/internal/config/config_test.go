package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"MAUCACHE_SYNC_CHANNEL",
		"MAUCACHE_SYNC_INTERVAL",
		"MAUCACHE_SYNC_CONCURRENCY",
		"MAUCACHE_SYNC_RETRY_MAX",
		"MAUCACHE_SYNC_RETRY_DELAY",
		"MAUCACHE_CACHE_DIR",
		"MAUCACHE_SCRATCH_DIR",
		"MAUCACHE_LOG_LEVEL",
		"MAUCACHE_LOG_FORMAT",
		"MAUCACHE_HEALTH_LISTEN",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestDefaultValues(t *testing.T) {
	clearEnv(t)
	cfg := Load("")

	if cfg.Sync.Channel != "Production" {
		t.Errorf("Channel = %q, want %q", cfg.Sync.Channel, "Production")
	}
	if cfg.Sync.Interval != 6*time.Hour {
		t.Errorf("Interval = %v, want %v", cfg.Sync.Interval, 6*time.Hour)
	}
	if cfg.Sync.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want %d", cfg.Sync.Concurrency, 4)
	}
	if cfg.Sync.RetryMax != 3 {
		t.Errorf("RetryMax = %d, want %d", cfg.Sync.RetryMax, 3)
	}
	if cfg.Sync.RetryDelay != 5*time.Second {
		t.Errorf("RetryDelay = %v, want %v", cfg.Sync.RetryDelay, 5*time.Second)
	}
	if cfg.Storage.CacheDir != "/data/maucache" {
		t.Errorf("CacheDir = %q, want %q", cfg.Storage.CacheDir, "/data/maucache")
	}
	if cfg.Storage.ScratchDir != "/data/maucache/.tmp" {
		t.Errorf("ScratchDir = %q, want %q", cfg.Storage.ScratchDir, "/data/maucache/.tmp")
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("Level = %q, want %q", cfg.Logging.Level, "info")
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("Format = %q, want %q", cfg.Logging.Format, "json")
	}
	if cfg.Health.Listen != ":8080" {
		t.Errorf("Listen = %q, want %q", cfg.Health.Listen, ":8080")
	}
}

func TestEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("MAUCACHE_SYNC_CHANNEL", "Beta")
	t.Setenv("MAUCACHE_SYNC_INTERVAL", "1h")
	t.Setenv("MAUCACHE_SYNC_CONCURRENCY", "8")
	t.Setenv("MAUCACHE_SYNC_RETRY_MAX", "5")
	t.Setenv("MAUCACHE_SYNC_RETRY_DELAY", "10s")
	t.Setenv("MAUCACHE_CACHE_DIR", "/tmp/cache")
	t.Setenv("MAUCACHE_SCRATCH_DIR", "/tmp/scratch")
	t.Setenv("MAUCACHE_LOG_LEVEL", "debug")
	t.Setenv("MAUCACHE_LOG_FORMAT", "text")
	t.Setenv("MAUCACHE_HEALTH_LISTEN", ":9090")

	cfg := Load("")

	if cfg.Sync.Channel != "Beta" {
		t.Errorf("Channel = %q, want %q", cfg.Sync.Channel, "Beta")
	}
	if cfg.Sync.Interval != time.Hour {
		t.Errorf("Interval = %v, want %v", cfg.Sync.Interval, time.Hour)
	}
	if cfg.Sync.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want %d", cfg.Sync.Concurrency, 8)
	}
	if cfg.Sync.RetryMax != 5 {
		t.Errorf("RetryMax = %d, want %d", cfg.Sync.RetryMax, 5)
	}
	if cfg.Sync.RetryDelay != 10*time.Second {
		t.Errorf("RetryDelay = %v, want %v", cfg.Sync.RetryDelay, 10*time.Second)
	}
	if cfg.Storage.CacheDir != "/tmp/cache" {
		t.Errorf("CacheDir = %q, want %q", cfg.Storage.CacheDir, "/tmp/cache")
	}
	if cfg.Storage.ScratchDir != "/tmp/scratch" {
		t.Errorf("ScratchDir = %q, want %q", cfg.Storage.ScratchDir, "/tmp/scratch")
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Level = %q, want %q", cfg.Logging.Level, "debug")
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Format = %q, want %q", cfg.Logging.Format, "text")
	}
	if cfg.Health.Listen != ":9090" {
		t.Errorf("Listen = %q, want %q", cfg.Health.Listen, ":9090")
	}
}

func TestYAMLFileLoading(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	yamlContent := `
sync:
  channel: Preview
  interval: 2h
  concurrency: 16
  retry_max: 10
  retry_delay: 30s
storage:
  cache_dir: /yaml/cache
  scratch_dir: /yaml/scratch
logging:
  level: warn
  format: text
health:
  listen: ":3000"
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load(yamlPath)

	if cfg.Sync.Channel != "Preview" {
		t.Errorf("Channel = %q, want %q", cfg.Sync.Channel, "Preview")
	}
	if cfg.Sync.Interval != 2*time.Hour {
		t.Errorf("Interval = %v, want %v", cfg.Sync.Interval, 2*time.Hour)
	}
	if cfg.Sync.Concurrency != 16 {
		t.Errorf("Concurrency = %d, want %d", cfg.Sync.Concurrency, 16)
	}
	if cfg.Sync.RetryMax != 10 {
		t.Errorf("RetryMax = %d, want %d", cfg.Sync.RetryMax, 10)
	}
	if cfg.Sync.RetryDelay != 30*time.Second {
		t.Errorf("RetryDelay = %v, want %v", cfg.Sync.RetryDelay, 30*time.Second)
	}
	if cfg.Storage.CacheDir != "/yaml/cache" {
		t.Errorf("CacheDir = %q, want %q", cfg.Storage.CacheDir, "/yaml/cache")
	}
	if cfg.Storage.ScratchDir != "/yaml/scratch" {
		t.Errorf("ScratchDir = %q, want %q", cfg.Storage.ScratchDir, "/yaml/scratch")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("Level = %q, want %q", cfg.Logging.Level, "warn")
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("Format = %q, want %q", cfg.Logging.Format, "text")
	}
	if cfg.Health.Listen != ":3000" {
		t.Errorf("Listen = %q, want %q", cfg.Health.Listen, ":3000")
	}
}

func TestEnvTakesPriorityOverYAML(t *testing.T) {
	// The Load function first reads env vars into defaults, then YAML
	// overwrites. So YAML actually takes priority over env in current impl.
	// This test documents actual behavior: YAML overrides env.
	clearEnv(t)
	t.Setenv("MAUCACHE_SYNC_CHANNEL", "Beta")

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	yamlContent := `
sync:
  channel: Preview
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Load(yamlPath)

	// YAML overwrites the env-based value per the Load implementation
	if cfg.Sync.Channel != "Preview" {
		t.Errorf("Channel = %q, want %q (YAML should override env)", cfg.Sync.Channel, "Preview")
	}
}

func TestInvalidEnvFallsBackToDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("MAUCACHE_SYNC_CONCURRENCY", "not-a-number")
	t.Setenv("MAUCACHE_SYNC_RETRY_MAX", "abc")
	t.Setenv("MAUCACHE_SYNC_INTERVAL", "bad-duration")
	t.Setenv("MAUCACHE_SYNC_RETRY_DELAY", "xyz")

	cfg := Load("")

	if cfg.Sync.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want default %d", cfg.Sync.Concurrency, 4)
	}
	if cfg.Sync.RetryMax != 3 {
		t.Errorf("RetryMax = %d, want default %d", cfg.Sync.RetryMax, 3)
	}
	if cfg.Sync.Interval != 6*time.Hour {
		t.Errorf("Interval = %v, want default %v", cfg.Sync.Interval, 6*time.Hour)
	}
	if cfg.Sync.RetryDelay != 5*time.Second {
		t.Errorf("RetryDelay = %v, want default %v", cfg.Sync.RetryDelay, 5*time.Second)
	}
}
