# go-canvas 工作区命令
#
# layout-snapshot 需 PHP 8.3 + ext-intl。Go CI 不装 PHP:快照 JSON 提交进本仓库,
# 仅人工触发再生成——何时需要重导、双端同步规则见 docs/layout-snapshot.md。

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

.PHONY: test vet layout-snapshot

test:
	go test ./...
	cd image-renderer && go test ./...

vet:
	go vet ./...
	cd image-renderer && go vet ./...

layout-snapshot:
	$(PHP) ../php-canvas-next/scripts/export-layout-snapshot.php renderer/testdata/layout-snapshot.json
	@echo "快照已更新:人审 git diff(diff 即双端布局行为 diff)后随代码一并提交"
