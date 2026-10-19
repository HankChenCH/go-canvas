# AGENTS.md

## Purpose

Go 移植版画布渲染库 `github.com/HankChenCH/go-canvas`：与 `php-canvas-next`（PHP 权威端，本地同级 `../php-canvas-next`）共享同一领域语言与 graph wire 契约——`layer.Node` 逐字段键级对齐 PHP 各图层 `graph()` 产物，同一份 graph JSON 双端互读。核心 module **纯 stdlib 零第三方依赖**（ADR-0002）；位图后端是嵌套 module `image-renderer/`（消费方走 module proxy 解析核心已发布 tag，本地双 module 联调经仓库根 `go.work`）。Go 1.25。中文是代码注释的工作语言；commit 用 conventional-commit 前缀 + 中文 subject。

## Layout

- `canvas/` — 画布纯结构容器（尺寸 + priority 降序图层 + 画布背景）；`canvas.FromGraph(g Graph)` 是公共反序列化 API；`Graph = {canvas, layers}`。画布背景（ADR 0022）：`CanvasBackground` 纯值对象（color/image 可空 `*string`，空串归 nil），`Canvas.SetBackground` 空背景与 nil 同态归一（wire 整键不落），`CanvasSize` 字段序 = 键序 width,height,background、`Background` 指针 omitempty 条件写键；背景静态，hydrate/expand 仅透传。
- `layer/` — 图层树纯结构节点：wire 面（`Node`）+ 布局数据，无 I/O 无像素。`FromGraph` 按 type 裸 switch 分派（LayerFactory 等价物），未知类型报 `ErrUnknownLayerType`。
  - `node.go` — wire 面逐字段对齐 PHP（结构体字段序 = JSON 键序）；`Data` 双形态自定义 `MarshalJSON`：`RowsPath` 非空 = 表格形态（仅 rowsPath 一键），否则内容层形态（valueType/expression/value）。
  - `table.go` + `rowtemplate.go` — 表三层容器（add 副作用同 PHP）+ V2 行模板 `TableRowTemplate`（宽度耦合沿用、高度耦合全豁免的装配路径 `AddTemplateContentLayer`/`cellFromGraph(template)`）。
  - 表达式载体（V2）：三内容层 `SetExpression`（value 载体恒镜像原文，字面 setter 解除标记）；wire 上 TextLayer 恒写三键 data、Image/Qr 条件写键（未标记两键）。
- `hydrate/` — 数据填充步骤（V2，渲染前独立纯结构步骤）：`hydrate.NewHydrator(evaluator).Hydrate(canvas, dataset)`——全画布标记字段求值 + 模板表实例化；实现走 graph 层改写 + `canvas.FromGraph` 重建（V1 高度耦合重放免费来自解码路径）。含 `ExpressionEvaluator` 注入缝 + 默认受限插值求值器（内建函数 v1 = date/number/fixed/percent：`dateformat.go`/`numberformat.go` 与 PHP `DateFormat`/`NumberFormat` 同构，十进制字符串舍入、moment token 表；钉稿与共享 fixture 见根 `.scratch/expression-type-methods/`）+ 软诊断发射缝（工票 05，spec/pipeline.md §4）：`Hydrate(c, dataset, warnings...)` 可选收集器（`DiagnosticCollector`，未传 = 静默兜底、存量管线零变化），文案类表达式兜底空串发射 `expression_fallback_empty`（`Diagnostic`，code/message/path 与 PHP 逐字对齐，path = 产物树中立记法）。
- `paginate/` — 分页与文档编译管线（spec §4/§10，ADR 0009）：`Paginator.Paginate`（Canvas 1→n，分页选项 `{pageHeight, truncate?}`，页几何 fail-fast 在 Paginate 入口）+ `DocumentCompiler.Compile`（N 帧 × 1 dataset × 流链，spec §10.3 时序：0 链校验 → 1 全量 hydrate → 2 flow 分配 → 3 回写+分页）双入口；`FlowChainNode`（4 键封闭流链节点，宽松解码留存原始键面供链校验裁决）、`CompileResult`（帧序×页序页序列 + warnings）、paginate/document 两段稳定 code sentinel。**全管线已落地**：链校验 `ValidateFlowChain` 逐规则镜像 PHP 并导出供模板保存期预检（工票 09）；装箱/编译语义镜像 PHP `CanvasPaginator`/`DocumentCompiler`/`Compiler\Flow`（工票 10：`geometry.go` 表几何共享件、`flow.go` 消费策略族 CapacityTake/QuotaTake/PagedTake，paginator/document 走「graph rows 整体替换 → FromGraph 重建」同型路径）——语义 fixture 全绿并入 `make test`；warnings 已实接（工票 05）：`hydrateAll` 全帧共享单一收集器，hydrate 兜底软诊断按帧序汇入 `CompileResult.Warnings`（不带帧标识；`Diagnostic` 定义在 hydrate 包、此处类型别名）。分页页 canvas 键手工重组：**背景必须显式携带每页**（`split` 唯一手工 CanvasSize 构造点，ADR 0022 易漏点 2；document 编译经 Paginate 免费继承）。
- `renderer/` — 渲染契约与模板：后端只实现 `Begin/End/DrawRect/DrawImage/DrawText` 五原语（可选实现 `DefaultResolver()` 自带默认物化器，nil-resolver 组装时优先采用）；泛型辅助 `RenderAs[T]`/`RenderLayerAs[T]`（ADR-0010，复用 `New(backend, nil)` 模板路径）把产物类型断言收敛到调用方泛型实参；**渲染器无预遍历**——paintImage/paintText/paintQrCode 分支开头按需 `ResolveLayer`（text 分支先物化再 `Lines()`），失败即抛、渲染面丢弃（end 不达）。画布背景铺装（ADR 0022）在 `Render` 建面后根层循环前 paintBackground：先色后图（DrawRect 无边框 → 背景图经画布级物化整幅 cover），`RenderLayer` 不画背景。
- `resolver/` — 资源物化（下载/缓存/QR 缝），结果回写图层（`SetResolvedSrc/SetResolvedFont`）；画布级入口 `ResolveBackground`（背景图不是图层，物化结果以返回值交渲染器，img_layers 同目录同语义）。`Resolve` 全画布预遍历 API 保留但渲染器不再调用（角色 = 绘制时物化）。
- `text/` — 断行/度量契约 + 对齐版默认实现；注入点在 `layer.TextLayer`。
- `image-renderer/` — 嵌套 module：位图后端（`imagerenderer.Renderer` 实现 Backend；`NewRenderer` 返回携带默认物化器——二维码缝默认接线——的具体后端，配 `RenderAs` 即完整管线）+ `typography/` 增强文本 + `cmd/visualcheck` 目验样图。
- 语义语料（工票 07 起消费翻转）：语料仓 `canvas-semantics` 以 **submodule pin** 进仓（语义权威，PHP/Go/TS 三端平等消费，语料 diff 即语义变更、升级 = 一次可审的指针移动），runner 直接读 `canvas-semantics/corpus/`：`hydrate/fixture_test.go`（expression-eval v2.0 四模块 104 用例 + fn-date 内 dateTokens 14 行，token 表表面对 = `hydrate/dateformat_test.go`；expand-semantics v1.1 = 29 用例，expect.warnings 软诊断序列比对 = code + path + 序列序）、`paginate/semantics_test.go`（paginate-semantics v1.0 = 12 用例 + flow-semantics v1.1 = 23 用例含软诊断面），随常规 `go test` 运行。**fresh clone 先 `git submodule update --init`**（runner 未检出即红，无降级路径）。支持面声明 = 仓根 `support-surface.json`（渲染端四契约全模块，声明即 runner 套件必须绿；CI core job 以语料仓内 canvasctl `validate --surface` 三互检，工票 08）。`renderer/testdata/layout-snapshot.json` 是布局快照（wire 字节面，PHP 导出、非语料），见 `docs/layout-snapshot.md`。

## Commands

```sh
make test             # 核心 module + image-renderer 两轮 go test(含语义语料 runner,需先检出 submodule)
make vet
make layout-snapshot  # PHP 导出布局快照（需 PHP 8.3 + ext-intl），人审 diff 后随代码提交
make visual-check     # 目验样图 → image-renderer/visual-check.png
```

fresh clone 先 `git submodule update --init`（语料仓未检出 runner 即红）。Homebrew PHP 是 keg-only：`export PATH="$(brew --prefix)/opt/php@8.3/bin:$PATH"`（仅 `make layout-snapshot` 需要）。Go CI 不装 PHP——语义语料经 submodule 检出，布局快照 JSON 提交进本仓库、仅人工触发再生成。

## Rules and gotchas

- **依赖方向单向**：Renderer → Canvas → Layer；`hydrate` 属结构侧（只可 import canvas/layer，不得引 renderer/resolver）；核心 module 保持零第三方依赖——把图像/文本库引回核心是回归。图层 setter 禁止 I/O，物化只发生在 `resolver`。
- **标准管线**（V2）：`renderer.Render(hydrate.NewHydrator(nil).Hydrate(canvas, dataset))`——填充（数据维度，渲染前纯结构）与物化（资源维度，绘制分派前惰性）两维分离；dataset nil 或无绑定画布，两步均恒等直通（返回原画布对象）。
- **wire 条件写键（V2）**：模板态 TableLayer 写 `data`/`template`、不写 `rows`；V1 态恒写 `rows`、不写 `template`/`data`（template ⊕ rows XOR，双键同现报 `ErrTemplateRowsConflict`）。`"template": null` 与缺键同判。表达式标记：TextLayer 恒写三键 data，Image/Qr 仅标记态写 `expression` 键（保 Go↔PHP 字节 parity）；`data.value` 恒镜像表达式原文，求值结果永不落 graph。
- **错误 code 是三端契约**（spec §5.2）：sentinel + `%w` 包装，`errors.Is` 判定；sentinel 值 = 稳定 code（静态文案码带描述，上下文码 bare + wrap）。消息可改、**code 不可改**；新增错误必须归入解码/填充/绘制三阶段之一。唯一豁免：调用侧用法错误（如 `RenderAs[T]` 的期望类型与后端产物不符，工票 13）发生在渲染成功之后，不属渲染失败，普通可读 error 即可、不设 code。
- **`Data` 的 `MarshalJSON` 分形态**：`RowsPath` 非空 = 表格形态仅一键。给 `Data` 加字段先想清楚属于哪个形态；Image/Qr 的 `value` 恒写键（可为 null）不可加 omitempty。
- **填充器的图改写**：模板行每行独立 `json.Unmarshal` 模板载荷（PHP 数组值拷贝同款）——Node 的 `Rows/Cells` 是指针切片、`Content/Template` 是 RawMessage，直接浅拷贝改写会跨行别名；`Hydrate` 不回写源画布有单测锁定。
- **wire 字节面 / 语义改动**：字节面改动必须 `make layout-snapshot` 重导出并人审 diff（存量用例零 diff = 存量图字节等价）；表达式/填充等编译语义改动走语料仓提案通道（`canvas-semantics/proposals/`），本仓随 submodule 指针移动跟进，runner 红即实现未跟上语义。
- **priority 语义**：越大越先渲染（视觉上越垫底）。
