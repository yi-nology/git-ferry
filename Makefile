.PHONY: build run restart clean clean-data test lint fmt vet tidy generate apidoc docker-build

APP_NAME := git-ferry
BUILD_DIR := ./output
VERSION_PKG := github.com/yi-nology/git-ferry/internal/version
# 版本号编译时注入:默认取 git describe(tag 或 commit),可用 `make build VERSION=v1.7.1` 覆盖
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	@mkdir -p $(BUILD_DIR)
	@go build -ldflags "-X $(VERSION_PKG).Version=$(VERSION)" -o $(BUILD_DIR)/$(APP_NAME) .
	@echo ">> built $(BUILD_DIR)/$(APP_NAME) (version $(VERSION))"

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

# 从 IDL 生成 OpenAPI 3.0 spec → docs/openapi.json(同时更新内嵌副本)
apidoc:
	@go run ./cmd/apidoc idl docs/openapi.json
	@cp docs/openapi.json internal/pkg/swagger/openapi.json
	@echo ">> generated docs/openapi.json + internal/pkg/swagger/openapi.json"

docker-build:
	@docker build --build-arg VERSION=$(VERSION) -t $(APP_NAME):latest .
