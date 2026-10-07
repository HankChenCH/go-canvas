package typography

// UAX #29 字素簇切分:go-text/typesetting GraphemeIterator 适配核心 text.Segmenter
// 接缝(研究文档 §4.3 选型)。注入 text.Uax14LineBreaker.WithSegmenter 即补齐核心
// 默认码点切分的降级缺口(ZWJ 组合序列/regional pair/组合字符按单簇计宽与断行,
// 等价 PHP 有 ext-intl 的正常路径)。

import (
	"github.com/go-text/typesetting/segmenter"

	"github.com/HankChenCH/go-canvas/text"
)

// GraphemeSegmenter UAX #29 字素簇切分器。零状态,值类型可共享;
// Split 每次调用用独立的局部 Segmenter,多 goroutine 并发调用安全。
type GraphemeSegmenter struct{}

// NewGraphemeSegmenter 构造字素簇切分器
func NewGraphemeSegmenter() *GraphemeSegmenter { return &GraphemeSegmenter{} }

// Split implements text.Segmenter:切为字素簇序列,空文本返回空序列。
// typesetting 拒绝非法 UTF-8(InitWithString 报错),此时降级核心码点切分
// (等价 PHP 无 ext-intl 的降级路径;JSON wire 面的正常输入不可达)
func (GraphemeSegmenter) Split(s string) []string {
	if s == "" {
		return nil
	}

	var seg segmenter.Segmenter
	if err := seg.InitWithString(s); err != nil {
		return text.CodePointSegmenter{}.Split(s)
	}

	it := seg.GraphemeIterator()
	clusters := make([]string, 0, len(s))
	for it.Next() {
		clusters = append(clusters, string(it.Grapheme().Text))
	}
	return clusters
}
