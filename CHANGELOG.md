# Changelog

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## 1.4.0 - 2026-10-18

- 图片层适配模式 fit 跟随（与 `php-canvas-next` v1.4.0 对等，ADR 0023）：`layer.ImageLayer` 静态声明 `SetFit`/`Fit`（空串 = cover 缺省，显式 cover 归一，未知枚举串宽容透传）、`ContainRect` 纯布局收缩矩形（对齐放置公式抽 `alignedX`/`alignedY` 与 `ImageOrigin` 同源）；wire 条件写键（缺省整键不落、落键殿后 spec 末位 shape/align/position/fit，缺省态字节面与无字段版本一致），hydrate/expand 对 fit 仅透传（独立层与模板表实例化两路径测试锁定）
- 渲染臂 contain（ADR 0023）：`renderer.paintImage` 白名单在渲染臂——`image.DecodeConfig` 读物化后源图内在尺寸（绘制分派期 I/O，stdlib PNG/JPEG/GIF 注册集）算收缩矩形再调 `DrawImage`，原语签名与 cover 语义一字不动；读取失败/未知值降级 cover，cover 路径零回归
- 共享 fixture 重导（layout-snapshot 新增 image-fit-wire parity 用例，存量零 diff；expand-semantics 24 用例零 diff）；`image-renderer` 位图后端零改动，补 contain 像素点位断言（空隙透明露下层与盒背景、align + padding 放置、未知值/显式 cover 降级与 cover 点位一致、损坏图渲染结局与 cover 逐位等价）

## 1.3.0 - 2026-10-18

- 画布背景声明式填充跟随（与 `php-canvas-next` v1.3.0 对等，ADR 0022）：`canvas.CanvasBackground` 纯值对象（color/image 可空分量、空串归缺省、宽容读归一），`Canvas.SetBackground` 空背景与无背景同态；wire 条件写键（两分量皆空整键不落，键序钉死 canvas{width,height,background} / background{color,image}），落键/不落键两态解码 → 再编码往返恒等
- 渲染铺装：`Render` 建面后根层循环前先色后图（DrawRect 无边框 → 背景图整幅 cover），背景图画布级惰性物化（`resolver.ResolveBackground` 新入口，img_layers 同目录同语义，失败即抛无产物）；`RenderLayer` 不画背景（无画布矩形语义）
- 分页每页携带背景：`Paginator.split` 页 canvas 键手工重组显式携带（易漏点 2）；hydrate 对背景仅透传（零改动，测试锁定）
- 共享 fixture 重导（layout-snapshot 7 用例 / hydrate 128 用例）人审存量零 diff；目验样图换装画布背景，产物与换装前逐字节一致；嵌套 module `image-renderer` 仅 visualcheck 换装（v1.0.1）

## 1.2.0 - 2026-10-15

- 核心 module `hydrate/` 资源表达式单值门 `expression_resource_single_ref`（与 `php-canvas-next` v1.2.0 对等，ADR 0020）：已标记 Image/Qr 的 expression 须恰为一个 `{{...}}` 闭合片段且占满整串，求值前 fail-fast
- 共享 fixture 再生成 24 用例

## 1.1.0 - 2026-10-11

- 核心 module `hydrate/` 镜像实现表达式内建函数（与 `php-canvas-next` v1.1.0 对等，ADR 0018）：`dateformat.go`（moment 风格 14 token、方括号转义、严格 ISO 解析、Sakamoto 周几）与 `numberformat.go`（十进制字符串语义、half-up 朝 +∞、千分位/定点/百分比），纯 stdlib 不破
- 求值器片段分类（路径｜调用）与调用解析（首参吞至顶层 `,`/`)`、单引号翻倍转义）；未知函数/参数个数/文法错 fail-fast 并入 `expression_syntax_error`，零新 sentinel
- 共享 fixture contract v2：118 项断言全绿（104 用例 + dateTokens 14 行端到端）
- 收敛两处旧实现与 PHP 权威端的分叉（单测锁定）：数组中途下钻 = 缺字段信号（含 PHP 数字串键转换语义）、片段 trim = PHP trim 字符集（不剥 NBSP 等非 ASCII 空白）
- wire 字节面零改动，嵌套 module `image-renderer` 本版无变化

## 1.0.0 - 2026-10-07

首个稳定版，与 `php-canvas-next` v1.0.0 对等。

- 核心 module（纯 stdlib，零第三方依赖）：`canvas` / `layer` / `text` / `hydrate` / `paginate` / `renderer` / `resolver`
- 位图后端嵌套 module `image-renderer`（`imagerenderer.Renderer`，产物 `*image.NRGBA`；go-text/typesetting + yeqown/go-qrcode）
- 与 PHP 端共享 wire 字节面（layout-snapshot）与语义 fixture（expression-eval / expand-semantics / paginate-semantics / flow-semantics），由 PHP 导出提交进本仓库
