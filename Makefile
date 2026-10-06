# SSR —— 常用任务的固定入口
#
# 只做"把已有命令收拢成一行"，不藏参数、不改变行为；每条都能在 AGENTS.md §4 找到对应说明。
# 用法：make 或 make help 看全部目标。

SHELL := /bin/bash
.DEFAULT_GOAL := help
FRONTEND := frontend

.PHONY: help build test check fmt vet frontend frontend-test frontend-install dev preview local package version clean

help: ## 显示所有目标
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## 编译全部 Go 包
	go build ./...

test: ## 全部测试（提交前必跑）
	go test -count=1 ./...

check: ## 提交前的完整校验：Go 测试 + 前端测试 + 生成物一致 + gofmt + vet
	$(MAKE) test
	$(MAKE) frontend-test
	go run ./cmd/ssr-core genlogcolumns --check --config config
	$(MAKE) fmt
	$(MAKE) vet

fmt: ## 只检查格式（不修改文件；要修就自己跑 gofmt -w）
	@files="$$(git ls-files '*.go' | while read -r f; do [ -f "$$f" ] && echo "$$f"; done)"; \
	if [ -z "$$files" ]; then echo "没有需要检查的 Go 文件"; exit 0; fi; \
	out="$$(gofmt -l $$files)"; \
	if [ -n "$$out" ]; then echo "以下文件未格式化，请运行 gofmt -w："; echo "$$out"; exit 1; fi; \
	echo "gofmt 干净"

vet: ## go vet 静态检查
	go vet ./...

frontend-install: ## 安装前端依赖（npm ci）
	npm ci --prefix $(FRONTEND)

frontend: ## 前端类型检查与构建（tsc --noEmit + vite build）
	npm run build --prefix $(FRONTEND)

frontend-test: ## 前端单元测试（vitest + jsdom，测真实渲染与交互）
	npm test --prefix $(FRONTEND)

dev: ## 开发模式：Wails 热重载，接真实后端
	wails dev

preview: ## 只看界面：构建前端并起本地预览（模拟 Wails 桥接）
	./scripts/preview.sh --build

local: ## 刷新本地运行包 release/SingleSourceReady/（允许脏工作区）
	./scripts/release-local.sh

package: ## 本地自测发布包组装（不压缩、允许脏工作区）
	go run ./cmd/ssr-core release --allow-dirty --no-zip

version: ## 打印程序版本（来自 git tag / 构建期信息）
	go run ./cmd/ssr-core version

clean: ## 清掉本地构建产物（不动 release/ 运行包与数据）
	rm -rf build/local-publish build/bin
