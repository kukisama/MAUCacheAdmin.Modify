package cdn

import (
	"context"
	"strings"
)

// 频道 GUID 映射
// 对应 PowerShell Get-MAUApps.ps1 第 17-21 行的 switch ($Channel) 块
var channelPaths = map[string]string{
	"Production": "/pr/C1297A47-86C4-4C1F-97FA-950631F94777/MacAutoupdate/",
	"Preview":    "/pr/1ac37578-5a24-40fb-892e-b89d85b6dfaa/MacAutoupdate/",
	"Beta":       "/pr/4B2D7701-0A4F-49C8-B4CB-0C2D4043F51F/MacAutoupdate/",
}

const cdnBase = "https://officecdnmac.microsoft.com"

// FetchBuilds 获取 builds.txt 并解析为版本号切片
// 对应 PowerShell: Get-MAUProductionBuilds.ps1
// 注意: 目前只有 Production 频道有 builds.txt
func (c *Client) FetchBuilds(ctx context.Context) ([]string, error) {
	url := cdnBase + channelPaths["Production"] + "builds.txt"
	body, err := c.GetString(ctx, url)
	if err != nil {
		return nil, err
	}

	// 按行分割，过滤空行
	// 对应 PowerShell: .Split([System.Environment]::NewLine) + FixLineBreaks 过滤器
	var builds []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			builds = append(builds, s)
		}
	}
	return builds, nil
}
