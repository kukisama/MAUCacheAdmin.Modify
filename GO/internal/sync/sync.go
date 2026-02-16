package sync

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"maucache/internal/cdn"
	"maucache/internal/config"
	"maucache/internal/health"
)

// Engine 同步引擎，编排整个同步流程
// 对应 PowerShell MacUpdatesOffice.Modify.ps1 主脚本
type Engine struct {
	cfg     *config.Config
	client  *cdn.Client
	log     *slog.Logger
	tracker *health.Tracker
}

// NewEngine 创建同步引擎
func NewEngine(cfg *config.Config, log *slog.Logger, tracker *health.Tracker) *Engine {
	return &Engine{
		cfg:     cfg,
		client:  cdn.NewClient(),
		log:     log,
		tracker: tracker,
	}
}

// RunOnce 执行一次完整同步
// 对应 MacUpdatesOffice.Modify.ps1 的完整流程
func (e *Engine) RunOnce(ctx context.Context) error {
	start := time.Now()
	e.tracker.SetRunning(true)
	defer e.tracker.SetRunning(false)

	e.log.Info("===== 开始同步 =====", "channel", e.cfg.Sync.Channel)

	// 步骤1: 清理旧文件
	// 对应 MacUpdatesOffice.Modify.ps1 第 15-36 行
	// 修复 P4：不递归删除 collateral 目录下的文件
	cleanCount := Cleanup(e.cfg.Storage.CacheDir, e.log)
	e.log.Info("清理完成", "deleted", cleanCount)

	// 步骤2: 获取构建版本
	// 对应 MacUpdatesOffice.Modify.ps1 第 45 行: $builds = Get-MAUProductionBuilds
	builds, err := e.client.FetchBuilds(ctx)
	if err != nil {
		return fmt.Errorf("获取 builds.txt 失败: %w", err)
	}
	e.log.Info("构建版本", "count", len(builds))

	// 步骤3: 获取所有应用信息
	// 对应 MacUpdatesOffice.Modify.ps1 第 48 行: $apps = Get-MAUApps -Channel Production
	apps, err := e.client.FetchAllApps(ctx, e.cfg.Sync.Channel, e.log)
	if err != nil {
		return fmt.Errorf("获取应用列表失败: %w", err)
	}
	e.log.Info("应用信息获取完成", "count", len(apps))

	// 步骤4: 保存编录文件
	// 对应 MacUpdatesOffice.Modify.ps1 第 51-52 行:
	//   Save-MAUCollaterals -MAUApps $apps -CachePath $maupath -isProd $true
	//   Save-oldMAUCollaterals -MAUApps $apps -CachePath $maupath
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, true, e.log)
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, false, e.log)

	// 步骤5-6: 生成下载计划
	// 对应 MacUpdatesOffice.Modify.ps1 第 55-56 行:
	//   $dlJobs = Get-MAUCacheDownloadJobs -MAUApps $_ -DeltaFromBuildLimiter $builds
	jobs, err := PlanDownloads(ctx, e.client, apps, builds, e.cfg.Storage.CacheDir, e.log)
	if err != nil {
		return fmt.Errorf("生成下载计划失败: %w", err)
	}
	e.log.Info("下载计划", "total_jobs", len(jobs))

	// 步骤7-8: 执行下载
	// 对应 MacUpdatesOffice.Modify.ps1 第 57 行:
	//   Invoke-MAUCacheDownload -MAUCacheDownloadJobs $dlJobs -CachePath $maupath -ScratchPath $mautemppath -Force
	result := ExecuteDownloads(ctx, e.client, jobs, e.cfg, e.log)

	elapsed := time.Since(start)
	e.tracker.RecordSync(result.Downloaded, result.Skipped, result.Failed, elapsed)
	e.log.Info("===== 同步完成 =====",
		"downloaded", result.Downloaded,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"duration", elapsed.Round(time.Second),
	)

	return nil
}

// RunLoop 定时循环执行同步
// 对应 PowerShell CreateScheduledTask.ps1 的计划任务功能
// 内建调度器，无需外部计划任务
func (e *Engine) RunLoop(ctx context.Context) {
	// 启动时立即执行一次
	if err := e.RunOnce(ctx); err != nil {
		e.log.Error("同步失败", "error", err)
	}

	ticker := time.NewTicker(e.cfg.Sync.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.log.Info("收到退出信号，停止同步")
			return
		case <-ticker.C:
			if err := e.RunOnce(ctx); err != nil {
				e.log.Error("同步失败", "error", err)
			}
		}
	}
}
