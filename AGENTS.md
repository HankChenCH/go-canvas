# AGENTS.md

## Purpose

Go 移植版画布渲染库 `github.com/hankchen/go-canvas`：与 `php-canvas-next`（PHP 权威端，本地同级 `../php-canvas-next`）共享同一领域语言与 graph wire 契约——`layer.Node` 逐字段键级对齐 PHP 各图层 `graph()` 产物，同一份 graph JSON 双端互读。核心 module **纯 stdlib 零第三方依赖**（ADR-0002）；位图后端是嵌套 module `image-renderer/`（go.mod `replace => ../`）。Go 1.25。中文是代码注释的工作语言；commit 用 conventional-commit 前缀 + 中文 subject。

## Layout

- `canvas/` — 画布纯结构容器（尺寸 + priority 降序图层）；`canvas.FromGraph(g Graph)` 是公共反序列化 API；`Graph = {canvas, layers}`。
- `layer/` — 图层树纯结构节点：wire 面（`Node`）+ 布局数据，无 I/O 无像素。`FromGraph` 按 type 裸 switch 分派（LayerFactory 等价物），未知类型报 `ErrUnknownLayerType`。
  - `node.go` — wire 面逐字段对齐 PHP（结构体字段序 = JSON 键序）；`Data` 双形态自定义 `MarshalJSON`：`RowsPath` 非空 = 表格形态（仅 rowsPath 一键），否则内容层形态（valueType/expression/value）。
  - `table.go` + `rowtemplate.go` — 表三层容器（add 副作用同 PHP）+ V2 行模板 `TableRowTemplate`（宽度耦合沿用、高度耦合全豁免的装配路径 `AddTemplateContentLayer`/`cellFromGraph(template)`）。
  - 表达式载体（V2）：三内容层 `SetExpression`（value 载体恒镜像原文，字面 setter 解除标记）；wire 上 TextLayer 恒写三键 data、Image/Qr 条件写键（未标记两键）。
- `hydrate/` — 数据填充步骤（V2，渲染前独立纯结构步骤）：`hydrate.NewHydrator(evaluator).Hydrate(canvas, dataset)`——全画布标记字段求值 + 模板表实例化；实现走 graph 层改写 + `canvas.FromGraph` 重建（V1 高度耦合重放免费来自解码路径）。含 `ExpressionEvaluator` 注入缝 + 默认受限插值求值器。
- `renderer/` — 渲染契约与模板：后端只实现 `Begin/End/DrawRect/DrawImage/DrawText` 五原语（可选实现 `DefaultResolver()` 自带默认物化器，nil-resolver 组装时优先采用）；泛型辅助 `RenderAs[T]`/`RenderLayerAs[T]`（ADR-0010，复用 `New(backend, nil)` 模板路径）把产物类型断言收敛到调用方泛型实参；**渲染器无预遍历**——paintImage/paintText/paintQrCode 分支开头按需 `ResolveLayer`（text 分支先物化再 `Lines()`），失败即抛、渲染面丢弃（end 不达）。
- `resolver/` — 资源物化（下载/缓存/QR 缝），结果回写图层（`SetResolvedSrc/SetResolvedFont`）。`Resolve` 全画布预遍历 API 保留但渲染器不再调用（角色 = 绘制时物化）。
- `text/` — 断行/度量契约 + 对齐版默认实现；注入点在 `layer.TextLayer`。
- `image-renderer/` — 嵌套 module：位图后端（`imagerenderer.Renderer` 实现 Backend；`NewRenderer` 返回携带默认物化器——二维码缝默认接线——的具体后端，配 `RenderAs` 即完整管线）+ `typography/` 增强文本 + `cmd/visualcheck` 目验样图。
- 共享 fixture（均 PHP 导出、JSON 提交进本仓库）：`hydrate/testdata/`（expression-eval v1 + expand-semantics v1，runner = `hydrate/fixture_test.go`）、`renderer/testdata/layout-snapshot.json`（布局快照，见 `docs/layout-snapshot.md`）。

## Commands

```sh
make test             # 核心 module + image-renderer 两轮 go test
make vet
make layout-snapshot  # PHP 导出布局快照（需 PHP 8.3 + ext-intl），人审 diff 后随代码提交
make hydrate-fixtures  # PHP 导出 V2 共享 fixture（同上）
make visual-check     # 目验样图 → image-renderer/visual-check.png
```

Homebrew PHP 是 keg-only：`export PATH="$(brew --prefix)/opt/php@8.3/bin:$PATH"`。Go CI 不装 PHP——fixture JSON 提交进本仓库，仅人工触发再生成。

## Rules and gotchas

- **依赖方向单向**：Renderer → Canvas → Layer；`hydrate` 属结构侧（只可 import canvas/layer，不得引 renderer/resolver）；核心 module 保持零第三方依赖——把图像/文本库引回核心是回归。图层 setter 禁止 I/O，物化只发生在 `resolver`。
- **标准管线**（V2）：`renderer.Render(hydrate.NewHydrator(nil).Hydrate(canvas, dataset))`——填充（数据维度，渲染前纯结构）与物化（资源维度，绘制分派前惰性）两维分离；dataset nil 或无绑定画布，两步均恒等直通（返回原画布对象）。
- **wire 条件写键（V2）**：模板态 TableLayer 写 `data`/`template`、不写 `rows`；V1 态恒写 `rows`、不写 `template`/`data`（template ⊕ rows XOR，双键同现报 `ErrTemplateRowsConflict`）。`"template": null` 与缺键同判。表达式标记：TextLayer 恒写三键 data，Image/Qr 仅标记态写 `expression` 键（保 Go↔PHP 字节 parity）；`data.value` 恒镜像表达式原文，求值结果永不落 graph。
- **错误 code 是三端契约**（spec §5.2）：sentinel + `%w` 包装，`errors.Is` 判定；sentinel 值 = 稳定 code（静态文案码带描述，上下文码 bare + wrap）。消息可改、**code 不可改**；新增错误必须归入解码/填充/绘制三阶段之一。唯一豁免：调用侧用法错误（如 `RenderAs[T]` 的期望类型与后端产物不符，工票 13）发生在渲染成功之后，不属渲染失败，普通可读 error 即可、不设 code。
- **`Data` 的 `MarshalJSON` 分形态**：`RowsPath` 非空 = 表格形态仅一键。给 `Data` 加字段先想清楚属于哪个形态；Image/Qr 的 `value` 恒写键（可为 null）不可加 omitempty。
- **填充器的图改写**：模板行每行独立 `json.Unmarshal` 模板载荷（PHP 数组值拷贝同款）——Node 的 `Rows/Cells` 是指针切片、`Content/Template` 是 RawMessage，直接浅拷贝改写会跨行别名；`Hydrate` 不回写源画布有单测锁定。
- **wire 字节面 / 语义改动**：字节面改动必须 `make layout-snapshot` 重导出并人审 diff（存量用例零 diff = 存量图字节等价）；表达式/填充语义改动跑 `make hydrate-fixtures`（PHP 是语义权威）。
- **priority 语义**：越大越先渲染（视觉上越垫底）。
