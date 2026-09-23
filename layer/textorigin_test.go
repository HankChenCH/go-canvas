package layer_test

// TextOrigin 布局锚点用例(工单 02 新增,无 PHP 对应用例):
// PHP getTextOrigin 的 bottom 分支含 GD 基线魔数 - round(fontSize*0.1),
// Go 有意不移植(ADR-0003)——布局层返回纯对齐锚点,基线差由渲染后端 drawText
// 消化;本用例把该偏离锁定为规格。期望值按纯对齐公式手算。

import (
	"testing"

	"github.com/hankchen/go-canvas/layer"
)

func TestTextOriginPureAlignmentAnchor(t *testing.T) {
	tests := []struct {
		name string
		l    *layer.TextLayer
		x    int
		y    int
	}{
		{
			// left/bottom 单行:纯锚点 = 内容盒底(PHP 为 48,去 GD 魔数后 50,预期差异)
			name: "左下单行",
			l: layer.NewTextLayer(layer.WithSize(100, 50),
				layer.WithText("文本"), layer.WithFont("", 20, "#000")),
			x: 0, y: 50,
		},
		{
			// center/center:posx=100/2,posy=(50-20×0)/2
			name: "居中单行",
			l: layer.NewTextLayer(layer.WithSize(100, 50),
				layer.WithText("文本"), layer.WithFont("", 20, "#000"),
				layer.WithHorizontalAlign(layer.AlignCenter), layer.WithVerticalAlign(layer.AlignCenter)),
			x: 50, y: 25,
		},
		{
			// right/bottom 多行(字号 10、盒宽 50:'一二三四五六七' 断两行):
			// posx=内容盒宽,posy=50-10×(2-1)
			name: "右下两行",
			l: layer.NewTextLayer(layer.WithSize(50, 50),
				layer.WithText("一二三四五六七"), layer.WithFont("", 10, "#000"),
				layer.WithAutowrap(true),
				layer.WithHorizontalAlign(layer.AlignRight), layer.WithVerticalAlign(layer.AlignBottom)),
			x: 50, y: 40,
		},
		{
			// autoHeight + center:posy = 行高像素/2(PHP autoHeight 分支同式)
			name: "auto高居中",
			l: layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(),
				layer.WithText("文本"), layer.WithFont("", 20, "#000"),
				layer.WithVerticalAlign(layer.AlignCenter)),
			x: 0, y: 10,
		},
		{
			// autoHeight + padding + bottom×autowrap:内容盒高必须用动态高
			//(行数×行高像素+padding,Height() 覆写结果)而非原始 height 字段——
			// PHP getContentHeight 经 $this->getHeight() 动态分派。7 行、行高 23、
			// 动态高 169、内容盒高 161:posy = 161-23×(7-1) = 23
			//(工单10 布局快照 fixture 抓出的移植偏差)
			name: "auto高底部多行动态内容高",
			l: layer.NewTextLayer(layer.WithSize(100, 0), layer.WithAutoHeight(),
				layer.WithText("画布渲染库快照用例第一段超长文本\n\n第二段落继续自动断行"),
				layer.WithFont("", 16, "#000"), layer.WithLineHeight(1.4),
				layer.WithPadding(4), layer.WithAutowrap(true)),
			x: 0, y: 23,
		},
		{
			// 未知对齐值 → 0,0(PHP match default 臂)
			name: "未知对齐归零",
			l: layer.NewTextLayer(layer.WithSize(100, 50),
				layer.WithText("文本"), layer.WithFont("", 20, "#000"),
				layer.WithHorizontalAlign("up"), layer.WithVerticalAlign("sideways")),
			x: 0, y: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if x, y := tt.l.TextOrigin(); x != tt.x || y != tt.y {
				t.Errorf("TextOrigin() = (%d, %d), want (%d, %d)", x, y, tt.x, tt.y)
			}
		})
	}
}
