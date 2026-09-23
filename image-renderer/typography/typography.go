// Package typography 增强文本实现:真实字体度量、完整 UAX #14 断行、
// UAX #29 字素切分——三者均非默认,经核心 module 的既有接缝注入(ADR-0003)。
//
// 默认策略是对齐 PHP 的核心 text 包实现(启发式度量 + 照搬版贪心断行器 +
// 码点切分),保证开箱即双端一致;追求排版质量时换用本包并逐组件注入:
//
//	l := layer.NewTextLayer(
//	    // 度量:opentype Face 真实宽度(advance+kern),经工厂接缝
//	    layer.WithMeasurerFactory(typography.OpenTypeMeasurerFactory),
//	    // 断行:typesetting LineIterator 全量断行机会 + 贪心定行
//	    layer.WithLineBreaker(typography.NewUax14LineBreaker()),
//	)
//	// 字素:UAX #29 簇切分既可注入增强断行器之外的独立场景,
//	// 也用于补齐核心断行器的码点降级缺口
//	coreBreaker := text.NewUax14LineBreaker().WithSegmenter(typography.NewGraphemeSegmenter())
//
// 三个组件相互独立,可与核心实现任意组合(见 inject_test.go)。
//
// # 与 PHP 对齐版的取舍
//
// 增强实现的断行行数/行内容可能与对齐版不同:词内不硬断、数值串保持、
// CR LF/LS/结尾换行语义等。差异在 linebreak_test.go 用例表中逐条标注原因;
// 需要双端行级一致时保持默认实现(ADR-0003)。
//
// # 并发注意
//
//   - OpenTypeMeasurerFactory 返回的度量器持有 opentype.Face,非并发安全,
//     限单 goroutine 串行使用;工厂每次调用新建实例,跨 goroutine 各自获取。
//   - Uax14LineBreaker 与 GraphemeSegmenter 零状态(每次调用用局部 Segmenter),
//     值可共享,并发调用安全。
//   - 字体解析结果(*opentype.Font,并发安全)进程内缓存共享,Face 每次新建。
//
// 选型证据:工作区 docs/golang-port-research.md §4.3
// (x/image/font/opentype + go-text/typesetting/segmenter)。
package typography
