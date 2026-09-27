// Package layer 图层树纯结构节点:只承载"设定 + 纯布局数据",不渲染像素、不做 I/O。
// graph wire 面与 php-canvas-next 逐字段键级对齐,graph → 解码 → graph 往返恒等;
// 图层类型标识与 PHP 类名常量同名,同一份 graph JSON 双端互读。
package layer

import (
	"errors"
	"fmt"
)

// 图层类型标识(graph.type):与 PHP 类名常量同名,跨端固定。
const (
	// TypeImage 图片图层
	TypeImage = "ImageLayer"
	// TypeText 文本图层
	TypeText = "TextLayer"
	// TypeQrCode 二维码图层
	TypeQrCode = "QrCodeLayer"
	// TypeTable 表格图层(行容器)
	TypeTable = "TableLayer"
	// TypeTableRow 表格行图层(单元格容器)
	TypeTableRow = "TableRowLayer"
	// TypeTableCell 表格单元格图层(内容层包装)
	TypeTableCell = "TableCellLayer"
	// TypeTableRowTemplate 表格行模板(TableLayer V2,spec §2):仅合法出现在
	// TableLayer 的 template 字段内;出现在画布图层序列/rows/cells/content 属
	// 非法 wire,工厂虽能解码,契约禁止(对齐 PHP 不拒绝)
	TypeTableRowTemplate = "TableRowTemplate"
)

// 解码期错误稳定 code(spec §5.2,三端一致性抓手:消息可本地化,code 稳定;
// PHP 侧对位 DecodeException.getErrorCode,errors.Is 判定)
var (
	// ErrUnknownLayerType graph 解码遇未知图层类型(消息含类型名)
	ErrUnknownLayerType = errors.New("unknown_layer_type")
	// ErrTemplateRowsConflict template ⊕ rows 双键同现(spec §2.2 XOR)
	ErrTemplateRowsConflict = errors.New("template_rows_conflict: template 与 rows 键不得同时出现")
	// ErrRowsPathMissing 模板态缺 data.rowsPath(spec §3.3,先紧后松——补默认值是
	// 非破坏性变更,反向是破坏性的)
	ErrRowsPathMissing = errors.New("rows_path_missing: 模板态表格缺少 data.rowsPath")
)

// Layer 图层接口:画布容器与 graph 序列化的最小面向
type Layer interface {
	// TypeName 图层类型标识(graph.type)
	TypeName() string
	// Priority 绘制次序:越大越先渲染(视觉上越垫底)
	Priority() int
	// Visible 显隐设定(layer-panel-ux 工单 01):false 的根图层被渲染循环跳过
	// (隐藏 = 最终输出排除)
	Visible() bool
	// Graph 序列化为 wire 节点(无损)
	Graph() Node
}

// FromGraph 按 type 标识重建图层实例(对齐 PHP LayerFactory);未知类型报错
func FromGraph(n Node) (Layer, error) {
	switch n.Type {
	case TypeImage:
		return ImageFromGraph(n), nil
	case TypeText:
		return TextFromGraph(n), nil
	case TypeQrCode:
		return QrCodeFromGraph(n), nil
	case TypeTable:
		return TableFromGraph(n)
	case TypeTableRow:
		return TableRowFromGraph(n)
	case TypeTableCell:
		return TableCellFromGraph(n)
	case TypeTableRowTemplate:
		l, err := TableRowTemplateFromGraph(n)
		if err != nil {
			return nil, err
		}
		return l, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownLayerType, n.Type)
	}
}
