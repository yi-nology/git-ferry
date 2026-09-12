.PHONY: build run restart clean test tidy generate docker-build

APP_NAME := git-sync-service
BUILD_DIR := ./output
VERSION_PKG := github.com/yi-nology/git-sync-service/internal/version
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

# 本地依赖同级仓 ../git-sync-core（go.mod replace）。core 已发布 tag 后可去掉 replace。
tidy:
	@go mod tidy

test:
	@go test ./... -race -count=1

clean:
	@rm -rf $(BUILD_DIR) data/

generate:
	@echo "Generating code from IDL..."
	@cd idl && thriftgo --out ../biz --go --go-recurse 10 git_sync.thrift

# 在父目录（含 git-sync-core）执行：make -C git-sync-service docker-build
docker-build:
	@cd .. && docker build -f git-sync-service/Dockerfile -t $(APP_NAME):latest .
