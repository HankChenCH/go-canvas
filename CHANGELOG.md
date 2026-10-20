# Changelog

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## 1.7.0 - 2026-10-20

**Breaking**（ADR 0026，接口扩展，/v2 显式延后）:渲染后端契约新增可选形状能力口 `renderer.ShapeCapableBackend`（嵌入 `Backend` 的加法接口，两新原语 `DrawEllipse(x, y, w, h, fillColor, stroke)` 与 `DrawImageMasked(src, srcRect, dstRect, maskRect)`）——既有 `Backend` 实现零破坏（接口探测为可选能力口，未实现者对 clip 声明静默降级矩形渲染），希望承接圆形档的后端按新接口补两原语。Go 生态全第一方锁步升级，本版以 minor 携 breaking note（ADR 0024 先例），外部后端作者出现时再议 /v2。

- 图片层裁剪形状 clip 跟随（与 `php-canvas-next` 对等，ADR 0026 裁剪形状圆形档）：`layer.ImageLayer` 增 `ClipRect`/`ClipCircle` 常量与 `SetClip`/`Clip`（空串 = rect 缺省，显式 rect 归一空串，未知值宽容透传，与 src/fit 正交）；wire 条件写键（缺省整键不落、殿后 spec 末位 fit 之后，键序 shape/align/position/fit/clip，缺省态字节面与无字段版本一致），存量 wire 断言按设计保绿（layout-snapshot 存量零 diff 无需重导）；hydrate/expand 对 spec 仅透传（`image-fit-spec-passthrough` 锚点普适语义覆盖，零改动）
- 内切圆与圆周描边纯函数（与 PHP 同公式、整除向零截断同门）：`ImageLayer.InscribedCircleRect`（边长 = min(内容区宽高)、盒内对齐定位——center 取整盒中点、left/top 贴 padding 原点侧、非方盒恒正方几何留透明边不变形为椭圆）、`ImageLayer.CircleStroke`（宽取四边已声明宽最小值、色按 top→right→bottom→left 取首个非空边色、四边全未声明不描边；未声明或 width≤0 的边不计，与 borderBands 同纪律）
- 渲染臂圆形档三步分派：`paintImage` 双门（clip 白名单 + `ShapeCapableBackend` 类型断言）命中走 `paintImageCircle`——① 椭圆背景填充（矩形盒 DrawRect 不画）→ ② 内容按 fit 绘出后遮罩 blit（掩码矩形 = 内切圆矩形）→ ③ 边框沿圆周描环；未知 clip 值与后端缺能力口降级矩形路径（fit 未知值纪律同门），内切圆边长 ≤0 零几何零绘制，内在尺寸不可用退化 fill 形态再裁圆（ADR 0024 纪律）；fit 分派几何抽 `paintFitBlits` 两形态共用同一份防数学漂移（先绘后裁，fit 与 clip 正交）
- 嵌套 module `image-renderer`（**v1.2.0**，核心与位图后端锁步 minor）两原语实现（stdlib draw 掩码面，二值覆盖无抗锯齿对齐 PHP GD 驱动形态）：`DrawEllipse`（draw.DrawMask 椭圆填充 + 外缘内缩环带描边——内缩带与 DrawRect 盒内嵌边带同门）与 `DrawImageMasked`（blit 前段抽 `blitSource` 与 DrawImage 同源 + 椭圆掩码逐像素置透明）；像素点位平移 PHP 矩阵（四角透明露下层、圆心为图、环带为描边色、padding 留白、非方盒正圆透明边、circle × fit 四档组合、未知 clip 值降级矩形），无黄金文件

## 1.6.0 - 2026-10-20

- 核心 module renderer 包多页 walk 模板 `RenderPages`（go-pdf-renderer effort 工票 01，canvas-compiler 工票 07「AbstractRenderer 多页钩子」Go 同构）：`RenderPages(ctx, backend, pages []*canvas.Canvas) (any, error)`——逐页 Begin(页宽,页高) → 背景逐页显式铺装（ADR 0022，先色后图）→ 逐层 paint，全部页完成后统一 End 一次（n 页 = n 次 Begin + 1 次 End）；ctx 检查点页间一处新增（下一页 Begin 前），既有根层每图层前/表行每行前检查点同位不动；任一页任一原语/物化失败即 error、End 不达、整个文档无产物逃逸；空页序列守卫按调用侧用法错误处理（错误 code 豁免条款同步扩写）。既有 `Render`/`RenderLayer` 签名与行为零变化（`renderSurface` 页内段抽取 `paintPage`、单页两段式，存量测试保绿），wire 字节面不动（layout-snapshot 无需重导）
- 新增嵌套 module `pdf-renderer/`（**pdf-renderer/v1.0.0**，Go 侧 PDF 渲染后端，`github.com/go-pdf/fpdf`）：Backend 五原语全实现——px→pt ×0.75（96dpi 口径，页 MediaBox 同口径）、PNG/JPEG/GIF 直嵌（ClipRect 裁目标 + 放置矩阵映射 srcRect→dstRect，EXIF orientation 八向放置矩阵归正，空 srcRect 整图哨兵/空 dstRect 防护不画）、TTF 子集内嵌文本（AddUTF8Font，CJK 硬门槛；TTC 显式拒收；兜底 = PDF 核心字体 Helvetica，ASCII-only 语义对齐位图后端 basicfont）、矩形填充 + 边框盒内色带（borderBands 几何小件与位图后端对称复制）；`NewRenderer` 二维码缝默认接线镜像位图后端；多页入口 `RenderPages`/`RenderPagesAs`（工票 03）消费 `paginate.CompileResult.Canvases` 输出单文档；Begin 文档续页语义 + End 会话复位（复用实例得干净新文档）。验收 = poppler 四件套（pdfinfo/pdftoppm/pdftotext/pdffonts）点位断言，无黄金文件

## 1.5.0 - 2026-10-19

**Breaking**（ADR 0024，/v2 显式延后）:渲染后端原语 `renderer.Backend.DrawImage` 签名变更为 `DrawImage(src string, srcRect, dstRect image.Rectangle)`——双矩形 blit,原语零 fit 语义、零内在尺寸读取;`srcRect` 空矩形 = 整图哨兵(按解码实际尺寸取整图),`dstRect` 空矩形后端防护不画。Go 生态全第一方锁步升级,/v2 module path 不付,本版以 minor 携 breaking note,外部后端作者出现时再议。

- 图片层适配模式 fill/tile 转正(与 `php-canvas-next` v2.0.0 对等,ADR 0024):`layer.ImageLayer` 增 `FitFill`/`FitTile` 常量;`CoverRect`(源图坐标等比居中裁窗,公式唯一落点包级 `CoverSourceWindow` 供画布背景臂共用)、`TileRects`(tile 可见块 `TileBlock{Src, Dst}` 组,stdlib `image.Rectangle`——单元 = 内在尺寸 1:1、首块按对齐锚定(大图 center 负锚点露中部)、网格 ∩ 内容盒逐块行主序、末行末列半块截断)
- 渲染臂全 fit 分派:`paintImage` 白名单四档——cover(缺省/显式归一/未知值降级)等比居中裁窗、fill 整图进与 cover 同门 dstRect、contain 收缩(ADR 0023 语义逐字节不动)、tile 可见块逐块 blit;内在尺寸读取扩为全 fit 前置(`image.DecodeConfig` 绘制分派点一次,注册集 PNG/JPEG/GIF 不变),读取失败统一退化 fill 形态(srcRect 零哨兵,真损坏图在后端解码处报错,结局 parity);背景臂分派点算 cover 窗(ADR 0022 恒 cover 不变)、QR 臂零哨兵整图 blit(ADR 0015 内切正方形语义不变,零尺寸读取)
- `image-renderer` 位图后端 blit 实现(嵌套 module **v1.1.0**,同以 minor 携 breaking note):`DrawImage` 改双矩形拉伸搬运(解码 → 内在坐标系裁窗 → EXIF 转正后置 → CatmullRom 拉伸 → 合成),内嵌 cover 语义退役(`coverImage` 连同 ≥1 窗钳制移除——极端宽高比零窗走整图哨兵,对齐 PHP intval 不钳制语义);像素点位补 fill 拉伸形变、tile 网格锚定/截断/空隙、cover/contain 回归、损坏图全 fit 结局等价、EXIF 裁窗次序(内在坐标系裁窗)
- wire 零变化:fit 值是透传字符串,fill/tile 转正不改字节面——fit=fill/tile 落键/往返恒等腿、`make layout-snapshot` 存量零 diff(8 用例 92 记录复导验证);image-fit-wire 用例「未知 fill」表述注释同步(fill 转正后为落键值透传演示),白名单外降级示例改用从未入册的值

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
