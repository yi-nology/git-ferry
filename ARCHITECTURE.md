# Git Sync Service 架构设计文档

## 1. 项目概述

### 1.1 服务命名
**git-sync-service** - 独立的 Git 仓库同步微服务

### 1.2 三仓架构

| 仓库 | Import path | 角色 | 开源策略 |
|------|-------------|------|----------|
| `git-sync-core` | `github.com/yi-nology/git-sync-core` | 同步引擎（无 HTTP） | 公网开源 |
| `git-sync-service`（本仓） | `github.com/yi-nology/git-sync-service` | 公网壳：hz API + Vue | 公网开源 |
| `git-sync-intranet` | `github.com/yi-nology/git-sync-intranet` | 内网壳：网关身份头/SSO 钩子 | 可闭源 |

本地开发将三仓放在同一父目录，用 `go.mod` 的 `replace` 指向同级路径（core 发布 tag 后可改版本依赖）。

```
模式 A: 公网独立服务                 模式 B: 作为库              模式 C: 内网壳
┌────────────────────────┐      ┌──────────────────────┐   ┌────────────────────────┐
│ git-sync-service       │      │  git-manage-service  │   │ git-sync-intranet      │
│  main / biz / frontend │      │  ┌────────────────┐  │   │  auth(proxy/SSO hook)  │
│         │              │      │  │  自有 handler  │  │   │  main                  │
│         ▼              │      │  └───────┬────────┘  │   │    │                   │
│  git-sync-core         │◄─────┤          ▼           │   │    ▼                   │
│  model/service/…       │      │  git-sync-core       │   │  biz/serve（复用路由）  │
│         │              │      └──────────────────────┘   │    │                   │
│         ▼              │                                 │    ▼                   │
│  git-platform-sdk      │                                 │  git-sync-core         │
└────────────────────────┘                                 └────────────────────────┘
```

内网差异只放在 `git-sync-intranet/auth`（`SetAuthMiddlewareProvider`）与配置默认值，不复制业务逻辑。

### 1.3 设计原则

| 原则 | 说明 |
|------|------|
| **完全独立** | core 不依赖壳与 git-manage-service，自包含数据模型 |
| **单一依赖** | core 只依赖 git-platform-sdk 做 Git 平台操作 |
| **可复用库** | 独立仓 `git-sync-core`，外部 `require github.com/yi-nology/git-sync-core` |
| **壳不渗入** | hz handler / response / converter / version 留在本仓 |
| **hz 标准** | HTTP 层遵循 hz IDL 代码生成规范 |

### 1.4 核心功能

1. **同步任务管理** - CRUD 操作、启停控制
2. **同步执行** - 手动触发、定时任务、批量同步
3. **Webhook 触发** - 事件驱动的同步触发
4. **规则引擎** - 分支模式匹配、防抖控制
5. **同步历史** - 运行记录、日志查看

## 2. 目录结构

```
my_project/
├── git-sync-core/                   # 同步引擎库（独立仓）
│   ├── go.mod                       # github.com/yi-nology/git-sync-core
│   ├── sync.go                      # 库入口 (package sync)
│   ├── model/  service/  executor/  dao/  lock/
│
├── git-sync-service/                # 本仓：公网壳
│   ├── go.mod                       # replace core => ../git-sync-core
│   ├── main.go
│   ├── router.go / router_gen.go
│   ├── biz/
│   │   ├── handler/git_sync/
│   │   ├── model/                   # thrift 生成请求/响应
│   │   ├── router/                  # 路由 + 可替换鉴权 Provider
│   │   └── serve/                   # HTTP 启动（内网壳亦复用）
│   ├── internal/                    # converter / response / version
│   ├── frontend/
│   ├── idl/
│   └── conf/config.yaml
│
└── git-sync-intranet/               # 内网壳（独立仓）
    ├── go.mod                       # replace core + service => 同级路径
    ├── main.go
    ├── auth/                        # proxy 头 / API Key / SSO 钩子
    └── conf/config.yaml
```

## 3. 核心库 API 设计

### 3.1 对外暴露的接口

```go
// core/sync.go - 核心库入口（仓 git-sync-core）

package sync

// Service 同步服务核心
type Service struct {
    config      *Config
    repoDAO     *RepoDAO
    taskDAO     *SyncTaskDAO
    runDAO      *SyncRunDAO
    ruleDAO     *WebhookRuleDAO
    eventDAO    *WebhookEventDAO
    providerMgr *ProviderManager
    gitBackend  gitbackend.GitBackend
    lockSvc     lock.DistLock
}

// NewService 创建服务实例
func NewService(cfg *Config) (*Service, error)

// ===== 同步任务 API =====

func (s *Service) ListTasks(repoKey string) ([]*SyncTask, error)
func (s *Service) GetTask(key string) (*SyncTask, error)
func (s *Service) CreateTask(req *CreateTaskRequest) (*SyncTask, error)
func (s *Service) UpdateTask(req *UpdateTaskRequest) (*SyncTask, error)
func (s *Service) DeleteTask(key string) error
func (s *Service) RunTask(ctx context.Context, taskKey string) error
func (s *Service) RunTaskWithTrigger(ctx context.Context, taskKey, trigger string) error
func (s *Service) PreviewSync(req *PreviewSyncRequest) (*PreviewSyncResult, error)

// ===== 同步历史 API =====

func (s *Service) ListHistory(taskKey string, limit int) ([]*SyncRun, error)
func (s *Service) DeleteHistory(id uint) error

// ===== 仓库管理 API =====

func (s *Service) ListRepos() ([]*Repo, error)
func (s *Service) GetRepo(key string) (*Repo, error)
func (s *Service) CreateRepo(req *CreateRepoRequest) (*Repo, error)
func (s *Service) UpdateRepo(req *UpdateRepoRequest) (*Repo, error)
func (s *Service) DeleteRepo(key string) error
func (s *Service) TestConnection(ctx context.Context, repoKey string) (*TestConnectionResult, error)
func (s *Service) ListBranches(ctx context.Context, repoKey string) ([]string, error)

// ===== Webhook 规则 API =====

func (s *Service) ListRules(repoKey string) ([]*WebhookRule, error)
func (s *Service) GetRule(id uint) (*WebhookRule, error)
func (s *Service) CreateRule(req *CreateRuleRequest) (*WebhookRule, error)
func (s *Service) UpdateRule(req *UpdateRuleRequest) (*WebhookRule, error)
func (s *Service) DeleteRule(id uint) error

// ===== Webhook 事件 API =====

func (s *Service) ReceiveWebhook(ctx context.Context, repoKey string, payload *WebhookPayload) error
func (s *Service) ListEvents(repoKey string, limit int) ([]*WebhookEvent, error)
func (s *Service) RetryEvent(ctx context.Context, eventID uint) error

// ===== 生命周期 =====

func (s *Service) Start() error
func (s *Service) Stop()
```

### 3.2 使用方式

#### 模式 A: 独立部署

```go
// main.go（壳）
import synccore "github.com/yi-nology/git-sync-core"

cfg, _ := synccore.LoadConfig("conf/config.yaml")
svc, _ := synccore.NewService(cfg)
```

#### 模式 B: 作为库引用

```go
// 外部项目（如 git-manage-service）
import synccore "github.com/yi-nology/git-sync-core"

cfg := &synccore.Config{
    Database: synccore.DatabaseConfig{
        Driver: "mysql",
        DSN:    "user:pass@tcp(127.0.0.1:3306)/git_manage",
    },
    GitBackend: "gogit",
}

svc, _ := synccore.NewService(cfg)
svc.Start()
svc.RunTask(ctx, taskKey)
```

## 4. IDL 定义 (Thrift + hz 规范)

### 4.1 同步任务接口

```thrift
// idl/sync_task.thrift
namespace go sync_task

service SyncTaskService {
    sync_task.ListTasksResp ListTasks(1: sync_task.ListTasksReq req) (api.get="/api/v1/sync/tasks")
    sync_task.GetTaskResp GetTask(1: sync_task.GetTaskReq req) (api.get="/api/v1/sync/task")
    sync_task.CreateTaskResp CreateTask(1: sync_task.CreateTaskReq req) (api.post="/api/v1/sync/task")
    sync_task.UpdateTaskResp UpdateTask(1: sync_task.UpdateTaskReq req) (api.put="/api/v1/sync/task")
    sync_task.DeleteTaskResp DeleteTask(1: sync_task.DeleteTaskReq req) (api.delete="/api/v1/sync/task")
    sync_task.RunTaskResp RunTask(1: sync_task.RunTaskReq req) (api.post="/api/v1/sync/task/run")
    sync_task.PreviewSyncResp PreviewSync(1: sync_task.PreviewSyncReq req) (api.post="/api/v1/sync/preview")
    sync_task.ListHistoryResp ListHistory(1: sync_task.ListHistoryReq req) (api.get="/api/v1/sync/history")
}
```

### 4.2 仓库管理接口

```thrift
// idl/repo.thrift
namespace go repo

service RepoService {
    repo.ListReposResp ListRepos(1: repo.ListReposReq req) (api.get="/api/v1/repos")
    repo.GetRepoResp GetRepo(1: repo.GetRepoReq req) (api.get="/api/v1/repo")
    repo.CreateRepoResp CreateRepo(1: repo.CreateRepoReq req) (api.post="/api/v1/repo")
    repo.UpdateRepoResp UpdateRepo(1: repo.UpdateRepoReq req) (api.put="/api/v1/repo")
    repo.DeleteRepoResp DeleteRepo(1: repo.DeleteRepoReq req) (api.delete="/api/v1/repo")
    repo.TestConnectionResp TestConnection(1: repo.TestConnectionReq req) (api.post="/api/v1/repo/test")
    repo.ListBranchesResp ListBranches(1: repo.ListBranchesReq req) (api.get="/api/v1/repo/branches")
}
```

### 4.3 Webhook 接口

```thrift
// idl/webhook.thrift
namespace go webhook

service WebhookService {
    webhook.ListRulesResp ListRules(1: webhook.ListRulesReq req) (api.get="/api/v1/webhook/rules")
    webhook.GetRuleResp GetRule(1: webhook.GetRuleReq req) (api.get="/api/v1/webhook/rule")
    webhook.CreateRuleResp CreateRule(1: webhook.CreateRuleReq req) (api.post="/api/v1/webhook/rule")
    webhook.UpdateRuleResp UpdateRule(1: webhook.UpdateRuleReq req) (api.put="/api/v1/webhook/rule")
    webhook.DeleteRuleResp DeleteRule(1: webhook.DeleteRuleReq req) (api.delete="/api/v1/webhook/rule")
    webhook.ListEventsResp ListEvents(1: webhook.ListEventsReq req) (api.get="/api/v1/webhook/events")
    webhook.RetryEventResp RetryEvent(1: webhook.RetryEventReq req) (api.post="/api/v1/webhook/event/retry")
}
```

### 4.4 代码生成命令

```bash
# 生成代码
make generate

# 或者手动执行
cd idl && thriftgo --out ../biz --go --go-recurse 10 git_sync.thrift
```

## 5. 数据模型

### 5.1 repos 仓库表

```sql
CREATE TABLE repos (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    `key` VARCHAR(255) UNIQUE NOT NULL COMMENT '仓库唯一标识',
    name VARCHAR(255) NOT NULL COMMENT '显示名称',
    platform VARCHAR(50) NOT NULL COMMENT 'github/gitlab/gitea/forgejo/tencent_code',
    platform_owner VARCHAR(200) NOT NULL COMMENT '平台组织/用户',
    platform_repo VARCHAR(200) NOT NULL COMMENT '平台仓库名',
    clone_url VARCHAR(500) COMMENT '克隆地址',
    ssh_url VARCHAR(500) COMMENT 'SSH 地址',
    default_branch VARCHAR(100) DEFAULT 'main' COMMENT '默认分支',
    access_token TEXT COMMENT '访问令牌 (AES 加密)',
    webhook_secret VARCHAR(255) COMMENT 'Webhook 密钥',
    webhook_id BIGINT COMMENT '平台 Webhook ID',
    status VARCHAR(20) DEFAULT 'active' COMMENT 'active/inactive/error',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_platform (platform, platform_owner, platform_repo)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 5.2 sync_tasks 同步任务表

```sql
CREATE TABLE sync_tasks (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    `key` VARCHAR(36) UNIQUE NOT NULL COMMENT '任务 UUID',
    name VARCHAR(100) NOT NULL COMMENT '任务名称',
    source_repo_key VARCHAR(255) NOT NULL COMMENT '源仓库 Key',
    source_branch VARCHAR(255) NOT NULL COMMENT '源分支',
    target_repo_key VARCHAR(255) NOT NULL COMMENT '目标仓库 Key',
    target_branch VARCHAR(255) NOT NULL COMMENT '目标分支',
    sync_mode VARCHAR(20) DEFAULT 'single' COMMENT 'single/all-branch',
    cron VARCHAR(100) COMMENT 'Cron 表达式',
    webhook_token VARCHAR(36) UNIQUE COMMENT 'Webhook 触发 Token',
    enabled BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    git_tags BOOLEAN DEFAULT FALSE COMMENT '同步标签',
    git_force BOOLEAN DEFAULT FALSE COMMENT '强制推送',
    git_prune BOOLEAN DEFAULT FALSE COMMENT '清理远程分支',
    git_no_verify BOOLEAN DEFAULT FALSE COMMENT '跳过钩子',
    push_options VARCHAR(500) COMMENT '额外推送选项',
    last_run_at TIMESTAMP NULL COMMENT '最后运行时间',
    last_status VARCHAR(20) COMMENT '最后状态',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP NULL,
    INDEX idx_source_repo (source_repo_key),
    INDEX idx_target_repo (target_repo_key),
    INDEX idx_webhook_token (webhook_token),
    INDEX idx_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 5.3 sync_runs 同步运行记录表

```sql
CREATE TABLE sync_runs (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    task_key VARCHAR(36) NOT NULL COMMENT '任务 Key',
    trigger_source VARCHAR(20) NOT NULL COMMENT 'manual/cron/webhook',
    status VARCHAR(20) NOT NULL COMMENT 'running/success/failed/conflict',
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NULL,
    commit_range VARCHAR(255) COMMENT '提交范围',
    details TEXT COMMENT '执行日志',
    error_message TEXT COMMENT '错误信息',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_key (task_key),
    INDEX idx_status (status),
    INDEX idx_start_time (start_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 5.4 webhook_rules 规则表

```sql
CREATE TABLE webhook_rules (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    name VARCHAR(100) NOT NULL COMMENT '规则名称',
    repo_key VARCHAR(255) NOT NULL COMMENT '关联仓库',
    event_type VARCHAR(100) DEFAULT 'push' COMMENT '事件类型 (glob)',
    branch_pattern VARCHAR(255) COMMENT '分支模式 (glob, 逗号分隔)',
    action VARCHAR(50) DEFAULT 'sync' COMMENT '动作: sync/notify',
    sync_task_keys TEXT COMMENT '关联任务 Key (逗号分隔)',
    min_interval INT DEFAULT 60 COMMENT '最小间隔(秒)',
    enabled BOOLEAN DEFAULT TRUE,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_repo_key (repo_key),
    INDEX idx_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 5.5 webhook_events 事件表

```sql
CREATE TABLE webhook_events (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    event_id VARCHAR(100) UNIQUE NOT NULL COMMENT '平台事件 ID',
    repo_key VARCHAR(255) COMMENT '关联仓库',
    event_type VARCHAR(50) NOT NULL COMMENT 'push/cr.opened/...',
    source VARCHAR(20) NOT NULL COMMENT 'github/gitlab/gitea/...',
    actor_name VARCHAR(200) COMMENT '操作人',
    branch VARCHAR(255) COMMENT '分支',
    commit_sha VARCHAR(40) COMMENT '提交 SHA',
    payload JSON COMMENT '事件负载',
    status VARCHAR(20) DEFAULT 'received' COMMENT 'received/processed/failed',
    error_message TEXT,
    processed_at TIMESTAMP NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_event_id (event_id),
    INDEX idx_repo_key (repo_key),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 5.6 Go 模型定义

```go
// sync/model/sync_task.go
package model

type SyncTask struct {
    ID            uint       `json:"id" gorm:"primaryKey"`
    Key           string     `json:"key" gorm:"uniqueIndex"`
    Name          string     `json:"name"`
    SourceRepoKey string     `json:"sourceRepoKey"`
    SourceBranch  string     `json:"sourceBranch"`
    TargetRepoKey string     `json:"targetRepoKey"`
    TargetBranch  string     `json:"targetBranch"`
    SyncMode      string     `json:"syncMode"`
    Cron          string     `json:"cron"`
    WebhookToken  string     `json:"webhookToken"`
    Enabled       bool       `json:"enabled"`
    GitTags       bool       `json:"gitTags"`
    GitForce      bool       `json:"gitForce"`
    GitPrune      bool       `json:"gitPrune"`
    GitNoVerify   bool       `json:"gitNoVerify"`
    PushOptions   string     `json:"pushOptions"`
    LastRunAt     *time.Time `json:"lastRunAt"`
    LastStatus    string     `json:"lastStatus"`
    CreatedAt     time.Time  `json:"createdAt"`
    UpdatedAt     time.Time  `json:"updatedAt"`
}

type Repo struct {
    ID            uint      `json:"id" gorm:"primaryKey"`
    Key           string    `json:"key" gorm:"uniqueIndex"`
    Name          string    `json:"name"`
    Platform      string    `json:"platform"`
    PlatformOwner string    `json:"platformOwner"`
    PlatformRepo  string    `json:"platformRepo"`
    CloneURL      string    `json:"cloneUrl"`
    SSHURL        string    `json:"sshUrl"`
    DefaultBranch string    `json:"defaultBranch"`
    AccessToken   string    `json:"-"`
    WebhookSecret string    `json:"-"`
    WebhookID     int64     `json:"webhookId"`
    Status        string    `json:"status"`
    CreatedAt     time.Time `json:"createdAt"`
    UpdatedAt     time.Time `json:"updatedAt"`
}

type WebhookRule struct {
    ID            uint      `json:"id" gorm:"primaryKey"`
    Name          string    `json:"name"`
    RepoKey       string    `json:"repoKey"`
    EventType     string    `json:"eventType"`
    BranchPattern string    `json:"branchPattern"`
    Action        string    `json:"action"`
    SyncTaskKeys  string    `json:"syncTaskKeys"`
    MinInterval   int       `json:"minInterval"`
    Enabled       bool      `json:"enabled"`
    CreatedAt     time.Time `json:"createdAt"`
    UpdatedAt     time.Time `json:"updatedAt"`
}

type WebhookEvent struct {
    ID           uint       `json:"id" gorm:"primaryKey"`
    EventID      string     `json:"eventId" gorm:"uniqueIndex"`
    RepoKey      string     `json:"repoKey"`
    EventType    string     `json:"eventType"`
    Source       string     `json:"source"`
    ActorName    string     `json:"actorName"`
    Branch       string     `json:"branch"`
    CommitSHA    string     `json:"commitSha"`
    Payload      []byte     `json:"payload"`
    Status       string     `json:"status"`
    ErrorMessage string     `json:"errorMessage"`
    ProcessedAt  *time.Time `json:"processedAt"`
    CreatedAt    time.Time  `json:"createdAt"`
}

type SyncRun struct {
    ID            uint       `json:"id" gorm:"primaryKey"`
    TaskKey       string     `json:"taskKey"`
    TriggerSource string     `json:"triggerSource"`
    Status        string     `json:"status"`
    StartTime     time.Time  `json:"startTime"`
    EndTime       *time.Time `json:"endTime"`
    CommitRange   string     `json:"commitRange"`
    Details       string     `json:"details"`
    ErrorMessage  string     `json:"errorMessage"`
    CreatedAt     time.Time  `json:"createdAt"`
}
```

## 6. 与 git-platform-sdk 集成

### 6.1 SDK 能力利用

| SDK 包 | 用途 | 我们实现什么 |
|--------|------|-------------|
| `provider` | Webhook 解析、签名验证、仓库/分支 API | 规则匹配、任务触发 |
| `gitbackend` | Git fetch/push 操作 | 同步执行引擎 |
| `credential` | 凭证构建、加密 | 凭证管理 |
| `branchfilter` | 分支名 glob 匹配 | 分支过滤 |

### 6.2 ProviderManager

```go
// sync/provider_manager.go
package sync

import (
    "sync"
    sdkprov "github.com/yi-nology/git-platform-sdk/provider"
)

type ProviderManager struct {
    providers map[string]sdkprov.Provider
    mu        sync.RWMutex
}

func (m *ProviderManager) GetProvider(repo *Repo) (sdkprov.Provider, error) {
    key := fmt.Sprintf("%s:%s", repo.Platform, repo.Key)

    m.mu.RLock()
    if p, ok := m.providers[key]; ok {
        m.mu.RUnlock()
        return p, nil
    }
    m.mu.RUnlock()

    p, err := sdkprov.NewProvider(sdkprov.Config{
        Platform: sdkprov.Platform(repo.Platform),
        BaseURL:  m.getBaseURL(repo),
        Token:    repo.AccessToken,
    })
    if err != nil {
        return nil, err
    }

    m.mu.Lock()
    m.providers[key] = p
    m.mu.Unlock()

    return p, nil
}
```

### 6.3 Webhook 接收

```go
// sync/webhook.go
func (s *Service) ReceiveWebhook(ctx context.Context, repoKey string, payload *WebhookPayload) error {
    repo, _ := s.repoDAO.FindByKey(repoKey)
    provider, _ := s.providerMgr.GetProvider(repo)

    // 使用 git-platform-sdk 解析事件
    event, err := provider.ParseWebhookEvent(r, repo.WebhookSecret)
    if err != nil {
        return err
    }

    // 持久化事件
    whEvent := &WebhookEvent{
        EventID:   event.ID,
        RepoKey:   repoKey,
        EventType: event.Type,
        Source:    string(event.Source),
        Branch:    event.Branch,
        CommitSHA: event.CommitSHA,
    }
    s.eventDAO.Create(whEvent)

    // 匹配规则并触发同步
    go s.applyRules(ctx, repoKey, event)
    return nil
}
```

### 6.4 分支过滤

```go
// sync/rule.go
import branchfilter "github.com/yi-nology/git-platform-sdk/branchfilter"

func matchBranch(pattern, branch string) bool {
    if pattern == "" {
        return true
    }
    bf := branchfilter.New(pattern)
    return bf.Match(branch)
}
```

## 7. 依赖关系

### 7.1 直接依赖

```
git-sync-service
    │
    ├── git-platform-sdk          # 唯一 Git 依赖
    │   ├── provider/             # Webhook 解析、平台 API
    │   ├── gitbackend/           # Git fetch/push
    │   ├── credential/           # 凭证管理、加密
    │   └── branchfilter/         # 分支过滤
    │
    ├── cloudwego/hertz           # HTTP 框架
    ├── gorm                      # ORM
    ├── redis/go-redis            # 分布式锁
    └── robfig/cron               # 定时任务
```

### 7.2 与 git-manage-service 关系

| 维度 | git-manage-service | git-sync-service |
|------|-------------------|------------------|
| 数据库 | 独立 | 独立 |
| API | 独立 | 独立 |
| Git 操作 | go-git 直接调用 | git-platform-sdk |
| Webhook | 自行解析 | git-platform-sdk |
| 凭证 | 自行管理 | git-platform-sdk |
| 平台支持 | 有限 | 5 平台完整支持 |

### 7.3 迁移路径

```
阶段 1: 引入依赖
    git-manage-service/go.mod
    require github.com/yi-nology/git-sync-service v0.1.0

阶段 2: 替换代码
    import synccore "github.com/yi-nology/git-sync-service/sync"
    svc, _ := synccore.NewService(cfg)

阶段 3: 删除旧代码
    移除 git-manage-service 中的 sync 相关代码
```

## 8. API 接口列表

| 方法 | 路径 | 描述 |
|------|------|------|
| **同步任务** | | |
| GET | /api/v1/sync/tasks | 任务列表 |
| GET | /api/v1/sync/task?key=xxx | 任务详情 |
| POST | /api/v1/sync/task | 创建任务 |
| PUT | /api/v1/sync/task | 更新任务 |
| DELETE | /api/v1/sync/task?key=xxx | 删除任务 |
| POST | /api/v1/sync/task/run | 运行任务 |
| POST | /api/v1/sync/preview | 预览同步 |
| GET | /api/v1/sync/history | 同步历史 |
| **仓库管理** | | |
| GET | /api/v1/repos | 仓库列表 |
| GET | /api/v1/repo?key=xxx | 仓库详情 |
| POST | /api/v1/repo | 创建仓库 |
| PUT | /api/v1/repo | 更新仓库 |
| DELETE | /api/v1/repo?key=xxx | 删除仓库 |
| POST | /api/v1/repo/test | 测试连接 |
| GET | /api/v1/repo/branches?key=xxx | 分支列表 |
| **Webhook** | | |
| GET | /api/v1/webhook/rules | 规则列表 |
| GET | /api/v1/webhook/rule?id=xxx | 规则详情 |
| POST | /api/v1/webhook/rule | 创建规则 |
| PUT | /api/v1/webhook/rule | 更新规则 |
| DELETE | /api/v1/webhook/rule?id=xxx | 删除规则 |
| POST | /api/webhook/receive/:repoKey | 接收 Webhook |
| GET | /api/v1/webhook/events | 事件列表 |
| POST | /api/v1/webhook/event/retry?id=xxx | 重试事件 |

## 9. 配置文件

```yaml
# conf/config.yaml

server:
  host: 0.0.0.0
  port: 8890
  mode: debug

database:
  driver: mysql
  dsn: user:pass@tcp(127.0.0.1:3306)/git_sync?charset=utf8mb4&parseTime=True
  max_idle_conns: 10
  max_open_conns: 100

redis:
  addr: 127.0.0.1:6379
  password: ""
  db: 0

git:
  backend: gogit
  temp_dir: /tmp/git-sync

sync:
  max_concurrent: 5
  default_timeout: 300
  retry_count: 3

webhook:
  rate_limit: 100

log:
  level: info
  format: json
```

## 10. 部署配置

### 10.1 Dockerfile

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o git-sync-service .

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/git-sync-service .
COPY --from=builder /app/conf ./conf
EXPOSE 8890
ENTRYPOINT ["./git-sync-service", "-env", "prod"]
```

### 10.2 Makefile

```makefile
.PHONY: build run clean test gen

APP_NAME := git-sync-service
BUILD_DIR := ./output

build:
	@go build -o $(BUILD_DIR)/$(APP_NAME) .

run:
	@go run main.go -env dev

gen:
	@hz update -idl idl/sync_task.proto
	@hz update -idl idl/repo.proto
	@hz update -idl idl/webhook.proto

test:
	@go test ./...

clean:
	@rm -rf $(BUILD_DIR)

docker-build:
	@docker build -t $(APP_NAME):latest .

docker-run:
	@docker run -p 8890:8890 $(APP_NAME):latest
```

## 11. 实施步骤

| 阶段 | 任务 | 预估工时 |
|------|------|---------|
| 1 | 项目初始化 + go.mod | 1h |
| 2 | sync/ 核心库目录结构 | 1h |
| 3 | 数据模型 (model/) | 2h |
| 4 | DAO 层 | 2h |
| 5 | ProviderManager | 1h |
| 6 | RepoService | 2h |
| 7 | SyncService + Executor | 4h |
| 8 | WebhookService + RuleEngine | 3h |
| 9 | CronService | 1h |
| 10 | IDL 定义 + hz 生成 | 2h |
| 11 | HTTP Handlers | 2h |
| 12 | 测试 + 调试 | 3h |
| **总计** | | **24h** |

---

**文档版本**: 2.0
**创建日期**: 2026-05-15
**更新日期**: 2026-05-16
**维护者**: git-sync-service team
