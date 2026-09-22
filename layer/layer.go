// Package layer 图层树纯结构节点:只承载"设定 + 纯布局数据",不渲染像素、不做 I/O。
// graph wire 面与 php-canvas-next 逐字段键级对齐,graph → 解码 → graph 往返恒等;
// 图层类型标识与 PHP 类名常量同名,同一份 graph JSON 双端互读。
package layer

import (
	"errors"
	"fmt"
)

// 图层类型标识(graph.type):与 PHP 类名常量同名,跨端固定。
// 文本/二维码/表格类型属工单 02/03,落地时在此追加。
const (
	// TypeImage 图片图层
	TypeImage = "ImageLayer"
)

// ErrUnknownLayerType graph 解码遇未知图层类型(消息含类型名)
var ErrUnknownLayerType = errors.New("未知图层类型")

// Layer 图层接口:画布容器与 graph 序列化的最小面向
type Layer interface {
	// TypeName 图层类型标识(graph.type)
	TypeName() string
	// Priority 绘制次序:越大越先渲染(视觉上越垫底)
	Priority() int
	// Graph 序列化为 wire 节点(无损)
	Graph() Node
}

// FromGraph 按 type 标识重建图层实例(对齐 PHP LayerFactory);未知类型报错
func FromGraph(n Node) (Layer, error) {
	switch n.Type {
	case TypeImage:
		return ImageFromGraph(n), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownLayerType, n.Type)
	}
}
