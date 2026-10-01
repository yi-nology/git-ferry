# 贡献指南

感谢你对 GitFerry（摆渡）的兴趣！本文件说明如何参与开发与提交变更。

[English contributing guide](CONTRIBUTING.md)

## 快速开始

```bash
git clone https://github.com/yi-nology/git-ferry.git
cd git-ferry
make tidy
make test
make build        # 服务端 git-ferry
make build-cli    # CLI gitferry
```

依赖：Go 1.26+；前端开发另需 Node.js 20+（`frontend/`）。

## 开发流程

1. Fork 并建分支：`git checkout -b feature/my-change`
2. 改代码；保持 `make test` / `make lint` / `make vet` 通过
3. 涉及 API 时跑 `make apidoc` 更新 `docs/openapi.json`
4. 涉及 CLI / Skills 时同步 `skills/` 与 `docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md` 中的命令表
5. 提交 PR，填写模板

## 代码结构速览

| 路径 | 职责 |
|------|------|
| `biz/` | hz 生成的路由/handler（壳层） |
| `internal/` | 壳层内部包（agent、ops、notify…） |
| `cmd/gitferry/` + `internal/cli/` | CLI |
| `skills/` | AI Agent Skills |
| `frontend/` | Vue 3 控制台 |
| `idl/` | thrift IDL |

同步引擎在 [git-sync-core](https://github.com/yi-nology/git-sync-core)，本仓以 Go module 依赖；联调未发布 core 用本地 `go.work`，**不要提交 `replace`**。

## 提交规范

- 标题用祈使句：`Add task retry shortcut` / `Fix webhook debounce`
- 一个 PR 聚焦一件事；大重构拆多个 PR
- 用户可见变更写进 `CHANGELOG.md` 的 `[Unreleased]`
- 不提交密钥、`.env`、`data/`、本地 `go.work`

## 测试

```bash
make test           # go test ./... -race
make lint           # golangci-lint
cd frontend && npm ci && npm run build && npx vue-tsc -b
```

新功能优先补测试；修 bug 补回归用例。

## 文档

- 架构：`ARCHITECTURE.md`
- API：`docs/openapi.json`（`make apidoc` 生成）
- CLI / Skills 设计：`docs/superpowers/plans/2026-09-30-agent-native-cli-skills.md`
- 双语 README：改 `README.md` 时同步 `README.zh-CN.md`

## 行为准则

保持尊重与建设性。骚扰、歧视或恶意行为将不被接受。

## 许可证

贡献默认以 [MIT License](LICENSE) 授权。
