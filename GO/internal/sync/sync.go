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

	e.log.Info("===== 开始同步 =====",
		"channel", e.cfg.Sync.Channel,
		"cache_dir", e.cfg.Storage.CacheDir,
		"concurrency", e.cfg.Sync.Concurrency,
	)

	// 步骤1: 清理旧文件
	// 对应 MacUpdatesOffice.Modify.ps1 第 15-36 行
	// 修复 P4：不递归删除 collateral 目录下的文件
	cleanStart := time.Now()
	cleanCount := Cleanup(e.cfg.Storage.CacheDir, e.log)
	e.log.Info("步骤1: 清理完成", "deleted", cleanCount, "duration", time.Since(cleanStart).Round(time.Millisecond))

	// 步骤2: 获取构建版本
	// 对应 MacUpdatesOffice.Modify.ps1 第 45 行: $builds = Get-MAUProductionBuilds
	buildStart := time.Now()
	builds, err := e.client.FetchBuilds(ctx)
	if err != nil {
		return fmt.Errorf("获取 builds.txt 失败: %w", err)
	}
	e.log.Info("步骤2: 构建版本获取完成", "count", len(builds), "duration", time.Since(buildStart).Round(time.Millisecond))

	// 步骤3: 获取所有应用信息
	// 对应 MacUpdatesOffice.Modify.ps1 第 48 行: $apps = Get-MAUApps -Channel Production
	appStart := time.Now()
	apps, err := e.client.FetchAllApps(ctx, e.cfg.Sync.Channel, e.log)
	if err != nil {
		return fmt.Errorf("获取应用列表失败: %w", err)
	}
	e.log.Info("步骤3: 应用信息获取完成", "count", len(apps), "duration", time.Since(appStart).Round(time.Millisecond))

	// 步骤4: 保存编录文件
	// 对应 MacUpdatesOffice.Modify.ps1 第 51-52 行:
	//   Save-MAUCollaterals -MAUApps $apps -CachePath $maupath -isProd $true
	//   Save-oldMAUCollaterals -MAUApps $apps -CachePath $maupath
	collStart := time.Now()
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, true, e.log)
	SaveCollaterals(ctx, e.client, apps, e.cfg.Storage.CacheDir, false, e.log)
	e.log.Info("步骤4: 编录文件保存完成", "duration", time.Since(collStart).Round(time.Millisecond))

	// 步骤5-6: 生成下载计划
	// 对应 MacUpdatesOffice.Modify.ps1 第 55-56 行:
	//   $dlJobs = Get-MAUCacheDownloadJobs -MAUApps $_ -DeltaFromBuildLimiter $builds
	planStart := time.Now()
	jobs, err := PlanDownloads(ctx, e.client, apps, builds, e.cfg.Storage.CacheDir, e.log)
	if err != nil {
		return fmt.Errorf("生成下载计划失败: %w", err)
	}
	needDownload := 0
	var totalBytes int64
	for _, j := range jobs {
		if j.NeedDownload {
			needDownload++
			totalBytes += j.SizeBytes
		}
	}
	e.log.Info("步骤5-6: 下载计划生成完成",
		"total_jobs", len(jobs),
		"need_download", needDownload,
		"skip", len(jobs)-needDownload,
		"total_size_mb", fmt.Sprintf("%.2f", float64(totalBytes)/1024/1024),
		"duration", time.Since(planStart).Round(time.Millisecond),
	)

	// 步骤7-8: 执行下载
	// 对应 MacUpdatesOffice.Modify.ps1 第 57 行:
	//   Invoke-MAUCacheDownload -MAUCacheDownloadJobs $dlJobs -CachePath $maupath -ScratchPath $mautemppath -Force
	dlStart := time.Now()
	result := ExecuteDownloads(ctx, e.client, jobs, e.cfg, e.log)

	elapsed := time.Since(start)
	e.tracker.RecordSync(result.Downloaded, result.Skipped, result.Failed, elapsed)

	// 判断同步结果状态
	status := "成功"
	if result.Failed > 0 {
		status = "部分失败"
	}

	e.log.Info("===== 同步完成 =====",
		"status", status,
		"downloaded", result.Downloaded,
		"skipped", result.Skipped,
		"failed", result.Failed,
		"download_duration", time.Since(dlStart).Round(time.Second),
		"total_duration", elapsed.Round(time.Second),
	)

	if result.Failed > 0 {
		e.log.Warn("存在下载失败的文件，请检查日志中的错误信息", "failed_count", result.Failed)
	}

	return nil
}

// RunLoop 定时循环执行同步
// 对应 PowerShell CreateScheduledTask.ps1 的计划任务功能
// 内建调度器，无需外部计划任务
func (e *Engine) RunLoop(ctx context.Context) {
	e.log.Info("同步引擎启动，进入定时循环模式",
		"interval", e.cfg.Sync.Interval,
		"channel", e.cfg.Sync.Channel,
	)

	// 启动时立即执行一次
	if err := e.RunOnce(ctx); err != nil {
		e.log.Error("首次同步失败", "error", err)
	}

	ticker := time.NewTicker(e.cfg.Sync.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.log.Info("收到退出信号，停止同步引擎")
			return
		case t := <-ticker.C:
			e.log.Info("定时触发同步", "trigger_time", t.Format(time.RFC3339))
			if err := e.RunOnce(ctx); err != nil {
				e.log.Error("定时同步失败", "error", err)
			}
			e.log.Info("下次同步时间", "next", time.Now().Add(e.cfg.Sync.Interval).Format(time.RFC3339))
		}
	}
}
