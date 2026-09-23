package resolver

import "context"

// QRMaterializer 二维码物化缝(ADR-0002):核心 module 零第三方依赖,只定义接口;
// 固定选项——UTF-8、纠错 High、size=max(宽,1)、margin=0、roundBlock=None、
// 纯黑前景/纯白背景、PNG 输出——的实现放在 M2 渲染后端 module(go-qrcode),
// 使用者组装时经 WithQRMaterializer 接线
type QRMaterializer interface {
	// Materialize 按固定选项生成二维码 PNG 字节;width 为铺放宽度(正方形)
	Materialize(ctx context.Context, text string, width int) ([]byte, error)
}
