package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"maucache/internal/cdn"
)

// DownloadJob 对应 PowerShell Get-MAUCacheDownloadJobs.ps1 返回的 PSCustomObject
type DownloadJob struct {
	AppName      string
	LocationURI  string
	Payload      string // 文件名
	SizeBytes    int64
	LastMod      time.Time
	NeedDownload bool
}

// delta 包匹配模式：xxx_16.90.24121212_to_16.93.25011212_xxx.pkg
// 对应 Get-MAUCacheDownloadJobs.ps1 第 52 行: $pattern = '.*?([\d.]+)_to_([\d.]+).*'
var deltaPattern = regexp.MustCompile(`([\d.]+)_to_([\d.]+)`)

// PlanDownloads 生成下载计划
// 对应 Get-MAUCacheDownloadJobs.ps1 + Invoke-MAUCacheDownload.ps1 的缓存验证部分
func PlanDownloads(ctx context.Context, client *cdn.Client, apps []cdn.AppInfo, builds []string, cacheDir string, log *slog.Logger) ([]DownloadJob, error) {
	buildSet := make(map[string]bool)
	for _, b := range builds {
		buildSet[b] = true
	}

	var allJobs []DownloadJob

	for _, app := range apps {
		// 收集当前版本的包 URI
		// 对应 Get-MAUCacheDownloadJobs.ps1 第 34 行
		uris := uniqueStrings(app.PackageURIs)

		// 用 builds.txt 过滤 delta 包
		// 对应 Get-MAUCacheDownloadJobs.ps1 第 50-57 行
		var filtered []string
		for _, u := range uris {
			matches := deltaPattern.FindStringSubmatch(u)
			if matches == nil {
				// 不是 delta 包，保留
				filtered = append(filtered, u)
			} else {
				// 是 delta 包，只保留 from_version 在 builds.txt 中的
				fromVer := matches[1]
				if buildSet[fromVer] {
					filtered = append(filtered, u)
				}
			}
		}

		// 对每个 URI 发 HEAD 请求 + 比对本地缓存
		for _, uri := range filtered {
			payload := filepath.Base(uri)

			// HEAD 请求获取元信息
			// 对应 Get-MAUCacheDownloadJobs.ps1 第 68-72 行
			size, lastMod, err := client.Head(ctx, uri)
			if err != nil {
				log.Warn("HEAD 请求失败", "uri", uri, "error", err)
				continue
			}

			job := DownloadJob{
				AppName:     app.AppName,
				LocationURI: uri,
				Payload:     payload,
				SizeBytes:   size,
				LastMod:     lastMod,
			}

			// 缓存验证：文件存在 + 大小匹配
			// 对应 Invoke-MAUCacheDownload.ps1 第 71-86 行
			localPath := filepath.Join(cacheDir, payload)
			fi, err := os.Stat(localPath)
			if err != nil || fi.Size() != size {
				job.NeedDownload = true
			}

			allJobs = append(allJobs, job)
		}

		log.Info("计划完成", "app", app.AppName, "packages", len(filtered))
	}

	return allJobs, nil
}

// uniqueStrings 返回去重后的字符串切片
func uniqueStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
