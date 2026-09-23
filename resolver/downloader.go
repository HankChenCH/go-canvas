package resolver

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// Downloader 远程资源下载器契约:注入自定义实现(代理、鉴权、限流等)
type Downloader interface {
	// Download 拉取远程资源字节;失败返回含 URL 的错误
	Download(ctx context.Context, rawURL string) ([]byte, error)
}

// HTTPDownloader 默认下载器:stdlib net/http 实现(对应 PHP file_get_contents
// 默认实现);非 2xx 响应与空响应体均视作失败(对应 PHP file_get_contents 的
// false 返回)
type HTTPDownloader struct {
	// Client 发请求用的 HTTP 客户端;nil 时用 http.DefaultClient,
	// 注入可定制代理/超时
	Client *http.Client
}

// Download implements Downloader
func (d *HTTPDownloader) Download(ctx context.Context, rawURL string) ([]byte, error) {
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造下载请求失败(%s): %w", rawURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("下载请求失败(%s): %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载响应非 2xx:HTTP %d(%s)", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取下载响应失败(%s): %w", rawURL, err)
	}
	return body, nil
}
