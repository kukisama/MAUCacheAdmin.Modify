package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 应用配置
// 对应 PowerShell MacUpdatesOffice.Modify.ps1 的 param 块中的三个参数，
// 并扩展为完整配置体系，支持环境变量 + YAML 文件双重配置
type Config struct {
	Sync    SyncConfig    `yaml:"sync"`
	Storage StorageConfig `yaml:"storage"`
	Logging LogConfig     `yaml:"logging"`
	Health  HealthConfig  `yaml:"health"`
}

// SyncConfig 同步引擎配置
type SyncConfig struct {
	Channel     string        `yaml:"channel"`     // Production / Preview / Beta
	Interval    time.Duration `yaml:"interval"`    // 同步间隔，默认 6h
	Concurrency int           `yaml:"concurrency"` // 并发下载数，默认 4
	RetryMax    int           `yaml:"retry_max"`   // 重试次数，默认 3
	RetryDelay  time.Duration `yaml:"retry_delay"` // 重试退避基数，默认 5s
}

// StorageConfig 存储路径配置
// 对应 PowerShell 的 $maupath 和 $mautemppath 参数
type StorageConfig struct {
	CacheDir   string `yaml:"cache_dir"`   // 对应 $maupath → /data/maucache
	ScratchDir string `yaml:"scratch_dir"` // 对应 $mautemppath → /data/maucache/.tmp
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`  // debug / info / warn / error
	Format string `yaml:"format"` // json / text
}

// HealthConfig 健康检查配置
type HealthConfig struct {
	Listen string `yaml:"listen"` // 管理 API 监听地址，默认 :8080
}

// Load 加载配置，优先级：环境变量 → YAML 文件 → 默认值
func Load(path string) *Config {
	cfg := &Config{
		Sync: SyncConfig{
			Channel:     envOr("MAUCACHE_SYNC_CHANNEL", "Production"),
			Interval:    durationOr("MAUCACHE_SYNC_INTERVAL", 6*time.Hour),
			Concurrency: intOr("MAUCACHE_SYNC_CONCURRENCY", 4),
			RetryMax:    intOr("MAUCACHE_SYNC_RETRY_MAX", 3),
			RetryDelay:  durationOr("MAUCACHE_SYNC_RETRY_DELAY", 5*time.Second),
		},
		Storage: StorageConfig{
			CacheDir:   envOr("MAUCACHE_CACHE_DIR", "/data/maucache"),
			ScratchDir: envOr("MAUCACHE_SCRATCH_DIR", "/data/maucache/.tmp"),
		},
		Logging: LogConfig{
			Level:  envOr("MAUCACHE_LOG_LEVEL", "info"),
			Format: envOr("MAUCACHE_LOG_FORMAT", "json"),
		},
		Health: HealthConfig{
			Listen: envOr("MAUCACHE_HEALTH_LISTEN", ":8080"),
		},
	}

	// YAML 文件如果存在则覆盖环境变量的值
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(data, cfg)
		}
	}

	return cfg
}

// LogEffective 输出当前生效的配置（供启动时日志记录）
func (c *Config) LogEffective(cfgPath string) map[string]interface{} {
	source := "环境变量/默认值"
	if cfgPath != "" {
		source = "YAML 文件: " + cfgPath
	}
	return map[string]interface{}{
		"config_source": source,
		"channel":       c.Sync.Channel,
		"interval":      c.Sync.Interval.String(),
		"concurrency":   c.Sync.Concurrency,
		"retry_max":     c.Sync.RetryMax,
		"retry_delay":   c.Sync.RetryDelay.String(),
		"cache_dir":     c.Storage.CacheDir,
		"scratch_dir":   c.Storage.ScratchDir,
		"log_level":     c.Logging.Level,
		"log_format":    c.Logging.Format,
		"health_listen": c.Health.Listen,
	}
}

// envOr 读取环境变量，不存在则返回默认值
func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// intOr 读取环境变量并解析为 int，失败则返回默认值
func intOr(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

// durationOr 读取环境变量并解析为 time.Duration，失败则返回默认值
func durationOr(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
