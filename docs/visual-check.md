# 目验脚本（visual-check）

一条命令渲染综合样图供人工目验——像素取样、golden 容差与布局快照之外的人眼验收面。
对应 PHP 侧 `php-canvas-image-renderer/scripts/visual-check.php`，样图内容复刻 PHP 版。

## 用法

```sh
make visual-check                 # 产物缺省 image-renderer/visual-check.png(已 gitignore)
make visual-check OUTPUT=out.png  # 或直接: cd image-renderer && go run ./cmd/visualcheck out.png
```

成功即打印产物路径与尺寸（`已输出: … (400x400)`）；缺字体、资源物化失败等以明确报错中止，不产残图。

脚本本体 `image-renderer/cmd/visualcheck/main.go`，含同包冒烟测试（结构断言 + 渲染无 panic + 产物 PNG 合法）。

## 样图内容与目验要点

400×400 画布，逐项对应一条验收线：

| 样图元素 | 目验要点 |
| --- | --- |
| 白底图层（priority 11，最垫底） | 全图无透明区域——priority 叠加语义：越大越先渲染（越垫底） |
| 蓝色头图（priority 10）+ 白色标题（priority 5） | 叠加次序正确：标题画在色块上，水平垂直双居中，中文不缺字、不成方块 |
| 中文段落（autowrap，#f5f7fa 底） | 断行遵循禁则：行首不出现句号/逗号/顿号等收尾标点；`hello world` 在词边界断行；段落背景与 padding 可见 |
| 表格三行两列（含嵌套单元格） | 表头行底色 #eef3fd、单元格边框 #dddddd、行高 auto 取最高单元格、内容文本 padding 可见 |
| 二维码（90×90） | 扫码可得 `https://github.com/hankchen/go-canvas`；按宽正方形铺放 |
| 条带图（本地生成双色 PNG） | 图片图层 cover 铺放：#e8f0e8 底 + #6dc287 色块，无拉伸变形 |
| 页脚（居中灰字） | 小字号文本垂直居中 |

## 字体

候选列表 CJK 优先，回退顺序与 PHP 脚本逐项对齐（STHeiti → Songti → Hiragino Sans GB →
DejaVuSans → Arial）。择取标准为「**存在且可加载**」：Go 侧 sfnt 解析不了部分 macOS 系统字体
（实测 STHeiti Medium.ttc 的 cmap 段数不受支持，PHP/GD 则能吃下），只查存在会把解析失败留到
渲染中段；加载探针复用渲染端同一入口（`typography.LoadFontFace`），探测口径与实际渲染一致。
全缺时报错逐条列明落选原因，提示安装字体或扩充 `fontCandidates`——内置点阵默认字体仅覆盖
ASCII，缺真实字体中文必然渲染为方块，故不静默降级。

## 与 PHP 版并排对照

`php php-canvas-image-renderer/scripts/visual-check.php out.png` 产出对照图。**布局级一致**
（断行行数、行内容、盒子坐标），逐项差异均为预期：

- 品牌字样与二维码内容为 go-canvas（标题/页脚/QR 内容，有意改写）；段落、表格、几何逐项同 PHP。
- 文本 y 基线差异按 ADR-0003 预期：PHP `getTextOrigin()` 的 GD 基线魔数 `- round(fontSize*0.1)`
  不移植，基线差由后端 `drawText` 用字体 metrics 消化（布局快照轨已单测钉死）。
- 字形观感可不同：双端实选字体文件可能不同（见上节），但默认度量器为启发式（与字体无关），
  断行结果不受影响。
- 像素不求等：GD 与 Go 位图引擎的分平台像素差异本就不在对等承诺内。
