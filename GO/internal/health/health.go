package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	gosync "sync"
	"time"
)

// Tracker 追踪同步状态，用于 /healthz 和 /sync/status 端点
// PowerShell 版没有此功能，新增用于 Docker 健康检查和运维监控
type Tracker struct {
	mu         gosync.RWMutex
	running    bool
	lastSync   time.Time
	downloaded int
	skipped    int
	failed     int
	duration   time.Duration
}

// NewTracker 创建状态追踪器
func NewTracker() *Tracker { return &Tracker{} }

// SetRunning 设置同步运行状态
func (t *Tracker) SetRunning(v bool) {
	t.mu.Lock()
	t.running = v
	t.mu.Unlock()
}

// RecordSync 记录同步结果
func (t *Tracker) RecordSync(downloaded, skipped, failed int, dur time.Duration) {
	t.mu.Lock()
	t.lastSync = time.Now()
	t.downloaded = downloaded
	t.skipped = skipped
	t.failed = failed
	t.duration = dur
	t.mu.Unlock()
}

// Serve 启动健康检查 HTTP 服务
func Serve(ctx context.Context, addr string, t *Tracker, log *slog.Logger) {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/sync/status", func(w http.ResponseWriter, r *http.Request) {
		t.mu.RLock()
		defer t.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"running":    t.running,
			"last_sync":  t.lastSync,
			"downloaded": t.downloaded,
			"skipped":    t.skipped,
			"failed":     t.failed,
			"duration":   t.duration.String(),
		})
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Info("Health API 启动", "addr", addr)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Error("Health API 异常退出", "error", err)
	}
}
