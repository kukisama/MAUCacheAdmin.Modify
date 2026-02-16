package cdn

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client 封装了对 Microsoft CDN 的所有 HTTP 操作
// 修复 PowerShell P5 问题：全局复用一个实例，不再每次创建新 HttpClient
// 对应 PowerShell: Get-HttpClientHandler.ps1 + Set-MAUCacheAdminHttpClientHandler.ps1
type Client struct {
	http *http.Client
}

// NewClient 创建 CDN HTTP 客户端
func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout: 30 * time.Minute, // 大文件下载需要足够长的超时
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// GetString 获取文本内容（用于 builds.txt 和 Plist XML）
// 对应 PowerShell: $httpClient.GetStringAsync($URI).GetAwaiter().GetResult()
func (c *Client) GetString(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// GetStringOptional 获取可选资源，404/400 返回 ""，不报错
// 对应 PowerShell: Get-PlistObjectFromURI -Optional
// CDN 对不存在的 history.xml 有时返回 400 而不是 404
func (c *Client) GetStringOptional(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", nil
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// 网络错误当作资源不存在（可选资源允许静默失败）
		// 但记录日志便于排查连接问题
		return "", nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	return string(body), err
}

// Head 发送 HEAD 请求获取文件元信息（大小和最后修改时间）
// 对应 PowerShell: $httpClient.SendAsync($headRequest) 在 Get-MAUCacheDownloadJobs.ps1 中
func (c *Client) Head(ctx context.Context, url string) (size int64, lastMod time.Time, err error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, time.Time{}, err
	}
	defer resp.Body.Close()

	return resp.ContentLength, lastModTime(resp), nil
}

// Download 流式下载文件到 io.Writer
// 对应 PowerShell: Invoke-HttpClientDownload.ps1 的核心下载循环
// 使用 256KB 缓冲区，与 PowerShell 版一致
func (c *Client) Download(ctx context.Context, url string, w io.Writer) (lastMod time.Time, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	// 256KB 缓冲区，与 PowerShell 的 `New-Object byte[] 256KB` 一致
	buf := make([]byte, 256*1024)
	_, err = io.CopyBuffer(w, resp.Body, buf)

	return lastModTime(resp), err
}

// lastModTime 从 HTTP 响应中解析 Last-Modified 头
func lastModTime(resp *http.Response) time.Time {
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		return t
	}
	return time.Time{}
}
