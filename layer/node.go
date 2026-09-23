package layer

import "encoding/json"

// graph wire 结构:与 php-canvas-next 各图层 graph() 产物逐字段键级对齐。
// 结构体字段顺序即 JSON 键序(对齐 PHP 关联数组字面量的写出顺序)。

// ValueTypeStatic data.valueType 的静态取值标识
const ValueTypeStatic = "StaticValue"

// Node 图层在 graph 中的 wire 节点。类型专属键仅由对应图层类型写出:
// data 属图片/文本/二维码图层,rows/cells/content 属对应表格容器——
// 非 nil 才写出(空容器也恒写键,见各容器 Graph);Content 以 RawMessage
// 承载以区分"键存在为 null"(空单元格)与"键缺省"(非容器)。
type Node struct {
	Type     string          `json:"type"`
	Priority int             `json:"priority"`
	Spec     Spec            `json:"spec"`
	Data     *Data           `json:"data,omitempty"`
	Rows     *[]Node         `json:"rows,omitempty"`
	Cells    *[]Node         `json:"cells,omitempty"`
	Content  json.RawMessage `json:"content,omitempty"`
}

// Spec 图层公共规格:形状 / 对齐 / 定位;文本图层追加 fontFamily 键
// (PHP TextLayer::graph() 在父类 spec 之后写入,键序 shape/align/position/fontFamily)
type Spec struct {
	Shape      Shape       `json:"shape"`
	Align      Align       `json:"align"`
	Position   Position    `json:"position"`
	FontFamily *FontFamily `json:"fontFamily,omitempty"`
}

// FontFamily 文本字体规格(wire 面仅文本图层携带)。
// font 保留完整原始值(路径/URL),与 PHP 一致不做 basename 截断
type FontFamily struct {
	Font      string `json:"font"`
	FontSize  int    `json:"fontSize"`
	FontColor string `json:"fontColor"`
	Angle     int    `json:"angle"`
	Autowrap  bool   `json:"autowrap"`
}

// Shape 盒模型:尺寸、auto 标志、行高倍数、内边距、边框、背景色
type Shape struct {
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	AutoWidth       bool    `json:"autoWidth"`
	AutoHeight      bool    `json:"autoHeight"`
	LineHeight      float64 `json:"lineHeight"`
	Padding         Padding `json:"padding"`
	Border          Border  `json:"border"`
	BackgroundColor *string `json:"backgroundColor"`
}

// Padding 内边距,恒 4 键,键序对齐 PHP 数组序(top/bottom/left/right)
type Padding struct {
	Top    float64 `json:"top"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
	Right  float64 `json:"right"`
}

// Border 四边边框,恒 4 键;未设的边为 null
type Border struct {
	Top    *BorderSide `json:"top"`
	Bottom *BorderSide `json:"bottom"`
	Left   *BorderSide `json:"left"`
	Right  *BorderSide `json:"right"`
}

// BorderSide 单边边框
type BorderSide struct {
	Width int    `json:"width"`
	Color string `json:"color"`
}

// Align 内容对齐:horizontal 取 left/center/right,vertical 取 top/center/bottom
type Align struct {
	Horizontal string `json:"horizontal"`
	Vertical   string `json:"vertical"`
}

// Position 定位:九锚点 + 父盒内偏移(不做边界钳位)
type Position struct {
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Position string `json:"position"`
}

// Data 图层业务数据。图片/二维码图层仅 valueType/value 两键——无 expression 预留键,
// 与 PHP 字节面一致;文本图层恒写 expression 空串占位(表达式引擎不存在,
// 仅保留 wire 字段,PHP 同款)
type Data struct {
	ValueType  string  `json:"valueType"`
	Expression *string `json:"expression,omitempty"`
	Value      *string `json:"value"`
}
