package cdn

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	gosync "sync"
)

// AppDef 对应 PowerShell Get-MAUApps.ps1 中 $targetApps 数组的每一项
type AppDef struct {
	AppID   string
	AppName string
}

// TargetApps 完整的 20 个应用列表
// 对应 Get-MAUApps.ps1 第 24-46 行
var TargetApps = []AppDef{
	{AppID: "0409MSau04", AppName: "MAU 4.x"},
	{AppID: "0409MSWD2019", AppName: "Word 365/2021/2019"},
	{AppID: "0409XCEL2019", AppName: "Excel 365/2021/2019"},
	{AppID: "0409PPT32019", AppName: "PowerPoint 365/2021/2019"},
	{AppID: "0409OPIM2019", AppName: "Outlook 365/2021/2019"},
	{AppID: "0409ONMC2019", AppName: "OneNote 365/2021/2019"},
	{AppID: "0409MSWD15", AppName: "Word 2016"},
	{AppID: "0409XCEL15", AppName: "Excel 2016"},
	{AppID: "0409PPT315", AppName: "PowerPoint 2016"},
	{AppID: "0409OPIM15", AppName: "Outlook 2016"},
	{AppID: "0409ONMC15", AppName: "OneNote 2016"},
	{AppID: "0409MSFB16", AppName: "Skype for Business"},
	{AppID: "0409IMCP01", AppName: "Intune Company Portal"},
	{AppID: "0409MSRD10", AppName: "Remote Desktop v10"},
	{AppID: "0409ONDR18", AppName: "OneDrive"},
	{AppID: "0409WDAV00", AppName: "Defender ATP"},
	{AppID: "0409EDGE01", AppName: "Edge"},
	{AppID: "0409TEAMS10", AppName: "Teams 1.0 classic"},
	{AppID: "0409TEAMS21", AppName: "Teams 2.1"},
	{AppID: "0409OLIC02", AppName: "Office Licensing Helper"},
}

// AppInfo 对应 PowerShell Get-MAUApp.ps1 返回的 PSCustomObject
type AppInfo struct {
	AppID   string
	AppName string
	Version string // 从 chk.xml 解析出来的版本号

	// 编录文件 URI
	CollateralURIs CollateralURIs

	// 从 AppID.xml 解析出的所有包的下载 URL
	PackageURIs []string

	// 从 AppID-history.xml 解析出的历史版本号
	HistoricVersions []string
	// 历史版本对应的包 URI（key=版本号）
	HistoricPackageURIs map[string][]string
}

// CollateralURIs 对应 PowerShell Get-MAUApp.ps1 中的 CollateralURIs 对象
type CollateralURIs struct {
	AppXML     string // {AppID}.xml
	CAT        string // {AppID}.cat
	ChkXml     string // {AppID}-chk.xml
	HistoryXML string // {AppID}-history.xml
}

// FetchAllApps 获取所有应用信息
// 对应 PowerShell: Get-MAUApps.ps1，但改为并发获取（原版是串行 ForEach-Object）
func (c *Client) FetchAllApps(ctx context.Context, channel string, log *slog.Logger) ([]AppInfo, error) {
	basePath, ok := channelPaths[channel]
	if !ok {
		return nil, fmt.Errorf("未知频道: %s", channel)
	}
	baseURL := cdnBase + basePath

	var (
		mu      gosync.Mutex
		results []AppInfo
		wg      gosync.WaitGroup
		errs    []error
	)

	// 并发获取每个应用的清单（PowerShell 原版是串行）
	sem := make(chan struct{}, 4) // 限制并发数
	for _, app := range TargetApps {
		wg.Add(1)
		go func(def AppDef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			info, err := c.fetchOneApp(ctx, def, baseURL, log)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", def.AppName, err))
				return
			}
			results = append(results, *info)
		}(app)
	}
	wg.Wait()

	if len(errs) > 0 {
		for _, e := range errs {
			log.Warn("获取应用信息失败", "error", e)
		}
	}

	return results, nil
}

// fetchOneApp 获取单个应用的完整信息
// 逐行对应 PowerShell: Get-MAUApp.ps1
func (c *Client) fetchOneApp(ctx context.Context, def AppDef, baseURL string, log *slog.Logger) (*AppInfo, error) {
	log.Info("处理应用", "appID", def.AppID, "appName", def.AppName)

	info := &AppInfo{
		AppID:   def.AppID,
		AppName: def.AppName,
		CollateralURIs: CollateralURIs{
			AppXML:     baseURL + def.AppID + ".xml",
			CAT:        baseURL + def.AppID + ".cat",
			ChkXml:     baseURL + def.AppID + "-chk.xml",
			HistoryXML: baseURL + def.AppID + "-history.xml",
		},
		HistoricPackageURIs: make(map[string][]string),
	}

	// 1. 获取 AppID.xml → 解析包列表
	// 对应 Get-MAUApp.ps1 第 37-43 行
	appXMLBody, err := c.GetString(ctx, info.CollateralURIs.AppXML)
	if err != nil {
		return nil, fmt.Errorf("获取 %s.xml 失败: %w", def.AppID, err)
	}
	packages, err := ParsePlistPackages(appXMLBody)
	if err != nil {
		return nil, fmt.Errorf("解析 %s.xml 失败: %w", def.AppID, err)
	}
	info.PackageURIs = packages.AllURIs()

	// 2. 获取 AppID-chk.xml → 解析版本号
	// 对应 Get-MAUApp.ps1 第 46-52 行
	chkBody, err := c.GetString(ctx, info.CollateralURIs.ChkXml)
	if err != nil {
		return nil, fmt.Errorf("获取 %s-chk.xml 失败: %w", def.AppID, err)
	}
	info.Version = ParsePlistVersion(chkBody)

	// 修复 version=99999 的情况
	// 对应 Get-MAUApp.ps1 第 55-59 行
	if info.Version == "99999" && len(packages.Versions) > 0 {
		info.Version = packages.Versions[0]
	}
	if info.Version == "99999" {
		info.Version = "Legacy"
	}

	// 3. 获取 AppID-history.xml（可选，404 正常）
	// 对应 Get-MAUApp.ps1 第 62-74 行
	histBody, err := c.GetStringOptional(ctx, info.CollateralURIs.HistoryXML)
	if err != nil {
		return nil, err
	}
	if histBody != "" {
		versions := ParsePlistStringArray(histBody)
		info.HistoricVersions = versions
		log.Debug("历史版本", "appID", def.AppID, "count", len(versions))

		// 获取每个历史版本的包列表
		for _, ver := range versions {
			histURL := baseURL + def.AppID + "_" + ver + ".xml"
			histXML, err := c.GetString(ctx, histURL)
			if err != nil {
				log.Warn("获取历史版本失败", "appID", def.AppID, "version", ver, "error", err)
				continue
			}
			histPkgs, _ := ParsePlistPackages(histXML)
			if histPkgs != nil {
				info.HistoricPackageURIs[ver] = histPkgs.AllURIs()
			}
		}
	}

	return info, nil
}

// ChannelBaseURL 返回频道的 CDN 基础 URL
func ChannelBaseURL(channel string) string {
	return cdnBase + channelPaths[channel]
}

// BuildVersionedURI 构建带版本号的编录 URI
// 对应 Save-MAUCollaterals.ps1 第 47-54 行的字符串操作
// 使用 url.Parse 替代手动字符串切割（修复 P9 URI 拼接问题）
func BuildVersionedURI(originalURI, version, newExt string) string {
	u, err := url.Parse(originalURI)
	if err != nil {
		return ""
	}
	segments := strings.Split(u.Path, "/")
	if len(segments) == 0 {
		return ""
	}
	lastSeg := segments[len(segments)-1]
	dotIdx := strings.LastIndex(lastSeg, ".")
	if dotIdx < 0 {
		return ""
	}
	baseName := lastSeg[:dotIdx]
	ext := newExt
	if ext == "" {
		ext = lastSeg[dotIdx:]
	}
	segments[len(segments)-1] = baseName + "_" + version + ext
	u.Path = strings.Join(segments, "/")
	return u.String()
}
