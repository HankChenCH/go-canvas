# go-canvas

[`php-canvas-next`](https://github.com/HankChenCH/php-canvas-next) 的 Go 移植：与 PHP 权威端共享同一 graph wire 契约与领域语言（`layer.Node` 逐字段键级对齐 PHP 各图层 `graph()` 产物），同一份 graph JSON 双端互读、布局级一致。

## 模块结构

核心 module `github.com/HankChenCH/go-canvas`（**纯 stdlib，零第三方依赖**）：

| 包 | 职责 |
| --- | --- |
| `canvas/` | 画布纯结构容器；`canvas.FromGraph` 反序列化 |
| `layer/` | 图层树纯结构节点（wire 面 + 纯布局函数，无 I/O 无像素） |
| `text/` | 断行/度量契约与对齐版默认实现 |
| `hydrate/` | 渲染前数据填充（表达式求值 + 模板表实例化） |
| `paginate/` | 内容流文档编译与分页（`Paginator` / `DocumentCompiler`） |
| `renderer/` | 渲染契约：后端实现五原语，泛型辅助 `RenderAs[T]` / `RenderLayerAs[T]` |
| `resolver/` | 资源物化（下载/缓存/QR 缝），结果回写图层 |

位图后端是嵌套 module `github.com/HankChenCH/go-canvas/image-renderer`（`imagerenderer.Renderer`，标准库 image + go-text/typesetting + yeqown/go-qrcode）。

## 安装

```sh
go get github.com/HankChenCH/go-canvas
# 位图后端（自动带上核心 module）：
go get github.com/HankChenCH/go-canvas/image-renderer
```

要求 Go 1.25+。文本渲染需提供真实 TTF/TTC 字体资源。

## 使用

graph JSON 与 PHP 端 `graph()` 产物互通，直接解码重建：

```go
import (
	"context"
	"encoding/json"
	"image"

	"github.com/HankChenCH/go-canvas/canvas"
	"github.com/HankChenCH/go-canvas/image-renderer"
	"github.com/HankChenCH/go-canvas/renderer"
)

var graph canvas.Graph
_ = json.Unmarshal(graphJSON, &graph)
c, err := canvas.FromGraph(graph)

img, err := renderer.RenderAs[*image.NRGBA]( // 透明底 *image.NRGBA
	context.Background(),
	imagerenderer.NewRenderer(nil), // nil = 后端自带默认资源物化器（含 QR 缝）
	c,
)
```

数据填充（可选，渲染前独立纯结构步骤，dataset nil 时恒等直通）：

```go
c, err = hydrate.NewHydrator(nil).Hydrate(c, dataset) // nil = 默认受限插值求值器
```

## 双端一致性

- wire 字节面与语义以 **PHP 为权威**：`renderer/testdata/layout-snapshot.json`（布局快照）与 `hydrate/testdata/`、`paginate/testdata/`（语义 fixture）由 PHP 端导出提交进本仓库，`make layout-snapshot` / `make hydrate-fixtures` 等按需再生成，何时重导见各 make 目标注释。
- 设计共识见 [DESIGN.md](DESIGN.md)；开发约定见 [AGENTS.md](AGENTS.md)。

## 开发

```sh
make test          # 核心 module + image-renderer 两轮 go test（含语义 fixture runner）
make vet
make visual-check  # 目验样图 → image-renderer/visual-check.png
```

仓库根的 `go.work` 供本地双 module 联调（image-renderer 即时解析本地核心改动）；发布后的消费方解析由 CI 的 `GOWORK=off` job 验证。
