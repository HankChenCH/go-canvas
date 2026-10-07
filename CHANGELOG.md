# Changelog

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## 1.0.0 - 2026-10-07

首个稳定版，与 `php-canvas-next` v1.0.0 对等。

- 核心 module（纯 stdlib，零第三方依赖）：`canvas` / `layer` / `text` / `hydrate` / `paginate` / `renderer` / `resolver`
- 位图后端嵌套 module `image-renderer`（`imagerenderer.Renderer`，产物 `*image.NRGBA`；go-text/typesetting + yeqown/go-qrcode）
- 与 PHP 端共享 wire 字节面（layout-snapshot）与语义 fixture（expression-eval / expand-semantics / paginate-semantics / flow-semantics），由 PHP 导出提交进本仓库
