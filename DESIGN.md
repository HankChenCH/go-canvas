# go-canvas 设计共识

2026-09-22 与 PHP 实现对照的移植设计访谈（grilling 全树走完）确认的共识。移植蓝本与逐条证据见 `../docs/golang-port-research.md`；领域词汇见根目录 `CONTEXT.md`；关键决策的 why 见 `../docs/adr/000{1,2,3}-*.md`。

## 定位

- 根需求是**双端互通**：同一份 graph JSON 两端都能渲染，布局级一致（断行行数、行内容、图层坐标），像素不求等。
- v1 **严格对等** PHP 已有能力：圆角、透明度、混合模式、阴影一律不做，另立项。
- 表达式引擎不存在；wire 面保留 `data.expression` 空串占位，与 PHP 字节面兼容。

## 仓库与模块

- 位置：本目录（工作区子目录，独立 git 仓库）；暂无远端、暂不配 CI，推远端时再加 GitHub Actions。
- module path：`github.com/hankchen/go-canvas`；Go 基线 1.25。
- **2 modules**：
  - 根 module = 核心包（`canvas`/`layer`/`renderer`/`resolver`/`text`），**纯 stdlib 零第三方依赖**（ADR-0002）；依赖方向 canvas ← {renderer, resolver}、text ← layer ← canvas，由 import 图在编译期硬保证。
  - `image-renderer/`（M2 新建嵌套 module）：x/image、go-text/typesetting、go-qrcode、按需 bild。
- **纯 Go 硬红线**：不接受 CGO（ADR-0001）。

## API 形态

- functional options：`NewTextLayer(opts ...Option)` 全 options；auto 用 `WithAutoWidth()` / `WithAutoHeight()`（graph 面本来就是 autoWidth 布尔标志，不复刻 PHP setter 的 `'auto'` 字符串糖）。
- 表格保留 `AddRow` / `AddCell` / `AddContentLayer` 增量方法（副作用锚点：同步行宽、取最高单元格、压平内容层）；`WithRows` / `WithCells` 批量 option 内部调同一条 add 路径；`FromGraph` 反序列化复现该路径。
- 度量器工厂用函数类型 `MeasurerFactory func(fontFile string, fontSize float64) TextMeasurer`（PHP 注入工厂是因为 Face 昂贵，Go 函数类型即工厂）。

## 文本策略（ADR-0003）

- 默认 = 对齐 PHP：`HeuristicMeasurer`（ASCII 0.55 字宽）+ 照搬版贪心 UAX #14 断行器。
- 增强实现（opentype 真实度量 + typesetting 完整 UAX #14）M3 交付，可注入、非默认。
- PHP `getTextOrigin()` 的 GD 基线魔数 `- round(fontSize*0.1)` **不移植**：布局层返回纯对齐锚点，基线差由渲染后端 `drawText` 用字体 metrics 消化；双端布局快照中文本 y 坐标是预期差异字段。
- 字素切分：核心包零依赖 → text 包定义 `Segmenter` 接缝，默认码点切分（等价 PHP 无 ext-intl 的降级路径）；M3 的文本 module 注入 UAX #29 实现（等价 PHP 有 ext-intl 的正常路径）。
- 内置默认字体（工单 07）：Go 无 GD 内置字体的对应物，空串/纯数字字体 id 的"内置默认字体语义"取 x/image 自带 `basicfont.Face7x13` 兜底（后端 module 零新增依赖；固定 7×13 点阵仅覆盖 ASCII 且字号无效，CJK 必须显式提供真实字体文件）。真实字体按 magic bytes 分派 opentype Parse/ParseCollection（TTC 取首个 face）；opentype.Face 非并发安全，按渲染会话（Renderer 实例）持有。

## 行为对等（可观察行为面）

- priority 越大越先渲染（越垫底）；排序稳定（对齐 PHP 8.0+ usort）。
- 默认值逐字段对齐：TextLayer 垂直 bottom、ImageLayer center/center，其余 left/top。
- QR 固定选项：ECC=High、margin=0、roundBlockSizeMode=None、黑白双色、按宽度正方形铺放。
- 容器 add 副作用一次性同步（与 PHP 一致，先 add 后改尺寸不重算）。
- graph 往返恒等；type 常量与 PHP 同名（`TextLayer` 等）；json tag 逐字段对齐 PHP 键名（含 `spec.fontFamily` 这个字面键）。
- 数值语义：布局全程整型、向零截断（与 PHP intval 一致）；PHP setter 面的数字字符串宽容性不复刻（Go 无 setter 面）。

## 有意偏离 PHP 的点（勿"修复"）

1. QR 物化接口注入，核心零依赖（ADR-0002）。
2. 缓存：`os.UserCacheDir()/go-canvas/` + sha256(完整 URL) 键——PHP 的 basename 键有同名碰撞，属旧债不继承；缓存不在互通契约面。
3. GD 基线魔数不移植（ADR-0003）。
4. 图层 setter 的 fluent 链改为 functional options；隐藏行为（border width=0 清除等）落在 option/方法构造器里。
5. 背景空串语义：`WithBackground("")` 归 null（等价 PHP `setBackground(null)` 清除）；PHP 传 `''` 存空串的行为不复刻（Go 无 null 字符串字面）。wire 解码侧指针直传，`""` 与 null 照常区分，往返与互通不受影响。
6. padding 双值选项 `WithPaddingVH(vertical, horizontal)` 取 CSS/PHP 首参垂直序（PHP `setPadding(1,2)` 上下=1、左右=2）。

## 验收与测试

- 结构侧：phpunit 用例平移为 Go 表驱动测试（排序稳定性、graph 快照与往返、断行 12 例、度量 4 例、锚点 9 位 + 负溢出、Resolver 缓存行为）。
- 布局快照 fixture 轨（M3）：PHP 导出脚本产出"断行行数 + 行内容 + 锚点坐标"JSON 进仓库，Go 断言；PHP 导出走 Makefile 人工触发，不进 Go CI。文本 y 为预期差异字段。
- 渲染侧（M2）：像素属性断言（点位取样）+ golden 容差，不做逐像素复刻；平移 visual-check 目验脚本。

## 交付节奏

- M1（本 module）：结构 + graph 序列化 + 布局 + 文本契约 + 对齐版默认实现。
- M2：渲染后端 module（五原语 + 文本绘制 + QR 物化 + 像素属性断言）。
- M3：增强实现（真实度量 + segmenter 断行 + UAX #29 字素）+ 布局快照 fixture + 目验脚本。
