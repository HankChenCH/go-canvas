package paginate

// 流链校验(PHP FlowChainValidator 逐规则镜像,spec §10.2,ADR 0009,工票 09):
// fail-fast 于文档编译入口、先于全量 hydrate(spec §10.3 时序步骤 0),也供
// 模板保存期预检(example/server,03 票 §7——保存时打回,渲染期只防御性复跑)。
//
// 违反归类(稳定 code 是三端一致性锚点,spec §10.4:sentinel + %w 包装、
// errors.Is 判定,消息可改、code 不可改):
//   - flow_chain_invalid:节点键封闭、frame 索引/重复、mode/quota/omitIfEmpty
//     形态、链序一致递增、paged 须在链尾、链上帧须有模板态顶层表;
//   - flow_rows_path_inconsistent:链内各帧 rowsPath 一致 + 链外帧顶层表不得
//     与链同 rowsPath(同路径的表游离链外 = 分配权威被架空的幽灵行,不限定模板态);
//   - paginate_target_invalid:任意帧 ≥2 张顶层 TableLayer 原样抛(分页段 code
//     不换段——「恰有 1 张」的 2 表特例由它接管,链上链外一体适用)。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/layer"
)

// 流链节点 mode 取值(PHP MODE_FIXED/MODE_PAGED 私有常量同款,包内共用)
const (
	modeFixed = "fixed"
	modePaged = "paged"
)

// flowChainNodeKeys 流链节点封闭键面(spec §10.2:节点是可序列化声明数据,
// 过 wire、进 fixture、被三端镜像;PHP NODE_KEYS 同款)
var flowChainNodeKeys = map[string]struct{}{
	"frame": {}, "mode": {}, "quota": {}, "omitIfEmpty": {},
}

// tableSlot 帧的顶层槽位表:index = 图层序列下标(与 Graph().Layers 同序,
// 编译器产物回写定位用),table = 表实例
type tableSlot struct {
	index int
	table *layer.TableLayer
}

// ValidateFlowChain 流链校验(PHP FlowChainValidator::validate 逐条镜像):
// 对模板态声明帧序列 + 流链节点做入口裁决。frames 为声明态(校验不求值不填充);
// chain 节点经 wire 解码(宽松留存原始键面,裁决走 raw)或程序化构造
// (raw=nil,按「恰好类型化键在场」裁决——工票 08 约定)。
//
// 返回 frame 下标 => 该帧顶层槽位表在图层序列中的下标(编译器回写定位用);
// 校验失败返回 nil 与包装稳定 code 的错误。
func ValidateFlowChain(frames []*canvas.Canvas, chain []FlowChainNode) (map[int]int, error) {
	frameCount := len(frames)

	// 节点级规则逐节点裁决,段内次序对齐 PHP:键封闭 → frame → 重复 →
	// mode → quota → omitIfEmpty → 链序
	seenFrames := map[int]struct{}{}
	previousFrame := -1 // 链上 frame 恒 ≥0(负数先行拒绝),-1 = 尚无前驱
	for i := range chain {
		node := &chain[i]

		// 键封闭:未知键拒绝(封闭数据通道,CompileContext.options 同款)。
		// raw 是无序 map,按字典序报首个未知键,保证失败消息确定
		if node.raw != nil {
			keys := make([]string, 0, len(node.raw))
			for k := range node.raw {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if _, known := flowChainNodeKeys[k]; !known {
					return nil, fmt.Errorf("%w: 未知键 %s(封闭键面 frame/mode/quota/omitIfEmpty)", ErrFlowChainInvalid, k)
				}
			}
		}

		// frame:整数字面量(PHP is_int,"x"/1.5/null 均拒)且 0 ≤ idx < 帧数
		frame, ok := nodeFrame(node)
		if !ok || frame < 0 || frame >= frameCount {
			return nil, fmt.Errorf("%w: frame 索引越界或非整数(%s,帧数 %d)", ErrFlowChainInvalid, nodeKeyRepr(node, "frame"), frameCount)
		}
		if _, dup := seenFrames[frame]; dup {
			return nil, fmt.Errorf("%w: frame %d 在链上重复", ErrFlowChainInvalid, frame)
		}
		seenFrames[frame] = struct{}{}

		// mode:fixed|paged 二选一(宽松解码把非字符串形态落成零值,同样拒)
		if node.Mode != modeFixed && node.Mode != modePaged {
			return nil, fmt.Errorf("%w: mode 须为 fixed|paged,得到 %q", ErrFlowChainInvalid, node.Mode)
		}

		// quota:仅 fixed 合法且允许 0(non-negative-int,显式不吃行的声明位);
		// 键在场即裁决(PHP array_key_exists——quota:null 同样算携带)
		if nodeQuotaPresent(node) {
			if node.Mode == modePaged {
				return nil, fmt.Errorf("%w: paged 节点不得携带 quota", ErrFlowChainInvalid)
			}
			if !nodeQuotaNonNegative(node) {
				return nil, fmt.Errorf("%w: quota 须为非负整数,得到 %s", ErrFlowChainInvalid, nodeKeyRepr(node, "quota"))
			}
		}

		// omitIfEmpty:布尔形态(类型化零值不可区分缺席与 false,走原始键面裁决)
		if !nodeOmitIfEmptyOK(node) {
			return nil, fmt.Errorf("%w: omitIfEmpty 须为布尔,得到 %s", ErrFlowChainInvalid, nodeKeyRepr(node, "omitIfEmpty"))
		}

		// 链序:按帧序一致递增(可跳过无表帧,不可乱序回跳)
		if previousFrame >= 0 && frame <= previousFrame {
			return nil, fmt.Errorf("%w: 链序须按帧序一致递增(%d → %d 乱序回跳)", ErrFlowChainInvalid, previousFrame, frame)
		}
		previousFrame = frame
	}

	// paged 至多一个且须在链尾(链尾判定同时封住双 paged)
	last := len(chain) - 1
	for i := range chain {
		if chain[i].Mode == modePaged && i != last {
			return nil, fmt.Errorf("%w: paged 节点须在链尾(节点 %d/%d)", ErrFlowChainInvalid, i, last)
		}
	}

	// 任意帧 ≥2 顶层表 → paginate_target_invalid 原样抛(先于「恰有 1 张」的
	// 链上帧裁决:2 表特例由本规则接管,链上链外一体适用)
	for frameIndex, frame := range frames {
		if tables := topLevelTables(frame); len(tables) >= 2 {
			return nil, fmt.Errorf("%w: 分页要求画布上唯一顶层 TableLayer,帧 %d 实际 %d 张", ErrPaginateTargetInvalid, frameIndex, len(tables))
		}
	}

	// 链上帧恰有 1 张顶层模板表,取其 rowsPath 对齐全链(链内一致性)
	tableIndexByFrame := make(map[int]int, len(chain))
	chainRowsPath := ""
	chainRowsPathSet := false // PHP $chainRowsPath null 哨兵同构:空串也是合法链路径,须与「未定」区分
	for i := range chain {
		frameIndex := chain[i].Frame
		tables := topLevelTables(frames[frameIndex])
		if len(tables) == 0 {
			return nil, fmt.Errorf("%w: 链上帧 %d 无顶层 TableLayer(须恰有 1 张模板态槽位表)", ErrFlowChainInvalid, frameIndex)
		}
		slot := tables[0] // ≥2 张已被 paginate_target_invalid 拦下,此处至多 1 张
		if slot.table.Template() == nil {
			return nil, fmt.Errorf("%w: 链上帧 %d 的顶层表非模板态(链槽位表须为模板态)", ErrFlowChainInvalid, frameIndex)
		}

		rowsPath := slot.table.RowsPath()
		if chainRowsPathSet && rowsPath != chainRowsPath {
			return nil, fmt.Errorf("%w: 链内 rowsPath 不一致(%s vs %s,帧 %d)", ErrFlowRowsPathInconsistent, chainRowsPath, rowsPath, frameIndex)
		}
		if !chainRowsPathSet {
			chainRowsPath, chainRowsPathSet = rowsPath, true
		}
		tableIndexByFrame[frameIndex] = slot.index
	}

	// rowsPath 链外碰撞:链外帧顶层表与链同 rowsPath → 幽灵行,拒绝
	// (不限定模板态,V1 表被 setRowsPath 标注同路径同样拒绝)
	if chainRowsPathSet {
		for frameIndex, frame := range frames {
			if _, onChain := tableIndexByFrame[frameIndex]; onChain {
				continue
			}
			for _, slot := range topLevelTables(frame) {
				if slot.table.RowsPath() == chainRowsPath {
					return nil, fmt.Errorf("%w: 链外帧 %d 顶层表与链同 rowsPath(%s),分配权威被架空", ErrFlowRowsPathInconsistent, frameIndex, chainRowsPath)
				}
			}
		}
	}

	return tableIndexByFrame, nil
}

// nodeFrame 节点 frame 值裁决:wire 解码节点(raw 在场)按整数字面量判定——
// 宽松解码会把 "x"/1.5/null 落成零值,类型化字段不可作依据;程序化构造
// (raw=nil)按「恰好类型化键在场」裁决,frame 恒整数(工票 08 约定)
func nodeFrame(node *FlowChainNode) (int, bool) {
	if node.raw == nil {
		return node.Frame, true
	}
	raw, present := node.raw["frame"]
	if !present {
		return 0, false
	}
	return jsonIntLiteral(raw)
}

// nodeQuotaPresent quota 键在场裁决:raw 键面 = 键存在即携带(含 null 字面量,
// PHP array_key_exists 同判);程序化构造 = Quota 指针非 nil 即携带
func nodeQuotaPresent(node *FlowChainNode) bool {
	if node.raw == nil {
		return node.Quota != nil
	}
	_, present := node.raw["quota"]
	return present
}

// nodeQuotaNonNegative quota 值裁决:整数字面量且 ≥0(wire 路径形态非法与负值
// 同拒;程序化构造恒整数,只判负值)
func nodeQuotaNonNegative(node *FlowChainNode) bool {
	if node.raw == nil {
		return *node.Quota >= 0
	}
	quota, ok := jsonIntLiteral(node.raw["quota"])
	return ok && quota >= 0
}

// nodeOmitIfEmptyOK omitIfEmpty 形态裁决:bool 字面量(wire 路径键在场即须为
// 布尔,PHP is_bool 同判;程序化构造恒布尔,无需裁决)
func nodeOmitIfEmptyOK(node *FlowChainNode) bool {
	if node.raw == nil {
		return true
	}
	raw, present := node.raw["omitIfEmpty"]
	if !present {
		return true
	}
	_, ok := jsonBoolLiteral(raw)
	return ok
}

// jsonIntLiteral 原始 JSON 载荷是否为整数字面量(PHP json_decode 后 is_int 同判:
// 字符串/布尔/null/浮点字面量("1.0"/"1e2")均非整数,溢出 int64 的长数字同拒)
func jsonIntLiteral(raw json.RawMessage) (int, bool) {
	v, ok := jsonLiteral(raw)
	if !ok {
		return 0, false
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil {
		return 0, false
	}
	return int(i), true
}

// jsonBoolLiteral 原始 JSON 载荷是否为布尔字面量
func jsonBoolLiteral(raw json.RawMessage) (bool, bool) {
	v, ok := jsonLiteral(raw)
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// jsonLiteral 单值解码(UseNumber 保数字字面量原形,不折算 float64)
func jsonLiteral(raw json.RawMessage) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// nodeKeyRepr 节点键失败消息里的原值形态(wire 路径回显原始键面字节,程序化构造
// 回显类型化值;消息非契约,只求可定位)
func nodeKeyRepr(node *FlowChainNode, key string) string {
	if node.raw != nil {
		if raw, ok := node.raw[key]; ok {
			return string(raw)
		}
		return "null"
	}
	switch key {
	case "frame":
		return strconv.Itoa(node.Frame)
	case "quota":
		if node.Quota != nil {
			return strconv.Itoa(*node.Quota)
		}
	}
	return "null"
}

// topLevelTables 帧的顶层表枚举(图层序 = GetLayers 序,priority 降序稳定序,
// 与 PHP FlowChainValidator::topLevelTables 同构)
func topLevelTables(frame *canvas.Canvas) []tableSlot {
	var tables []tableSlot
	for index, l := range frame.GetLayers() {
		if table, ok := l.(*layer.TableLayer); ok {
			tables = append(tables, tableSlot{index: index, table: table})
		}
	}
	return tables
}
