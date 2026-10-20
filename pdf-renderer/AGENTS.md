# pdf-renderer AGENTS.md

## Purpose

go-canvas 的 PDF 渲染后端(嵌套 module `github.com/HankChenCH/go-canvas/pdf-renderer`,包名 `pdfrenderer`):以 go-pdf/fpdf 新建单文档为渲染面,实现核心 `renderer.Backend` 五原语,End 产物为 PDF 文档字节 `[]byte`(经 `renderer.RenderAs[[]byte]` 消费)。PHP 权威端的 PDF 姊妹件;位图后端 `image-renderer` 的 PDF 对位件——语义对齐、手段各异(像素搬运 vs 矢量直嵌)。中文是代码注释工作语言;commit 用 conventional-commit 前缀 + 中文 subject。

## Layout

- `backend.go` — Renderer 会话(Begin/End/DrawRect)+ 几何/颜色小件:`pxToPt`(96dpi,pt = px × 0.75,页 MediaBox/坐标/字号一律 ×0.75)、`borderBands`(盒内嵌色带,序 top/bottom/left/right,与 image-renderer 同一公式)、`growthBuffer`。Begin 是**文档续页语义**:首次建档、文档已在则 AddPageFormat 续页(单文档连续性,多页入口工票 03 逐页 Begin);End 防御性复查文档错误位,宁可无产物不可出半成品,**且一次性终结会话**(doc 与字体注册表复位)——End 后同一实例复用(单页/多页入口皆然)必须得到干净新文档,不得续进旧文档(工票 03 验收锁定);渲染失败路径 End 不达,失败后实例不复用(渲染面已弃,重试请各自 New)。
- `renderpages.go` — 多页入口(工票 03):`RenderPages`(便捷具体入口,`*Renderer` + 页序列直出 `[]byte`)与 `RenderPagesAs[T]`(泛型断言,镜像核心 renderas.go 消费面;断言发生在渲染成功后,用法错误不设 code)。编排归核心 `renderer.RenderPages`(工票 01),本文件只做后端接线与产物断言;`assertProduct` 为核心未导出辅助的本 module 自组件。
- `color.go` — `rgba` + `parseColor`(hex 四形态,非法输入报错中止;image-renderer color.go 同式复制,两后端防线对称)。
- `decode.go` — 内在尺寸探测(DecodeConfig,三格式注册)+ `sniffType` 魔数嗅探(扩展名不可信,直嵌 ImageType 用嗅探结果)。
- `exif.go` — EXIF orientation 最小解析(APP1/IFD0 Orientation 标签;image-renderer exif.go 同式复制)。转正消化手段不同:位图后端搬像素,PDF 后端经放置矩阵(`image.go orientAffine`)归正,直嵌字节零重编码。
- `image.go` — DrawImage:直嵌(RegisterImageOptionsReader,fpdf 按名去重)+ ClipRect 裁 dst + 放置矩阵映射 srcRect→dstRect。`affine` 2×3 仿射(y 向下)+ `orientAffine` 八向归正矩阵 + `placeAffine` 像素空间→PDF 原生空间发射公式(F∘P∘F 夹逼,docblock 有推导与恒等自检)。
- `text.go` — DrawText:TTF 子集内嵌(AddUTF8FontFromBytes)+ 对齐/基线/角度 + Helvetica 兜底。度量与字体坑见下节。
- `qr.go` — QRMaterializer + NewDefaultResolver + NewRenderer(ADR-0002 二维码缝默认接线;image-renderer qr.go 同式复制,物化字节面完全一致)。
- `cmd/visualcheck/` — 目验样张:综合样张出 .pdf + pdftoppm -r 96 预览 png(poppler 缺席时跳过预览只出 .pdf)。CJK 字体候选与测试探针同口径,`GO_CANVAS_VISUAL_FONT` 可覆写,全缺降级 Helvetica 只排 ASCII。

## Commands

```sh
go test ./...          # 本 module(poppler 四件套 + CJK TTF 在场时全量;CJK 腿无字体自动 skip)
GOWORK=off go test ./...   # 模拟 CI 消费方解析(module proxy,无 replace)
go run ./cmd/visualcheck   # 目验样张 → visual-check.pdf + 预览 png(已 gitignore)
```

依赖:**poppler 工具链**(pdfinfo/pdftoppm/pdftotext/pdffonts,验收四件套)+ **CJK 单体 TTF**(CI 显式装 fonts-ipafont-gothic;本地无字体该腿 t.Skip,可用 `GO_CANVAS_TEST_CJK_FONT=<单体 .ttf>` 指定)。macOS 本机可用 `/System/Library/Fonts/Supplemental/Arial Unicode.ttf`。

## Rules and gotchas

- **依赖方向单向**:本 module 只可 import 核心(canvas/layer/renderer/resolver)+ fpdf + go-qrcode;**不得 import image-renderer module**(共享语义 = 小件复制并注明出处,几何/颜色/EXIF/QR 四件即此)。核心 module 纯 stdlib 红线不变。
- **fpdf 的 TTC 陷阱**:`AddUTF8FontFromBytes` 对 ttcf 魔数解析失败但**不设错误位**(仅向 stdout 打日志)——静默吞掉会留到 SetFont 才爆 undefined font。`addUTF8Font` 先魔数嗅探拒收 TTC,再以 `GetFontDesc` 非零校验注册生效;垃圾字体字节的 fpdf 日志噪音(`get metrics Error`)是库行为,测试输出可见、无害。
- **字体族名禁用原始路径**:族名会写进 PDF Name 对象(/BaseFont),路径中的 `/` 与空格破坏字典解析(poppler 报 "Dictionary key must be a name object" 且字体面整体降级)——族名 = "f" + sha256 前 16 位hex(`addUTF8Font`)。
- **fpdf 静默无字体 ≠ 可渲染**:核心字体(Helvetica)的 `GetFontDesc` 恒零(AFM 度量未入 Desc),垂直 metrics 用标准 AFM 常量(718/-207,PDF 标准 14 字体规范值);UTF8 字体的 Desc 是 fpdf 归一的 per-1000 值,直接可用。
- **对齐偏移的单位口径**:penX/baselineY 已是 pt(GetStringWidth/GetFontDesc 度量),锚点坐标才 ×pxToPt——两者相加后再乘系数会把对齐整体缩水 0.75(曾踩:右对齐墨迹越锚 7px)。
- **放置矩阵双空间**:绘制推导全在 y 向下像素空间,发射前经 `placeAffine` 换算到 PDF 原生 y 向上;fpdf 自带的 TransformRotate/ClipRect 入参恒用户空间(内部自换算),两套口径不可混用。
- **fpdf 子集内嵌的 pdffonts 形态**:emb=yes / uni=yes 但 sub=no(无 6 字母子集前缀)——subset 语义由 GenerateCutFont 只保留用到的字形保证;pdffonts 断言认 "CID TrueType + Identity-H + yes"。
- **对齐锚点缺省**:TextLayer 默认 verticalAlign=bottom(wire 面历史语义),管线级文本测试需显式 WithVerticalAlign/WithHorizontalAlign。
- **CI**:pdf-renderer job 镜像 image-renderer job(GOWORK=off 无 replace,module proxy 解析核心已发布 tag),显式装 poppler-utils + fonts-ipafont-gothic。
