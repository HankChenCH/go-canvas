# go-canvas

[`php-canvas-next`](https://github.com/HankChenCH/php-canvas-next) 的 Go 移植：领域契约从语料仓 [`canvas-semantics`](https://github.com/HankChenCH/canvas-semantics) 定义（表达式求值/扫描、hydrate 填充、flow 流装箱、paginate 分页，语料先行，PHP/Go/TS 三端平等消费同一批语料用例）；graph wire 字节面仍以 PHP 权威端为锚，`layer.Node` 键级对齐 PHP 各图层 `graph()` 产物，同一份 graph JSON 双端互读、布局级一致。

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

- **编译语义**（求值/填充/分页/流装箱）以语料仓 `canvas-semantics` 为权威（submodule pin 消费，fresh clone 先 `git submodule update --init`）：PHP/Go/TS 三端 runner 共同消费同一批语料，语料 diff 即语义变更、升级 = 一次可审的 submodule 指针移动；本仓支持面见 `support-surface.json`。
- **wire 字节面**仍以 PHP 权威端为锚：`renderer/testdata/layout-snapshot.json`（布局快照）由 PHP 端导出提交进本仓库，`make layout-snapshot` 按需再生成，何时重导见 Makefile 注释与 `docs/layout-snapshot.md`。
- 设计共识见 [DESIGN.md](DESIGN.md)；开发约定见 [AGENTS.md](AGENTS.md)。

## 开发

```sh
git submodule update --init  # 首次检出语料仓(语义语料权威,runner 直接读 corpus/)
make test          # 核心 module + image-renderer 两轮 go test（含语义语料 runner）
make vet
make visual-check  # 目验样图 → image-renderer/visual-check.png
```

仓库根的 `go.work` 供本地双 module 联调（image-renderer 即时解析本地核心改动）；发布后的消费方解析由 CI 的 `GOWORK=off` job 验证。
