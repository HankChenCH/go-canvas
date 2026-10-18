package hydrate

// 软诊断发射缝(spec/pipeline.md §4/§5,PHP Compiler\Diagnostic +
// DiagnosticCollector 同构):硬失败走 error(三阶段 sentinel,spec §5.2),
// 策略性软失败走收集通道——发射是 Hydrator 的职责,读取归调用方汇入文档编译
// CompileResult.Warnings。现役发射面唯一:文案类表达式兜底空串。

// Diagnostic 软诊断值对象:Code 是三端一致性锚点(语料注册表
// expression_fallback_empty),Message 人读可本地化(不入断言面),Path 为产物树
// 定位(中立记法:产物树根相对点路径 + 十进制数组下标,spec/pipeline.md §5)
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// DiagnosticExpressionFallbackEmpty 软诊断 code:文案类表达式求值空串、
// 兜底为空(硬失败面见三阶段 sentinel;code 稳定不可改)
const DiagnosticExpressionFallbackEmpty = "expression_fallback_empty"

// DiagnosticCollector 软诊断收集器(PHP DiagnosticCollector 同构):Collect 发射、
// All 读取;未接入 = 静默兜底,独立管线 render(hydrate(...)) 行为与历史零变化
type DiagnosticCollector struct {
	diagnostics []Diagnostic
}

// NewDiagnosticCollector 构造空收集器
func NewDiagnosticCollector() *DiagnosticCollector {
	return &DiagnosticCollector{diagnostics: []Diagnostic{}}
}

// Collect 发射一条软诊断(发射序即收集序,语料期望序列逐用例钉定)
func (c *DiagnosticCollector) Collect(diagnostic Diagnostic) {
	c.diagnostics = append(c.diagnostics, diagnostic)
}

// All 已收集的诊断序列(恒非 nil;不携带帧标识,帧标识升格走 spec 版本)
func (c *DiagnosticCollector) All() []Diagnostic {
	return c.diagnostics
}
