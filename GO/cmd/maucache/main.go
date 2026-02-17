package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"maucache/internal/config"
	"maucache/internal/health"
	"maucache/internal/logging"
	"maucache/internal/sync"
)

func main() {
	// CLI 参数
	// 对应 PowerShell MacUpdatesOffice.Modify.ps1 的 param 块
	once := flag.Bool("once", false, "执行一次同步后退出（不启动定时循环）")
	cfgPath := flag.String("config", "", "配置文件路径（可选，默认读环境变量）")
	flag.Parse()

	// 加载配置
	cfg := config.Load(*cfgPath)

	// 初始化日志
	log := logging.New(cfg.Logging.Level, cfg.Logging.Format)

	// 记录配置生效信息
	cfgInfo := cfg.LogEffective(*cfgPath)
	log.Info("配置加载完成",
		"config_source", cfgInfo["config_source"],
		"channel", cfgInfo["channel"],
		"interval", cfgInfo["interval"],
		"concurrency", cfgInfo["concurrency"],
		"retry_max", cfgInfo["retry_max"],
		"retry_delay", cfgInfo["retry_delay"],
		"cache_dir", cfgInfo["cache_dir"],
		"scratch_dir", cfgInfo["scratch_dir"],
		"log_level", cfgInfo["log_level"],
		"log_format", cfgInfo["log_format"],
		"health_listen", cfgInfo["health_listen"],
	)

	// 优雅退出
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// 启动 health API（后台 goroutine）
	statusTracker := health.NewTracker()
	go health.Serve(ctx, cfg.Health.Listen, statusTracker, log)

	// 创建同步引擎
	engine := sync.NewEngine(cfg, log, statusTracker)

	if *once {
		// 单次模式：跑一次就退出
		if err := engine.RunOnce(ctx); err != nil {
			log.Error("同步失败", "error", err)
			os.Exit(1)
		}
		return
	}

	// 定时循环模式
	engine.RunLoop(ctx)
}
