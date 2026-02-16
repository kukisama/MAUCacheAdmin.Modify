package sync

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// 废弃文件列表
// 对应 MacUpdatesOffice.Modify.ps1 第 15-23 行的 $filesToDelete
var deprecatedFiles = []string{
	"Lync Installer.pkg",
	"MicrosoftTeams.pkg",
	"Teams_osx.pkg",
	"wdav-upgrade.pkg",
}

// Cleanup 清理旧文件
// 对应 MacUpdatesOffice.Modify.ps1 第 15-36 行的文件删除逻辑
// 修复 P4：xml 和 cat 只删根目录，不递归进 collateral/ 子目录
// scratchDir 参数允许清理配置的临时目录
func Cleanup(cacheDir string, log *slog.Logger) int {
	count := 0

	// 1. 删除废弃的具名文件（在根目录下查找）
	for _, name := range deprecatedFiles {
		path := filepath.Join(cacheDir, name)
		if _, err := os.Stat(path); err == nil {
			log.Debug("删除废弃文件", "path", path)
			os.Remove(path)
			count++
		}
	}

	// 2. 删除根目录（仅根目录！）下的 xml / cat / builds.txt
	// 修复 P4：不递归，不会误删 collateral/ 下的文件
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return count
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext == ".xml" || ext == ".cat" || name == "builds.txt" {
			path := filepath.Join(cacheDir, name)
			log.Debug("删除根目录旧文件", "path", path)
			os.Remove(path)
			count++
		}
	}

	// 3. 清理 scratch 临时目录残留
	scratchDir := filepath.Join(cacheDir, ".tmp")
	os.RemoveAll(scratchDir)
	_ = os.MkdirAll(scratchDir, 0750)

	return count
}
