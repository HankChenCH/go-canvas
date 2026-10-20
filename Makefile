# go-canvas 工作区命令
#
# 语义语料由 submodule canvas-semantics pin 消费(fresh clone 先
# git submodule update --init,CI checkout 已带 submodules);语料 diff 即语义变更,
# 升级 = 一次可审的 submodule 指针移动,无本地再生成步骤。
#
# layout-snapshot 是 wire 字节面(非语料),仍需 PHP 8.3 + ext-intl:快照 JSON 提交
# 进本仓库,仅人工触发再生成——何时需要重导、双端同步规则见 docs/layout-snapshot.md。

# 解析 PHP 8.3:PATH 上的 php@8.3 → Homebrew keg-only 路径 → 裸 php(过旧时报错由
# vendor 平台检查给出)
BREW_PREFIX := $(shell brew --prefix 2>/dev/null)
PHP := $(shell command -v php@8.3 2>/dev/null)
ifeq ($(PHP),)
ifneq ($(wildcard $(BREW_PREFIX)/opt/php@8.3/bin/php),)
  PHP := $(BREW_PREFIX)/opt/php@8.3/bin/php
else
  PHP := php
endif
endif

.PHONY: test vet layout-snapshot visual-check visual-check-bitmap visual-check-pdf

test:
	go test ./...
	cd image-renderer && go test ./...
	cd pdf-renderer && go test ./...

vet:
	go vet ./...
	cd image-renderer && go vet ./...
	cd pdf-renderer && go vet ./...

layout-snapshot:
	$(PHP) ../php-canvas-next/scripts/export-layout-snapshot.php renderer/testdata/layout-snapshot.json
	@echo "快照已更新:人审 git diff(diff 即双端布局行为 diff)后随代码一并提交"

# 目验脚本:渲染综合样图供人工目验(工单 11,用法与目验要点见 docs/visual-check.md)。
# OUTPUT= 语义逐后端钉死,复合目标只做编排、不收 OUTPUT(两后端取值形态不同,
# 复合传参会串义,传了即报错并指路):
#   visual-check-bitmap:OUTPUT = 完整 png 路径;缺省 image-renderer/visual-check.png(已 gitignore)
#   visual-check-pdf:OUTPUT = 产物基名,同时出 <基名>.pdf + <基名>.png 预览
#     (poppler 四件套,依赖见 pdf-renderer/AGENTS.md);缺省基名 visual-check
visual-check:
	@if [ -n "$(OUTPUT)" ]; then \
		echo "visual-check 是复合编排目标:两后端 OUTPUT 语义不同(png 全路径 vs pdf 产物基名),请分别跑 visual-check-bitmap / visual-check-pdf 传参" >&2; \
		exit 2; \
	fi
	$(MAKE) visual-check-bitmap
	$(MAKE) visual-check-pdf

visual-check-bitmap:
	cd image-renderer && go run ./cmd/visualcheck $(OUTPUT)

# PDF 目验:综合样张出 .pdf + pdftoppm 预览 png(poppler 四件套,依赖见
# pdf-renderer/AGENTS.md);缺省产物 pdf-renderer/visual-check.pdf/.png
visual-check-pdf:
	cd pdf-renderer && go run ./cmd/visualcheck $(OUTPUT)
