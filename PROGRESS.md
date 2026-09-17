# GitFerry 开发进度

## ✅ 已完成

### 1. 项目架构
- [x] Hertz + Thrift IDL 代码生成架构
- [x] 标准目录结构（idl/, biz/, internal/）
- [x] 双数据库驱动支持（SQLite/MySQL）
- [x] 配置文件（conf/config.yaml）

### 2. API 接口（24 个端点）
| 模块 | 端点 | 状态 |
|---|---|---|
| **Repo** | `GET /api/v1/repos` - 仓库列表 | ✅ |
| | `GET /api/v1/repo` - 仓库详情 | ✅ |
| | `POST /api/v1/repo/create` - 创建仓库 | ✅ |
| | `POST /api/v1/repo/update` - 更新仓库 | ✅ |
| | `POST /api/v1/repo/delete` - 删除仓库 | ✅ |
| | `POST /api/v1/repo/test` - 测试连接 | ✅ |
| | `GET /api/v1/repo/branches` - 分支列表 | ✅ |
| **SyncTask** | `GET /api/v1/sync/tasks` - 任务列表 | ✅ |
| | `GET /api/v1/sync/task` - 任务详情 | ✅ |
| | `POST /api/v1/sync/task/create` - 创建任务 | ✅ |
| | `POST /api/v1/sync/task/update` - 更新任务 | ✅ |
| | `POST /api/v1/sync/task/delete` - 删除任务 | ✅ |
| | `POST /api/v1/sync/task/run` - 执行任务 | ✅ |
| | `POST /api/v1/sync/preview` - 同步预览 | ✅ |
| | `GET /api/v1/sync/history` - 同步历史 | ✅ |
| **Webhook** | `GET /api/v1/webhook/rules` - 规则列表 | ✅ |
| | `GET /api/v1/webhook/rule` - 规则详情 | ✅ |
| | `POST /api/v1/webhook/rule/create` - 创建规则 | ✅ |
| | `POST /api/v1/webhook/rule/update` - 更新规则 | ✅ |
| | `POST /api/v1/webhook/rule/delete` - 删除规则 | ✅ |
| | `GET /api/v1/webhook/events` - 事件列表 | ✅ |
| | `POST /api/v1/webhook/event/retry` - 重试事件 | ✅ |
| | `POST /api/v1/webhook/receive/:repoKey` - 接收 Webhook | ✅ |
| **Health** | `GET /ping` - 健康检查 | ✅ |

### 3. 核心业务逻辑
- [x] 数据模型（Repo, SyncTask, SyncRun, WebhookRule, WebhookEvent）
- [x] DAO 层 CRUD 操作
- [x] 同步任务执行器（Git clone + push）
- [x] CRON 任务调度
- [x] Webhook 规则匹配引擎
- [x] Git Provider 接口抽象

### 4. Git Platform SDK 集成
- [x] 对接 `github.com/yi-nology/git-platform-sdk`
- [x] 替换 MockProvider 为真实 SDK Provider
- [x] 支持 6 个平台（GitHub, GitLab, Gitea, Gitee, Forgejo, 腾讯代码托管）
- [x] Webhook 签名验证
- [x] 事件规范化解析

### 5. 分布式锁（Redis）
- [x] Redis 分布式锁实现
- [x] 信号量并发控制
- [x] 本地锁回退（无 Redis 时）
- [x] 任务锁防重入

### 6. 增量同步优化
- [x] 首次 clone 使用 `--depth 1` 浅克隆
- [x] 后续同步使用 `fetch` 增量更新
- [x] 自动检测仓库是否存在决定 clone/fetch
- [x] 失败重试机制（指数退避）
- [x] 并发控制和超时管理

### 7. 构建测试
- [x] 编译通过
- [x] 服务正常启动（端口 8890）
- [x] API 端点响应正常

---

## 📋 待完成

### 1. 生产环境准备
- [ ] 认证中间件（JWT/API Key）
- [ ] 接口限流
- [ ] 请求日志
- [ ] Prometheus 监控指标
- [ ] 单元测试覆盖
- [ ] Docker 容器化
- [ ] docker-compose 编排

### 2. 功能增强
- [ ] WebSocket 实时状态推送
- [ ] 前端 API 联调
- [ ] 批量同步支持
- [ ] 同步冲突检测和处理
- [ ] Webhook 自动注册（创建 repo 时自动配置）

---

## 🚀 启动命令

```bash
# 开发环境启动
make run

# 编译
make build

# 依赖整理
make tidy

# API 测试
./test_api.sh
```

---

## 📌 架构说明

```
git-ferry/
├── idl/                          # Thrift API 定义
├── biz/
│   ├── handler/git_sync/        # Handler（调用 service 层）
│   ├── model/                   # 生成的请求/响应模型
│   └── router/                  # 生成的路由注册
├── internal/sync/               # 核心业务逻辑层
│   ├── model/                   # GORM 数据模型
│   ├── dao.go                   # DAO 数据访问层
│   ├── service.go               # Service 入口
│   ├── repo.go                  # 仓库管理
│   ├── task.go                  # 同步任务管理
│   ├── executor.go              # Git 执行器（增量同步）
│   ├── webhook.go               # Webhook 处理 + 规则引擎
│   ├── provider_manager.go      # Git Provider 管理（SDK 集成）
│   ├── lock.go                  # 分布式锁（Redis/Local）
│   └── config.go                # 配置管理
├── main.go                      # 服务入口
├── router.go                    # 自定义路由注册
└── conf/config.yaml             # 配置文件
```
