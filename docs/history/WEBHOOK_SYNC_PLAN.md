# Webhook 同步触发 — 完整规划文档

## 一、现状分析

| 组件 | 状态 | 问题 |
|------|------|------|
| `triggerSync()` | ❌ 空函数 | 只打日志，不执行同步 |
| 事件→分支映射 | ❌ 缺失 | 无法按推送分支触发对应同步任务 |
| 规则关联多任务 | ❌ 缺失 | 一条规则只能关联一个任务 |
| 自动创建规则 | ❌ 缺失 | 绑定仓库时不会自动创建 sync 规则 |
| Webhook 重试 | ❌ 缺失 | 重试不会重新触发规则引擎 |

### 关键代码位置

| 文件 | 作用 |
|------|------|
| `biz/handler/webhook/webhook_service.go` | HTTP 入口：Receive、TriggerSync、TriggerSyncByToken |
| `biz/service/webhookevent/webhook_event_service.go` | 核心处理：ProcessIncomingEvent、applyRules、triggerSync（空函数） |
| `biz/service/sync/sync_service.go` | 同步逻辑：ExecuteSyncWithTrigger、认证、fetch/push |
| `biz/service/sync/sync_executor.go` | Git 操作：doSyncSingleBranch、doSyncAllBranches |
| `biz/service/sync/cron_service.go` | 定时任务调度 |
| `biz/service/binding/binding_webhook.go` | 绑定仓库时自动注册 Webhook |
| `biz/model/po/webhook_event.go` | 数据模型：WebhookEvent、WebhookRule |
| `biz/model/po/sync_task.go` | 数据模型：SyncTask |
| `biz/model/po/sync_run.go` | 数据模型：SyncRun、TriggerSource 常量 |

---

## 二、整体流程设计

```
Git 平台推送事件 (GitHub/GitLab/Gitea/Forgejo)
       │
       ▼
POST /api/webhooks/receive
       │
       ▼
验证签名 → 解析事件 → 匹配仓库
       │
       ▼
ProcessIncomingEvent()
       │
       ├── 去重检查 (EventID)
       ├── 匹配仓库 (PlatformOwner/PlatformRepo)
       ├── 持久化 WebhookEvent
       │
       ▼
applyRules() [异步]
       │
       ├── 查询所有 enabled 的 WebhookRule
       │
       ├── 匹配规则:
       │   ├── 事件类型: rule.EventTypePattern ↔ event.Type (支持 glob)
       │   ├── 仓库模式: rule.RepoPattern ↔ event.Repo.FullName (支持通配符)
       │   ├── 分支模式: rule.BranchPattern ↔ event.Branch (新增)
       │   └── 防抖检查: rule.MinInterval (新增)
       │
       ├── Action = "sync"
       │         │
       │         ▼
       │   triggerSync(rule, event) ← 需要实现
       │         │
       │         ├── 解析关联的同步任务列表
       │         ├── 根据事件分支匹配任务源分支
       │         ├── 支持全分支/单分支模式
       │         ├── 执行同步 (go ExecuteSyncWithTrigger)
       │         └── 记录运行日志
       │
       ├── Action = "code_review"
       │         └── triggerCodeReview(event)
       │
       └── Action = "notify"
                 └── 发送通知
```

---

## 三、核心功能模块

### 模块 1：WebhookRule 模型增强

**新增字段：**

```go
type WebhookRule struct {
    // ... 现有字段保留
    Name              string `gorm:"type:varchar(100);uniqueIndex"`
    ProviderConfigID  uint
    EventTypePattern  string `gorm:"type:varchar(100)"`  // 如 "push", "cr.*"
    RepoPattern       string `gorm:"type:varchar(255)"`  // 如 "user/repo", "*"
    Action            string `gorm:"type:varchar(20)"`   // "sync", "notify", "code_review"
    ActionConfig      map[string]interface{} `gorm:"type:json"`
    Enabled           bool   `gorm:"default:true"`

    // ===== 新增字段 =====
    SyncTaskKeys      string `gorm:"type:text"`          // 关联多个任务 key，逗号分隔
    BranchPattern     string `gorm:"type:varchar(255)"`  // 分支匹配模式，如 "feature/*", "release/*"
    SyncMode          string `gorm:"type:varchar(20)"`   // "trigger" (按事件触发) 或 "always" (总是触发)
    MinInterval       int    `gorm:"default:60"`         // 最小触发间隔(秒)，防抖
    Description       string `gorm:"type:text"`          // 规则描述
}
```

**数据库迁移：**

```sql
ALTER TABLE webhook_rules
    ADD COLUMN sync_task_keys TEXT AFTER action_config,
    ADD COLUMN branch_pattern VARCHAR(255) AFTER sync_task_keys,
    ADD COLUMN sync_mode VARCHAR(20) DEFAULT 'trigger' AFTER branch_pattern,
    ADD COLUMN min_interval INT DEFAULT 60 AFTER sync_mode,
    ADD COLUMN description TEXT AFTER min_interval;
```

### 模块 2：triggerSync 实现

```go
func triggerSync(rule WebhookRule, event NormalizedEvent) {
    // 1. 解析关联的任务列表
    taskKeys := strings.Split(rule.SyncTaskKeys, ",")
    
    for _, taskKey := range taskKeys {
        taskKey = strings.TrimSpace(taskKey)
        if taskKey == "" {
            continue
        }
        
        // 2. 查找任务
        task, err := db.NewSyncTaskDAO().FindByKey(taskKey)
        if err != nil {
            log.Printf("Webhook sync: task %s not found: %v", taskKey, err)
            continue
        }
        
        if !task.Enabled {
            log.Printf("Webhook sync: task %s is disabled", taskKey)
            continue
        }
        
        // 3. 分支匹配检查
        if !matchTaskBranch(task, event) {
            log.Printf("Webhook sync: task %s branch mismatch", taskKey)
            continue
        }
        
        // 4. 防抖检查
        if rule.MinInterval > 0 {
            if !checkMinInterval(rule.ID, taskKey, rule.MinInterval) {
                log.Printf("Webhook sync: task %s skipped (min interval)", taskKey)
                continue
            }
        }
        
        // 5. 执行同步
        log.Printf("Webhook sync: triggering task %s for event %s", taskKey, event.ID)
        go func() {
            svc := NewSyncService()
            if err := svc.ExecuteSyncWithTrigger(task, po.TriggerSourceWebhook); err != nil {
                log.Printf("Webhook sync: task %s failed: %v", taskKey, err)
            }
        }()
    }
}
```

### 模块 3：分支匹配逻辑

```go
func matchTaskBranch(task SyncTask, event NormalizedEvent) bool {
    eventBranch := extractBranchFromEvent(event)
    if eventBranch == "" {
        return false
    }
    
    switch task.SyncMode {
    case "single":
        // 单分支模式：事件分支必须等于任务源分支
        return task.SourceBranch == eventBranch
    
    case "all-branch":
        // 全分支模式：任何推送都触发
        return true
    
    default:
        return task.SourceBranch == eventBranch
    }
}

func extractBranchFromEvent(event NormalizedEvent) string {
    // 从事件中提取分支名
    // GitHub: refs/heads/feature/foo → feature/foo
    // GitLab: refs/heads/feature/foo → feature/foo
    branch := event.Branch
    if strings.HasPrefix(branch, "refs/heads/") {
        branch = strings.TrimPrefix(branch, "refs/heads/")
    }
    return branch
}

func matchBranchPattern(pattern, branch string) bool {
    if pattern == "" || pattern == "*" {
        return true
    }
    
    // 支持逗号分隔的多模式
    patterns := strings.Split(pattern, ",")
    for _, p := range patterns {
        p = strings.TrimSpace(p)
        if matched, _ := filepath.Match(p, branch); matched {
            return true
        }
    }
    return false
}
```

### 模块 4：防抖机制

```go
var syncDebounceMap = sync.Map{}

type debounceEntry struct {
    LastRun   time.Time
    Count     int
}

func checkMinInterval(ruleID uint, taskKey string, minInterval int) bool {
    key := fmt.Sprintf("webhook_sync:%d:%s", ruleID, taskKey)
    
    if val, ok := syncDebounceMap.Load(key); ok {
        entry := val.(debounceEntry)
        if time.Since(entry.LastRun) < time.Duration(minInterval)*time.Second {
            return false // 间隔太短，跳过
        }
    }
    
    syncDebounceMap.Store(key, debounceEntry{
        LastRun: time.Now(),
    })
    return true
}
```

### 模块 5：自动创建 Sync 规则

当绑定仓库并创建同步任务时，自动创建 Webhook 规则：

```go
// binding_webhook.go - 在 CreateBinding 中调用
func autoCreateSyncRule(providerConfigID uint, repo Repo, syncTask SyncTask) {
    rule := WebhookRule{
        Name:             fmt.Sprintf("auto-sync-%s", syncTask.Key),
        ProviderConfigID: providerConfigID,
        EventTypePattern: "push", // 推送事件触发
        RepoPattern:      repo.FullName,
        Action:           "sync",
        SyncTaskKeys:     syncTask.Key,
        BranchPattern:    syncTask.SourceBranch,
        SyncMode:         syncTask.SyncMode,
        MinInterval:      60,
        Enabled:          true,
        Description:      fmt.Sprintf("自动同步规则: %s", syncTask.Key),
    }
    
    db.NewWebhookRuleDAO().Create(&rule)
}
```

---

## 四、规则匹配算法

```
输入: WebhookRule + NormalizedEvent

匹配流程:
┌─────────────────────────────────────────────┐
│ 1. 事件类型匹配                              │
│    rule.EventTypePattern ↔ event.Type       │
│    支持 glob: "push", "cr.*", "*"           │
│    不匹配 → 跳过                            │
└─────────────────────────────────────────────┘
              │ 匹配
              ▼
┌─────────────────────────────────────────────┐
│ 2. 仓库匹配                                  │
│    rule.RepoPattern ↔ event.Repo.FullName   │
│    支持通配符: "user/repo", "*/backend"      │
│    不匹配 → 跳过                            │
└─────────────────────────────────────────────┘
              │ 匹配
              ▼
┌─────────────────────────────────────────────┐
│ 3. 分支匹配 (新增)                           │
│    rule.BranchPattern ↔ event.Branch        │
│    支持通配符: "feature/*", "release/*"      │
│    留空 → 匹配所有分支                       │
│    不匹配 → 跳过                            │
└─────────────────────────────────────────────┘
              │ 匹配
              ▼
┌─────────────────────────────────────────────┐
│ 4. 防抖检查 (新增)                           │
│    rule.MinInterval vs 距上次触发时间        │
│    间隔不足 → 跳过                          │
└─────────────────────────────────────────────┘
              │ 通过
              ▼
┌─────────────────────────────────────────────┐
│ 5. 执行动作                                  │
│    action = "sync" → triggerSync()          │
│    action = "code_review" → triggerReview() │
│    action = "notify" → sendNotification()   │
└─────────────────────────────────────────────┘
```

---

## 五、数据流时序图

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  GitHub   │────▶│ Webhook  │────▶│  Event   │────▶│  Rule    │
│  Push     │     │ Receiver │     │ Processor│     │  Engine  │
└──────────┘     └──────────┘     └──────────┘     └──────────┘
                      │                │                │
                      ▼                ▼                ▼
                 ┌──────────┐    ┌──────────┐    ┌──────────┐
                 │  签名    │    │  去重    │    │  匹配    │
                 │  验证    │    │  检查    │    │  规则    │
                 └──────────┘    └──────────┘    └──────────┘
                                      │                │
                                      ▼                ▼
                                 ┌──────────┐    ┌──────────┐
                                 │  持久化  │    │  分支    │
                                 │  事件    │    │  匹配    │
                                 └──────────┘    └──────────┘
                                                       │
                                                       ▼
                                                 ┌──────────┐
                                                 │ trigger  │
                                                 │ Sync()   │
                                                 └──────────┘
                                                       │
                    ┌──────────────────────────────────┤
                    │                                  │
                    ▼                                  ▼
              ┌──────────┐                      ┌──────────┐
              │  任务 A  │                      │  任务 B  │
              │ (单分支) │                      │ (全分支) │
              └──────────┘                      └──────────┘
                    │                                  │
                    ▼                                  ▼
              ┌──────────┐                      ┌──────────┐
              │  分支    │                      │  直接    │
              │  匹配    │                      │  执行    │
              └──────────┘                      └──────────┘
                    │                                  │
                    ▼                                  ▼
              ┌──────────┐                      ┌──────────┐
              │ Execute  │                      │ Execute  │
              │ Sync     │                      │ Sync     │
              └──────────┘                      └──────────┘
                    │                                  │
                    ▼                                  ▼
              ┌──────────┐                      ┌──────────┐
              │ git fetch│                      │ git fetch│
              │ git push │                      │ git push │
              └──────────┘                      └──────────┘
                    │                                  │
                    ▼                                  ▼
              ┌──────────┐                      ┌──────────┐
              │  记录    │                      │  记录    │
              │  SyncRun │                      │  SyncRun │
              └──────────┘                      └──────────┘
```

---

## 六、API 设计

### 6.1 Webhook 规则 CRUD

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | /api/v1/webhook-rules | 获取规则列表 |
| GET | /api/v1/webhook-rule | 获取单个规则详情 |
| POST | /api/v1/webhook-rule/create | 创建规则 |
| POST | /api/v1/webhook-rule/update | 更新规则 |
| POST | /api/v1/webhook-rule/delete | 删除规则 |
| POST | /api/v1/webhook-rule/test | 测试规则触发 |

### 6.2 Thrift IDL 定义

```thrift
namespace go webhook_rule

service WebhookRuleService {
    ListRulesResponse ListRules(1: ListRulesRequest req)
    RuleResponse GetRule(1: GetRuleRequest req)
    RuleResponse CreateRule(1: CreateRuleRequest req)
    RuleResponse UpdateRule(1: UpdateRuleRequest req)
    EmptyResponse DeleteRule(1: DeleteRuleRequest req)
    TestResult TestRule(1: TestRuleRequest req)
}

struct WebhookRule {
    1: i64 id
    2: string name
    3: i64 provider_config_id
    4: string event_type_pattern
    5: string repo_pattern
    6: string action
    7: map<string, string> action_config
    8: bool enabled
    9: string sync_task_keys
    10: string branch_pattern
    11: string sync_mode
    12: i32 min_interval
    13: string description
    14: string created_at
    15: string updated_at
}

struct CreateRuleRequest {
    1: string name
    2: i64 provider_config_id
    3: string event_type_pattern
    4: string repo_pattern
    5: string action
    6: map<string, string> action_config
    7: bool enabled
    8: string sync_task_keys
    9: string branch_pattern
    10: string sync_mode
    11: i32 min_interval
    12: string description
}
```

---

## 七、UI 页面设计

### 页面 1：Webhook 规则列表

```
┌─────────────────────────────────────────────────────────────────┐
│  Webhook 规则管理                                    [+ 新建规则] │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  🔍 搜索规则...     状态: [全部 ▼]  动作: [全部 ▼]             │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 规则名称        事件类型  仓库模式   动作     状态   操作   │ │
│  ├────────────────────────────────────────────────────────────┤ │
│  │ auto-sync-main  push     */backend  同步    ✅ 启用  ⚙️   │ │
│  │ auto-review-cr  cr.*     *          代码审查 ✅ 启用  ⚙️   │ │
│  │ notify-deploy   push     */deploy   通知    ⏸ 停用  ⚙️   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  共 3 条规则                                                    │
└─────────────────────────────────────────────────────────────────┘
```

### 页面 2：创建/编辑 Webhook 规则

```
┌─────────────────────────────────────────────────────────────────┐
│  创建 Webhook 规则                                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  基本信息                                                       │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │ 规则名称: [auto-sync-main                               ] │ │
│  │ 描述:     [当 backend 仓库有推送时自动同步              ] │ │
│  │ 关联平台: [GitHub - user/backend           ▼]            │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  触发条件                                                       │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │ 事件类型: [push           ▼]                              │ │
│  │           ○ push  ○ cr.open  ○ tag_push  ○ 自定义        │ │
│  │                                                           │ │
│  │ 仓库模式: [*/backend      ]  (支持通配符 * 和 ?)          │ │
│  │                                                           │ │
│  │ 分支模式: [feature/*, main]  (逗号分隔，留空=任意分支)     │ │
│  │                                                           │ │
│  │ 最小间隔: [60] 秒  (防抖，防止短时间内重复触发)           │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  执行动作                                                       │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │ 动作类型: [同步任务        ▼]                             │ │
│  │           ○ 同步任务  ○ 代码审查  ○ 通知                 │ │
│  │                                                           │ │
│  │ 关联同步任务:                                              │ │
│  │ ☑ feature → main      (单分支)  GitHub/frontend-app      │ │
│  │ ☑ develop → staging   (全分支)  GitLab/backend-api       │ │
│  │ ☐ hotfix → prod       (单分支)  GitHub/frontend-app      │ │
│  │                                                           │ │
│  │ 同步触发模式:                                              │ │
│  │ ● 按事件分支触发 (仅匹配的分支变化时触发)                │ │
│  │ ○ 始终触发 (任何推送都触发关联的所有任务)                │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│                                    [取消]  [保存规则]           │
└─────────────────────────────────────────────────────────────────┘
```

### 页面 3：同步历史 — 增强筛选

```
┌─────────────────────────────────────────────────────────────────┐
│  同步历史                                          [导出] [清空] │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  总执行次数 1,234 │ 成功 1,180 │ 失败 42 │ 平均耗时 45s        │
│                                                                 │
│  🔍 搜索...  来源: [全部 ▼]  状态: [全部 ▼]  日期: [范围]     │
│                       ↑                                         │
│              新增 Webhook 来源筛选                              │
│              手动 / 定时 / Webhook                             │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ ✅ feature → main 同步成功                    2 分钟前     │ │
│  │ 来源: Webhook (push event)                                │ │
│  │ 触发规则: auto-sync-main                                  │ │
│  │ 分支: feature/new-ui → main                               │ │
│  │ 5 个提交   14:32:15 - 14:32:27                           │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ ❌ all-branch 同步失败                       1 小时前      │ │
│  │ 来源: 定时 (0 2 * * *)                                   │ │
│  │ 分支: origin/* → backup/*                                 │ │
│  │ 错误详情: 推送被拒绝...                                   │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 页面 4：Webhook 事件日志

```
┌─────────────────────────────────────────────────────────────────┐
│  Webhook 事件日志                                               │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  🔍 搜索事件...  来源: [全部 ▼]  状态: [全部 ▼]               │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 事件 ID       来源     仓库            类型    状态  操作  │ │
│  ├────────────────────────────────────────────────────────────┤ │
│  │ evt-abc123    GitHub   user/backend    push    ✅    ⚙️   │ │
│  │ evt-def456    GitLab   team/api        cr.open ✅    ⚙️   │ │
│  │ evt-ghi789    Gitea    org/docs        push    ❌    ⚙️   │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  点击事件展开详情:                                              │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ 事件: evt-abc123                                          │ │
│  │ 来源: GitHub                                              │ │
│  │ 仓库: user/backend                                        │ │
│  │ 类型: push                                                │ │
│  │ 分支: feature/new-ui                                      │ │
│  │ 提交: a1b2c3d feat: add new feature                      │ │
│  │ 触发规则: auto-sync-main                                  │ │
│  │ 触发任务: feature → main (成功)                           │ │
│  │ 时间: 2024-01-15 14:32:15                                │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## 八、实施步骤

| 阶段 | 任务 | 文件 | 预估工时 |
|------|------|------|---------|
| **1** | 实现 `triggerSync()` 核心逻辑 | `webhook_event_service.go` | 2h |
| **2** | 添加分支匹配逻辑 | `webhook_event_service.go` | 1h |
| **3** | 添加防抖机制 | `webhook_event_service.go` | 1h |
| **4** | WebhookRule 模型增加字段 | `webhook_event.go` + DB migration | 1h |
| **5** | Webhook 规则 CRUD API | `webhook_service.go` + IDL | 2h |
| **6** | 绑定仓库时自动创建 sync 规则 | `binding_webhook.go` | 1h |
| **7** | 同步历史增加 Webhook 来源筛选 | `sync_service.go` | 1h |
| **8** | 设计 UI 页面 | `design.pen` | 3h |
| **9** | 单元测试 | `*_test.go` | 2h |
| **10** | 集成测试 | - | 2h |
| **总计** | | | **16h** |

---

## 九、配置示例

### 9.1 单分支同步规则

```json
{
  "name": "auto-sync-feature-to-main",
  "provider_config_id": 1,
  "event_type_pattern": "push",
  "repo_pattern": "user/frontend-app",
  "action": "sync",
  "sync_task_keys": "task-feature-main",
  "branch_pattern": "feature/*",
  "sync_mode": "trigger",
  "min_interval": 60,
  "enabled": true
}
```

### 9.2 全分支同步规则

```json
{
  "name": "mirror-all-branches",
  "provider_config_id": 2,
  "event_type_pattern": "push",
  "repo_pattern": "*/backend-*",
  "action": "sync",
  "sync_task_keys": "task-mirror-1,task-mirror-2",
  "branch_pattern": "",
  "sync_mode": "always",
  "min_interval": 300,
  "enabled": true
}
```

### 9.3 多动作规则

```json
{
  "name": "push-handler",
  "provider_config_id": 1,
  "event_type_pattern": "push",
  "repo_pattern": "user/critical-repo",
  "action": "sync",
  "sync_task_keys": "task-sync-staging,task-sync-prod",
  "branch_pattern": "main, release/*",
  "sync_mode": "trigger",
  "min_interval": 120,
  "enabled": true
}
```

---

## 十、注意事项

### 10.1 安全性
- Webhook 签名验证必须启用
- 规则创建需要管理员权限
- 防抖间隔建议不低于 30 秒

### 10.2 性能
- 防抖使用内存缓存，重启后重置
- 大量规则时建议异步处理
- 分支匹配使用 `filepath.Match` 而非正则

### 10.3 可观测性
- 每次触发记录日志
- 同步历史标记触发来源
- Webhook 事件日志保留 30 天

### 10.4 兼容性
- 保留原有 `ActionConfig` 字段兼容旧规则
- 新增字段均可为空，向后兼容
- 迁移脚本自动生成默认值

---

**文档版本**: 1.0  
**创建日期**: 2026-05-15  
**维护者**: git-ferry team
