.PHONY: build build-cli run restart clean clean-data test lint fmt vet tidy generate apidoc docker-build install-skills pack-npm build-npm

APP_NAME := git-ferry
CLI_NAME := gitferry
BUILD_DIR := ./output
VERSION_PKG := github.com/yi-nology/git-ferry/internal/version
# 版本号编译时注入:默认取 git describe(tag 或 commit),可用 `make build VERSION=v1.7.1` 覆盖
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-X $(VERSION_PKG).Version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) .
	@echo ">> built $(BUILD_DIR)/$(APP_NAME) (version $(VERSION))"

# CLI（人 + Agent 入口），见 skills/ 与 docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md
build-cli:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-X main.Version=$(VERSION)" -o $(BUILD_DIR)/$(CLI_NAME) ./cmd/gitferry
	@echo ">> built $(BUILD_DIR)/$(CLI_NAME) (version $(VERSION))"

# 把 Agent Skills 装到全局（需 npx skills；无则只打印路径）
install-skills:
	@if command -v npx >/dev/null 2>&1; then npx skills add ./skills -y -g; else echo "skills/ 已就绪，请手动复制到 Agent skills 目录"; fi

# 本地 npm 打包自测（二进制塞进 npm-pkg 并 npm pack）
pack-npm:
	@bash scripts/pack-local.sh

# 只组装 npm 发布目录（不含本地二进制；发布流水线用）
build-npm:
	@bash scripts/build-npm.sh

# 本地开发启动:自动加载 .env(ENCRYPTION_KEY 等本地密钥,已被 gitignore)。
# 注意必须 `go run .` 整包编译,不能 `go run main.go`(后者只编译单文件,会报 undefined: register)
run:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; go run .

# 一键重启:先停掉占用 8890 的旧后端(连同 go run 包装进程),再以当前代码重新编译启动。
restart:
	@pid=$$(lsof -ti tcp:8890 2>/dev/null); \
	if [ -n "$$pid" ]; then \
		echo ">> stopping old backend (pid $$pid) ..."; \
		pkill -f "go run \.$$" 2>/dev/null || true; \
		kill $$pid 2>/dev/null || true; \
		sleep 2; \
		kill -9 $$pid 2>/dev/null || true; \
	fi
	@$(MAKE) run

# 依赖 github.com/yi-nology/git-sync-core(go.mod,可从模块代理拉取)。
# 本地改 core 时可用 go.work 或临时 replace,勿提交 replace。
tidy:
	@go mod tidy

test:
	@go test ./... -race -count=1

lint:
	@golangci-lint run ./...

fmt:
	@gofmt -s -w .

vet:
	@go vet ./...

# clean 只清编译产物;开发数据库用 clean-data 单独清,防止误删
clean:
	@rm -rf $(BUILD_DIR)

clean-data:
	@rm -rf data/

generate:
	@echo "Generating code from IDL..."
	@cd idl && thriftgo -r -g "go:package_prefix=github.com/yi-nology/git-ferry/biz" --out ../biz git_sync.thrift
	@hz model --idl idl/ops.thrift --module github.com/yi-nology/git-ferry --model_dir biz/model --snake_tag
	@hz model --idl idl/ai.thrift --module github.com/yi-nology/git-ferry --model_dir biz/model --snake_tag
	@hz model --idl idl/mirror.thrift --module github.com/yi-nology/git-ferry --model_dir biz/model
	@rm -rf biz/base biz/git_sync biz/operation_log biz/platform biz/repo biz/sync_task biz/system biz/webhook biz/ops biz/ai biz/mirror
	@echo ">> generated biz/model/{ops,ai,mirror} via hz"

# 从 IDL 生成 OpenAPI 3.0 spec → docs/openapi.json(同时更新内嵌副本)
apidoc:
	@go run ./cmd/apidoc idl docs/openapi.json
	@cp docs/openapi.json internal/pkg/swagger/openapi.json
	@echo ">> generated docs/openapi.json + internal/pkg/swagger/openapi.json"

docker-build:
	@docker build --build-arg VERSION=$(VERSION) -t $(APP_NAME):latest .
