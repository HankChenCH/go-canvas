// Package canvas 画布:纯结构容器(尺寸 + 按 priority 有序的图层),不含任何渲染状态。
// 渲染交给各后端,序列化产物(graph)与 php-canvas-next 键级互通。
package canvas

import (
	"sort"

	"github.com/hankchen/go-canvas/layer"
)

// Canvas 画布容器
type Canvas struct {
	width, height int

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

// Graph 序列化为结构树(无损),可 JSON 持久化、跨渲染端/跨语言复用
func (c *Canvas) Graph() Graph {
	nodes := make([]layer.Node, 0, len(c.layers))
	for _, l := range c.layers {
		nodes = append(nodes, l.Graph())
	}
	return Graph{
		Canvas: CanvasSize{Width: c.width, Height: c.height},
		Layers: nodes,
	}
}

// FromGraph 由 graph 结构重建画布(含全部图层,复现插入即排序);未知图层类型报错
func FromGraph(g Graph) (*Canvas, error) {
	c := &Canvas{width: g.Canvas.Width, height: g.Canvas.Height}
	for _, n := range g.Layers {
		l, err := layer.FromGraph(n)
		if err != nil {
			return nil, err
		}
		c.AddLayer(l)
	}
	return c, nil
}

// Graph 画布的 wire 结构:{"canvas":{width,height},"layers":[图层节点…]}
type Graph struct {
	Canvas CanvasSize   `json:"canvas"`
	Layers []layer.Node `json:"layers"`
}

// CanvasSize 画布尺寸
type CanvasSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
