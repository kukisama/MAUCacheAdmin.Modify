package sync

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"maucache/internal/cdn"
	"maucache/internal/config"

	"golang.org/x/sync/errgroup"
)

// DownloadResult 下载结果统计
type DownloadResult struct {
	Downloaded int
	Skipped    int
	Failed     int
}

// ExecuteDownloads 并发下载所有需要更新的文件
// 对应 Invoke-MAUCacheDownload.ps1 第 57-112 行的 foreach 循环
// 改进：串行 → 并发，静默失败 → 重试+报错
// 修复 P1（异常静默吞噬）、P8（重试逻辑缺陷）
func ExecuteDownloads(ctx context.Context, client *cdn.Client, jobs []DownloadJob, cfg *config.Config, log *slog.Logger) DownloadResult {
	cacheDir := cfg.Storage.CacheDir
	scratchDir := cfg.Storage.ScratchDir

	// 确保目录存在
	_ = os.MkdirAll(cacheDir, 0750)
	_ = os.MkdirAll(scratchDir, 0750)

	var downloaded, skipped, failed atomic.Int64

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Sync.Concurrency) // 并发数限制

	for _, job := range jobs {
		if !job.NeedDownload {
			skipped.Add(1)
			log.Info("缓存有效，跳过",
				"app", job.AppName,
				"file", job.Payload,
			)
			continue
		}

		g.Go(func() error {
			err := downloadOneFile(gCtx, client, job, cacheDir, scratchDir, cfg.Sync.RetryMax, cfg.Sync.RetryDelay, log)
			if err != nil {
				failed.Add(1)
				// 不 return error，一个文件失败不阻塞其他下载
				log.Error("下载失败",
					"app", job.AppName,
					"file", job.Payload,
					"error", err,
				)
				return nil
			}
			downloaded.Add(1)
			return nil
		})
	}

	_ = g.Wait()

	return DownloadResult{
		Downloaded: int(downloaded.Load()),
		Skipped:    int(skipped.Load()),
		Failed:     int(failed.Load()),
	}
}

// downloadOneFile 下载单个文件，带重试和原子写入
// 对应 Invoke-MAUCacheDownload.ps1 第 88-108 行
// 修复 P1: 不再静默吞噬异常
// 修复 P8: 重试次数可配置 + 指数退避
func downloadOneFile(ctx context.Context, client *cdn.Client, job DownloadJob, cacheDir, scratchDir string, maxRetry int, retryDelay time.Duration, log *slog.Logger) error {
	targetPath := filepath.Join(cacheDir, job.Payload)
	scratchPath := filepath.Join(scratchDir, job.Payload)

	log.Info("开始下载",
		"app", job.AppName,
		"file", job.Payload,
		"size_mb", job.SizeBytes/1024/1024,
	)

	var lastErr error
	for attempt := 0; attempt < maxRetry; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * retryDelay
			log.Warn("重试下载",
				"file", job.Payload,
				"attempt", attempt+1,
				"backoff", backoff,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		lastErr = doDownload(ctx, client, job.LocationURI, scratchPath)
		if lastErr == nil {
			break
		}
	}
	if lastErr != nil {
		return fmt.Errorf("重试 %d 次后仍失败: %w", maxRetry, lastErr)
	}

	// 原子 rename：scratch → cache
	// 对应 Invoke-MAUCacheDownload.ps1 第 108 行: Move-Item
	if err := os.Rename(scratchPath, targetPath); err != nil {
		return fmt.Errorf("移动文件失败: %w", err)
	}

	// 设置 LastModified（对应 PowerShell -UseRemoteLastModified）
	if !job.LastMod.IsZero() {
		_ = os.Chtimes(targetPath, job.LastMod, job.LastMod)
	}

	log.Info("下载完成", "file", job.Payload)
	return nil
}

// doDownload 执行一次下载（写到 scratch 路径）
func doDownload(ctx context.Context, client *cdn.Client, uri, scratchPath string) error {
	f, err := os.Create(scratchPath)
	if err != nil {
		return err
	}

	_, dlErr := client.Download(ctx, uri, f)
	closeErr := f.Close()

	if dlErr != nil {
		os.Remove(scratchPath) // 删除不完整文件
		return dlErr
	}
	if closeErr != nil {
		os.Remove(scratchPath)
		return closeErr
	}
	return nil
}
