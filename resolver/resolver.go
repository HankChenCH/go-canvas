// Package resolver 资源物化:渲染前的统一准备阶段。
// 把图层里的远程引用(图片 URL/字体 URL)下载进本地缓存、把二维码内容经
// QRMaterializer 缝物化为本地 PNG,并将结果回写图层——图层构造与 setter
// 始终无 I/O,渲染端只见本地路径。
// 缓存默认在 os.UserCacheDir()/go-canvas/ 下图片/字体/二维码三个子目录,
// 键 = sha256(完整 URL) + 原扩展名,命中即跳过、无失效机制(语义与 PHP 一致)。
// 缓存布局不属 graph 互通契约面。
package resolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"

	"github.com/hankchen/go-canvas/canvas"
	"github.com/hankchen/go-canvas/layer"
)

// 绘制期错误稳定 code(spec §5.2,三端一致性抓手:消息可本地化,code 稳定;
// PHP 侧对位 MaterializeException.getErrorCode,errors.Is 判定,消息形态
// 「code + 上下文」与 PHP 一致,双端日志可互查;渲染控制面两 code 的语义权威
// 单列为 Runtime\CancellationSource/TimeoutCancellation,见各自条目)
var (
	// ErrResourceDownloadFailed 远程资源下载失败/空响应
	ErrResourceDownloadFailed = errors.New("resource_download_failed")
	// ErrResourceSaveFailed 物化产物落盘失败(含缓存目录不可写,PHP 同款归类)
	ErrResourceSaveFailed = errors.New("resource_save_failed")
	// ErrQRGenerateFailed QR 生成/落盘失败
	ErrQRGenerateFailed = errors.New("qr_generate_failed")
	// ErrRenderDeadlineExceeded 渲染截止时间已到(渲染期控制面,ADR 0008;
	// code 语义权威 = php Runtime\TimeoutCancellation,工票 render-cancellation 02)
	ErrRenderDeadlineExceeded = errors.New("render_deadline_exceeded")
	// ErrRenderCancelled 人为取消(渲染期控制面,ADR 0008;
	// code 语义权威 = php Runtime\CancellationSource,工票 render-cancellation 02)
	ErrRenderCancelled = errors.New("render_cancelled")
)

// ctxAttribution 渲染期 ctx 归因(工票 render-cancellation 03):错误链命中
// context.DeadlineExceeded / context.Canceled 之一即映射为对应渲染 code 哨兵
// (spec §5.2 三端锚点),原错误经 %w 保留在链上——errors.Is 对 context 两错误
// 与渲染两 code 同时成立;未命中返回 nil
func ctxAttribution(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: render deadline exceeded: %w", ErrRenderDeadlineExceeded, err)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%w: rendering cancelled: %w", ErrRenderCancelled, err)
	default:
		return nil
	}
}

// CheckContext 渲染期控制面检查点(ADR 0008):ctx 已到期/取消即报渲染 code。
// 渲染器模板在根层循环每图层前、表行循环每行前调用(与 php 四检查点同位,
// 工票 03);下载发起前不再单设检查点——ctx 已随 NewRequestWithContext 穿线,
// 到期/取消由 net/http 免费打断并经下载错误归因映射回渲染 code
func CheckContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		// ctx.Err() 按契约只返回 nil/DeadlineExceeded/Canceled,恒命中归因;
		// 异类实现原样上冒,不吞错
		if mapped := ctxAttribution(err); mapped != nil {
			return mapped
		}
		return err
	}
	return nil
}

// ErrQRMaterializerRequired 二维码物化缝未接线(ADR-0002):核心只定义接口,
// 固定选项实现在 M2 渲染后端 module,组装时经 WithQRMaterializer 注入。
// 遇二维码图层而未接线时报错而非静默跳过——静默会产出缺二维码的错误渲染
var ErrQRMaterializerRequired = errors.New("二维码图层物化需要注入 QRMaterializer(ADR-0002:实现在渲染后端 module)")

// ResourceResolver 资源物化器
type ResourceResolver struct {
	downloader Downloader
	qr         QRMaterializer

	// cacheRoot 缓存根目录;空串表示惰性取 os.UserCacheDir()/go-canvas
	cacheRoot string
}

// Option 物化器构造选项
type Option func(*ResourceResolver)

// New 构造物化器:默认 stdlib HTTP 下载器;二维码物化缝默认未接线
// (ADR-0002,遇二维码图层时报错);缓存根目录默认 os.UserCacheDir()/go-canvas
func New(opts ...Option) *ResourceResolver {
	r := &ResourceResolver{}
	for _, o := range opts {
		o(r)
	}
	if r.downloader == nil {
		r.downloader = &HTTPDownloader{}
	}
	return r
}

// WithDownloader 注入下载器(代理、鉴权、限流等自定义场景);nil 恢复默认
func WithDownloader(d Downloader) Option {
	return func(r *ResourceResolver) {
		if d == nil {
			d = &HTTPDownloader{}
		}
		r.downloader = d
	}
}

// WithQRMaterializer 接线二维码物化缝(固定选项实现在 M2 渲染后端 module,ADR-0002)
func WithQRMaterializer(m QRMaterializer) Option {
	return func(r *ResourceResolver) { r.qr = m }
}

// WithCacheRoot 注入缓存根目录(测试用临时目录等);空串恢复默认
func WithCacheRoot(dir string) Option {
	return func(r *ResourceResolver) { r.cacheRoot = dir }
}

// Resolve 物化画布全部图层(按 priority 序),首个错误即中止
func (r *ResourceResolver) Resolve(ctx context.Context, c *canvas.Canvas) error {
	for _, l := range c.GetLayers() {
		if err := r.ResolveLayer(ctx, l); err != nil {
			return err
		}
	}
	return nil
}

// ResolveLayer 物化单图层;容器递归下钻 表→行→单元格→内容层。
// 入口检查点(与 php resolveLayer 入口同位,工票 03):物化前掐断已取消/到期的
// ctx,渲染器惰性调用与全画布预遍历 API 两路径同覆盖。
// 未知/外部图层类型静默跳过(对齐 PHP match default 臂)
func (r *ResourceResolver) ResolveLayer(ctx context.Context, l layer.Layer) error {
	if err := CheckContext(ctx); err != nil {
		return err
	}
	switch t := l.(type) {
	case *layer.TableLayer:
		for _, row := range t.Rows() {
			if err := r.ResolveLayer(ctx, row); err != nil {
				return err
			}
		}
	case *layer.TableRowLayer:
		for _, cell := range t.Cells() {
			if err := r.ResolveLayer(ctx, cell); err != nil {
				return err
			}
		}
	case *layer.TableCellLayer:
		if content := t.ContentLayer(); content != nil {
			return r.ResolveLayer(ctx, content)
		}
	case *layer.ImageLayer:
		return r.materializeImage(ctx, t)
	case *layer.TextLayer:
		return r.materializeFont(ctx, t)
	case *layer.QrCodeLayer:
		return r.materializeQrCode(ctx, t)
	}
	return nil
}

// materializeImage 图片物化:仅 URL 才下载;本地路径直接使用、不回写
// (ResolvedSrc 回退原始引用保持可见)
func (r *ResourceResolver) materializeImage(ctx context.Context, l *layer.ImageLayer) error {
	if l.Image() == nil {
		return nil
	}
	src := *l.Image()
	if !isRemoteURL(src) {
		return nil
	}

	p, err := r.cachedRemoteFile(ctx, "img_layers", src)
	if err != nil {
		return err
	}
	l.SetResolvedSrc(p)
	return nil
}

// materializeFont 字体物化:空串/纯数字 = 渲染端内置默认字体语义,跳过;
// 本地路径直接使用;仅 URL 下载并回写(渲染实际字体 = 物化结果优先)
func (r *ResourceResolver) materializeFont(ctx context.Context, l *layer.TextLayer) error {
	font := l.Font()
	if font == "" || IsNumeric(font) || !isRemoteURL(font) {
		return nil
	}

	p, err := r.cachedRemoteFile(ctx, "text_layers", font)
	if err != nil {
		return err
	}
	l.SetResolvedFont(p)
	return nil
}

// materializeQrCode 二维码物化:文本为空或已有物化结果则跳过(幂等);
// PNG 字节经 QRMaterializer 缝生成,缓存键 = 内容+宽度 hash,命中不再生成
func (r *ResourceResolver) materializeQrCode(ctx context.Context, l *layer.QrCodeLayer) error {
	if l.Text() == "" || l.ResolvedSrc() != nil {
		return nil
	}
	if r.qr == nil {
		return ErrQRMaterializerRequired
	}

	dir, err := r.ensureCacheDir("qr_layers")
	if err != nil {
		return err
	}
	p := filepath.Join(dir, qrCacheKey(l.Text(), l.Width()))
	if !fileExists(p) {
		png, err := r.qr.Materialize(ctx, l.Text(), l.Width())
		if err != nil {
			return fmt.Errorf("%w: qr code generate failed(%s): %w", ErrQRGenerateFailed, l.Text(), err)
		}
		if err := os.WriteFile(p, png, 0o644); err != nil {
			return fmt.Errorf("%w: qr code save to tmp path failed: %s: %w", ErrQRGenerateFailed, p, err)
		}
	}
	l.SetResolvedSrc(p)
	return nil
}

// cachedRemoteFile 远程文件落缓存:命中即跳过(不再下载、不覆写);
// 下载失败报含 URL 的错误,落盘失败报错(消息对齐 PHP)
func (r *ResourceResolver) cachedRemoteFile(ctx context.Context, sub, rawURL string) (string, error) {
	dir, err := r.ensureCacheDir(sub)
	if err != nil {
		return "", err
	}

	p := filepath.Join(dir, cacheKey(rawURL))
	if fileExists(p) {
		return p, nil
	}

	content, err := r.downloader.Download(ctx, rawURL)
	if err != nil {
		// ctx 归因优先(工票 03):下载被打断于 ctx 到期/取消时报渲染 code
		// 而非资源 code——同一失败两种归因,渲染 code 是三端锚点;下载自身的
		// 流超时仍归 resource_download_failed(spec §5.2,不另立 code)
		if mapped := ctxAttribution(err); mapped != nil {
			return "", mapped
		}
		// 双 %w:哨兵供 errors.Is 判定,底层错误(取消/超时等)不丢失
		return "", fmt.Errorf("%w: could not get remote file(%s): %w", ErrResourceDownloadFailed, rawURL, err)
	}
	if len(content) == 0 {
		return "", fmt.Errorf("%w: could not get remote file(%s)", ErrResourceDownloadFailed, rawURL)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		return "", fmt.Errorf("%w: remote file(%s) save to tmp path failed: %w", ErrResourceSaveFailed, rawURL, err)
	}
	return p, nil
}

// ensureCacheDir 确保缓存子目录存在且可写,返回目录路径。
// 默认根目录每次现算不回写字段,Resolve 可安全并发
func (r *ResourceResolver) ensureCacheDir(sub string) (string, error) {
	root := r.cacheRoot
	if root == "" {
		userDir, err := os.UserCacheDir()
		if err != nil {
			return "", fmt.Errorf("%w: 定位系统缓存目录失败: %w", ErrResourceSaveFailed, err)
		}
		root = filepath.Join(userDir, "go-canvas")
	}

	dir := filepath.Join(root, sub)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("%w: tmp path can not writable: %s: %w", ErrResourceSaveFailed, dir, err)
	}
	return dir, nil
}

// cacheKey 图片/字体缓存键 = sha256(完整 URL) 十六进制 + URL path 原扩展名。
// 有意偏离 PHP 的 basename 键:不同 URL 的同名文件不再相互覆盖(旧债不继承),
// 键对查询串敏感;URL 解析失败时扩展名缺省
func cacheKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	ext := ""
	if u, err := url.Parse(rawURL); err == nil {
		ext = path.Ext(u.Path)
	}
	return hex.EncodeToString(sum[:]) + ext
}

// qrCacheKey 二维码缓存键 = sha256(内容_宽度) 十六进制 + ".png"
// (对齐 PHP md5(text_width) 思路,换 sha256 且对完整内容生效)
func qrCacheKey(text string, width int) string {
	sum := sha256.Sum256([]byte(text + "_" + strconv.Itoa(width)))
	return hex.EncodeToString(sum[:]) + ".png"
}

// isRemoteURL 是否远程 URL:协议无关,凡可解析出 scheme 与 host 即视为远程
// (对齐 PHP FILTER_VALIDATE_URL 的"须带协议与主机";支持的协议面由注入的
// Downloader 决定),其余一律按本地路径直接使用
func isRemoteURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// IsNumeric 纯数字判定:strconv.ParseFloat 近似 PHP is_numeric(整型/浮点字面量),
// 超集接受 Inf/NaN 等——Resolver 侧无观察差异(这类值也必然非 URL,两分支同为
// 跳过)。纯数字字体 id 是旧库 GD 内置字体编号,语义 = "无字体文件"→渲染端内置
// 默认字体:本函数是 Resolver 跳过字体物化与渲染后端回退默认字体的**同一口径**
// (渲染后端据此判定,导出以防两处判定漂移)
func IsNumeric(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// fileExists 缓存存在性检查;非 NotExist 错误视作未命中,交由后续落盘报错
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
