# AI 运维平台 - Go 后端常用命令集合
# 使用方式：在仓库根目录执行 `make <target>`，例如 `make run`。

APP_NAME       := aiops-api
PKG            := github.com/734965549/aiops
BUILD_DIR      := bin
VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT         ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_AT       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS        := -s -w \
	-X '$(PKG)/internal/version.Version=$(VERSION)' \
	-X '$(PKG)/internal/version.Commit=$(COMMIT)' \
	-X '$(PKG)/internal/version.BuildAt=$(BUILD_AT)'

CONFIG         ?= configs/config.yaml

.PHONY: help tidy fmt fmt-check vet lint web-lint test build run migrate docker docker-prod clean

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

tidy: ## 整理依赖
	go mod tidy

fmt: ## 格式化 Go 源码（cmd/internal/pkg）
	gofmt -w cmd internal pkg

fmt-check: ## 检查 gofmt 是否已格式化
	@test -z "$$(gofmt -l cmd internal pkg)" || (gofmt -l cmd internal pkg && exit 1)

vet: ## 静态检查
	go vet ./...

web-lint: ## 前端 ESLint
	cd web && npm run lint

lint: fmt-check vet web-lint ## 工程检查（gofmt + go vet + 前端 eslint）

test: ## 单元测试（仅 cmd/internal/pkg，避免扫入 web/node_modules）
	go test ./cmd/... ./internal/... ./pkg/... -race -count=1

build: ## 构建二进制
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/api

run: ## 本地运行 API（默认配置 configs/config.yaml，注入版本信息）
	go run -ldflags "$(LDFLAGS)" ./cmd/api -config $(CONFIG)

migrate migrate-up: ## 执行数据库迁移（自研 runner，见 ops/migration-contract.md）
	go run ./cmd/migrate -config $(CONFIG)

docker: ## 构建 docker 镜像（标签 aiops-api:$(VERSION)，与 compose 中 AIOPS_VERSION 对齐）
	docker build -f deployments/Dockerfile -t $(APP_NAME):$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_AT=$(BUILD_AT) .

docker-prod: ## 生产构建 docker 镜像：拒绝 latest/dev/空/dirty/unknown/none 等不可追溯标签
	@if [ "$(VERSION)" = "latest" ] || [ "$(VERSION)" = "dev" ] || [ -z "$(VERSION)" ]; then \
		echo "ERROR: VERSION='$(VERSION)' 不允许用于生产构建；请使用不可变版本号（如 v1.2.0）或 digest（repo@sha256:...）。"; \
		exit 1; \
	fi
	@case "$(VERSION)" in *-dirty) \
		echo "ERROR: VERSION='$(VERSION)' 含 -dirty，工作区未提交变更；请先 commit/tag 后再构建。"; \
		exit 1;; esac
	@if [ "$(VERSION)" = "unknown" ] || [ "$(COMMIT)" = "none" ] || [ "$(BUILD_AT)" = "unknown" ]; then \
		echo "ERROR: 构建元数据不完整（VERSION/COMMIT/BUILD_AT）；请通过 CI 或显式传入 VERSION/COMMIT/BUILD_AT。"; \
		exit 1; \
	fi
	docker build -f deployments/Dockerfile -t $(APP_NAME):$(VERSION) \
		--build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_AT=$(BUILD_AT) .
	@echo "提示: push 后以 digest 填入 AIOPS_IMAGE，并运行 scripts/verify-prod-version.ps1 校验。"

clean: ## 清理构建产物
	rm -rf $(BUILD_DIR)
