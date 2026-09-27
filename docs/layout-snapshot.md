# 布局快照 fixture(双端布局回归保障)

PHP 导出脚本(`php-canvas-next/scripts/export-layout-snapshot.php`)对固定用例集产出布局快照
JSON,提交进本仓库 `renderer/testdata/layout-snapshot.json`,作为双端布局的**单一事实源**;
Go 测试(`renderer/layoutsnapshot_test.go`)读取快照,解码 graph 重建画布、驱动 Go 渲染模板
(录制假后端),逐记录断言。两端排版漂移在 CI 前被人眼可见的方式拦截——**快照 diff 即双端
布局行为 diff**。

快照每个用例含三部分:

- `graph`:输入。PHP `Canvas::graph()` 产物,取**重放一次的规范形**(见下文「往返不动点」);
- `records`:期望绘制记录。PHP 驱动真实 `AbstractRenderer` 模板录制的原语调用(paint 序,
  即 priority 降序;`rect` 盒坐标 / `text` 行与坐标 / `image` 盒坐标);
- `expectedYDiff`(text 记录):预期差异标注,见下文。

## 断言规则

**逐字段严格一致**:op 序列、盒子坐标 (x, y, w, h)、文本行数、行内容、行 x。

**预期差异字段——文本行 y**:PHP `getTextOrigin()` 的 bottom+autowrap 分支含 GD 基线魔数
`- round(fontSize * 0.1)`,Go 按 ADR-0003 不移植(布局层返回纯对齐锚点,基线差由渲染后端
`drawText` 用字体 metrics 消化)。快照中该偏移按记录标注为 `expectedYDiff`(其余分支恒 0),
Go 断言 `gotY == y + expectedYDiff`——即按 Go 自身纯对齐锚点语义验证,同时把双端分歧钉死在
文档化的魔数上:任一端改了这条分支(或加了别的偏移),测试即红。

**graph wire 互通**:Go 解码快照 graph → 重建画布 → `Graph()` 再序列化,须与输入逐字段一致。
比较前两侧各规整为 `any` 深比,消除 PHP pretty-print 缩进与浮点字面(`1.0` vs `1`)差异;
键集、键值与嵌套结构仍逐字段锁定。

**往返不动点**:构造态 graph 对「auto 单元格采纳内容高」的形状不是往返恒等
(`fromGraph→graph ≠ graph`,内容层被压平;PHP/Go 两端同款语义,均有单测锁定各自的
add 副作用)。导出脚本把用例重放一次(`Canvas::fromGraph($canvas->graph())`)取重建态
作为规范形——输入 graph、Go 解码再序列化、绘制记录三者同源,才可逐字段可比。

## 用例集

单点定义于导出脚本的 `buildCases()`,Go 侧不重复定义(输入漂移由 graph 比对自动拦截):

| 用例 | 覆盖 |
| --- | --- |
| `cjk-autowrap` | CJK 逐字断行、显式换行保留空行、padding、行高 1.4、autoHeight、预期差异分支 |
| `kinsoku-word-boundary` | 行首禁则上移、行末禁则下移、空格词边界、连字符保留行尾、无边界长词硬断、断点行尾空白剔除 |
| `emoji-mixed` | emoji 与中文混排断行 |
| `align-origin-grid` | 水平×垂直九种对齐组合、center×autoHeight、bottom×autowrap 固定高多行 |
| `anchors-containers` | 九锚点摆位、负偏移溢出、priority 渲染次序、表格嵌套(行堆叠/单元格横排/嵌套表/auto 高度传播) |
| `table-template-wire` | TableLayer V2 模板态 wire:template 键、data.rowsPath、表达式标记三键/两键形态 |
| `name-visible-contract` | 图层 name/visible 契约(layer-panel-ux 工单 01):name 仅非空写键、visible 仅 false 写键、hidden 根层渲染跳过(零绘制记录) |

**emoji 只用单码点字素**:`Uax14LineBreaker` 以字素簇为最小单元,本机 PHP 有 ext-intl
(字素簇切分),Go 核心默认码点切分——ZWJ 组合序列在两条默认切分路径下断行不同。该差异
属注入缝(等价 PHP 无 ext-intl 的降级路径 vs 正常路径),由 Go 单测的字素桩与增强实现
(typography 包)锁定;快照要求「对齐版默认实现下逐字段一致」,用例集必须保证两种切分
路径断行一致,故不纳入 ZWJ 序列。

## 何时需要重导(双端同步规则)

任一端改了**布局可观察面**,或**用例集**变化,即须重导快照并双端同步:

- 断行器(`Uax14LineBreaker`)/度量器(`HeuristicMeasurer`)、字素切分默认路径;
- `TextOrigin` / 锚点解析(`PositionResolver` / `ResolveAnchor`)、渲染模板遍历/下钻坐标;
- 表格 add 副作用、auto 尺寸语义、默认值(对齐、行高、盒模型、priority 排序);
- 用例集本身(只在导出脚本里改);
- `getTextOrigin` 的 GD 基线魔数分支(会体现在 `expectedYDiff` 上)。

流程:

1. 改动前确认基线绿:`go test ./renderer/ -run TestLayoutSnapshot`;
2. 行为改动落地后重导:`make layout-snapshot`(需本机 PHP 8.3 + ext-intl;Go CI 不装
   PHP,快照永远提交进仓库);
3. `git diff` 逐行人审——每行 diff 都应能对应到一个有意的行为变化(这就是「人眼可见」
   的拦截面),确认后快照与双端代码随同一提交序列合入;
4. 另一端跑全量 `make test` / `make vet` 验证。

快照 `meta.notes` 内置了上述关键口径(契约版本 `layout-snapshot v1`);改契约本身须同步
本文档、导出脚本与 Go 侧断言。
