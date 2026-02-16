package sync

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"maucache/internal/cdn"
)

// SaveCollaterals 保存编录文件
// isProd=true: 保存到 cacheDir 根目录 + 带版本号的编录（对应 Save-MAUCollaterals -isProd $true）
// isProd=false: 保存到 cacheDir/collateral/{version}/（对应 Save-oldMAUCollaterals）
func SaveCollaterals(ctx context.Context, client *cdn.Client, apps []cdn.AppInfo, cacheDir string, isProd bool, log *slog.Logger) {
	mode := "当前版本编录"
	if !isProd {
		mode = "历史版本编录"
	}
	log.Info("开始保存编录文件", "mode", mode, "app_count", len(apps))

	totalSaved := 0
	totalFailed := 0
	for _, app := range apps {
		var targetDir string
		if isProd {
			targetDir = cacheDir
		} else {
			// 对应 Save-oldMAUCollaterals.ps1 第 20-26 行
			targetDir = filepath.Join(cacheDir, "collateral", app.Version)
		}
		if err := os.MkdirAll(targetDir, 0750); err != nil {
			log.Warn("创建编录目录失败", "path", targetDir, "error", err)
			continue
		}

		// 基础编录 URI
		// 对应 Save-oldMAUCollaterals.ps1 第 30 行
		uris := []string{
			app.CollateralURIs.AppXML,
			app.CollateralURIs.CAT,
			app.CollateralURIs.ChkXml,
		}

		// isProd 时额外保存带版本号的 cat 和 xml
		// 对应 Save-MAUCollaterals.ps1 第 47-56 行
		if isProd {
			versionedCat := cdn.BuildVersionedURI(app.CollateralURIs.CAT, app.Version, "")
			versionedXml := cdn.BuildVersionedURI(app.CollateralURIs.CAT, app.Version, ".xml")
			if versionedCat != "" {
				uris = append(uris, versionedCat)
			}
			if versionedXml != "" {
				uris = append(uris, versionedXml)
			}
		}

		appSaved := 0
		for _, uri := range uris {
			fileName := filepath.Base(uri)
			outPath := filepath.Join(targetDir, fileName)

			f, err := os.Create(outPath)
			if err != nil {
				log.Warn("创建文件失败", "path", outPath, "error", err)
				totalFailed++
				continue
			}

			lastMod, dlErr := client.Download(ctx, uri, f)
			closeErr := f.Close()

			if dlErr != nil {
				log.Warn("下载编录失败", "app", app.AppName, "file", fileName, "uri", uri, "error", dlErr)
				os.Remove(outPath)
				totalFailed++
				continue
			}
			if closeErr != nil {
				log.Warn("关闭文件失败", "path", outPath, "error", closeErr)
				totalFailed++
				continue
			}

			if !lastMod.IsZero() {
				_ = os.Chtimes(outPath, lastMod, lastMod)
			}
			appSaved++
		}

		totalSaved += appSaved
		log.Debug("编录保存完成", "app", app.AppName, "version", app.Version, "dir", targetDir, "saved", appSaved, "total_uris", len(uris))
	}

	log.Info("编录文件保存完成", "mode", mode, "saved", totalSaved, "failed", totalFailed)
}
