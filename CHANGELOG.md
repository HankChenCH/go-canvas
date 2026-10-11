# Changelog

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

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
