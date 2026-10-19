# Changelog

本项目遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

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
