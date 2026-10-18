// Package canvas 画布:纯结构容器(尺寸 + 按 priority 有序的图层),不含任何渲染状态。
// 渲染交给各后端,序列化产物(graph)与 php-canvas-next 键级互通。
package canvas

import (
	"sort"

	"github.com/HankChenCH/go-canvas/layer"
)

// Canvas 画布容器
type Canvas struct {
	width, height int

	// background 画布背景(ADR 0022):nil = 无背景(wire 整键不落);渲染语义字段,不属图层树
	background *CanvasBackground

	// layers 按 priority 降序(等优先级保持插入序)
	layers []layer.Layer
}

// New 构造画布并收纳图层(对齐 PHP Canvas::make)
func New(width, height int, layers ...layer.Layer) *Canvas {
	c := &Canvas{width: width, height: height}
	for _, l := range layers {
		c.AddLayer(l)
	}
	return c
}

// Width 画布宽
func (c *Canvas) Width() int { return c.width }

// Height 画布高
func (c *Canvas) Height() int { return c.height }

// SetBackground 声明画布背景(ADR 0022,对齐 PHP Canvas::setBackground):
// nil 或空背景(两分量皆空)与无背景同态——wire 整键不落
func (c *Canvas) SetBackground(background *CanvasBackground) {
	if background.IsEmpty() {
		background = nil
	}
	c.background = background
}

// Background 画布背景;无背景返回 nil
func (c *Canvas) Background() *CanvasBackground { return c.background }

// AddLayer 追加图层并保持 priority 降序稳定排序
// (对齐 PHP 8.0+ usort:b.priority <=> a.priority)
func (c *Canvas) AddLayer(l layer.Layer) {
	c.layers = append(c.layers, l)
	sort.SliceStable(c.layers, func(i, j int) bool {
		return c.layers[i].Priority() > c.layers[j].Priority()
	})
}

// GetLayers 按 priority 降序返回图层副本。
// 普通切片可重复消费——graph() 与多次渲染互不影响;外部改写副本不影响画布。
func (c *Canvas) GetLayers() []layer.Layer {
	out := make([]layer.Layer, len(c.layers))
	copy(out, c.layers)
	return out
}

// Graph 序列化为结构树(无损),可 JSON 持久化、跨渲染端/跨语言复用。
// canvas 段键序钉死 width, height, background(background 条件写键,ADR 0022)
func (c *Canvas) Graph() Graph {
	nodes := make([]layer.Node, 0, len(c.layers))
	for _, l := range c.layers {
		nodes = append(nodes, l.Graph())
	}
	return Graph{
		Canvas: CanvasSize{Width: c.width, Height: c.height, Background: c.background},
		Layers: nodes,
	}
}

// FromGraph 由 graph 结构重建画布(含全部图层与背景,复现插入即排序);未知图层类型报错。
// background 宽容读,空背景与无背景同态归一
func FromGraph(g Graph) (*Canvas, error) {
	background := g.Canvas.Background
	if background.IsEmpty() {
		background = nil
	}
	c := &Canvas{width: g.Canvas.Width, height: g.Canvas.Height, background: background}
	for _, n := range g.Layers {
		l, err := layer.FromGraph(n)
		if err != nil {
			return nil, err
		}
		c.AddLayer(l)
	}
	return c, nil
}

// Graph 画布的 wire 结构:{"canvas":{width,height,background?},"layers":[图层节点…]}
type Graph struct {
	Canvas CanvasSize   `json:"canvas"`
	Layers []layer.Node `json:"layers"`
}

// CanvasSize 画布尺寸;键序钉死 width, height, background(ADR 0022)
type CanvasSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
	// Background 条件写键:nil 整键不落;落键仅非空分量落键(见 CanvasBackground)
	Background *CanvasBackground `json:"background,omitempty"`
}
