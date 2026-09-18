# 项目改进设计方案

## 概述

本文档描述了 git-sync-service 项目的全面改进方案，包括安全、锁、测试、文档等方面的改进。

## 1. 改进目标

- 修复所有严重和高优先级问题
- 提升代码质量和测试覆盖率
- 改进安全性和性能
- 更新文档和配置

## 2. 问题分类

### 2.1 严重问题 (CRITICAL)

1. **API 无认证保护** - 所有 API 端点都没有认证中间件
2. **Lock context 值丢失** - `Unlock` 无法找到锁值

### 2.2 高优先级问题 (HIGH)

3. **Goroutine context 取消问题** - `ReceiveWebhook` 中的 goroutine 使用会被取消的 context
4. **Semaphore 非原子操作** - `Semaphore.Acquire` 存在竞态条件
5. **字符串比较错误** - 使用 `err.Error() == "repo not found"` 而不是类型化错误
6. **NewRepoDAO 使用 panic** - 构造函数中使用 panic 而不是返回错误
7. **测试覆盖不足** - Executor、Task、Webhook、Cron 等核心功能无测试

### 2.3 中优先级问题 (MEDIUM)

8. **Webhook 速率限制未生效** - 配置存在但未使用
9. **日志配置未使用** - LogConfig 定义但未应用
10. **临时目录未清理** - 同步执行后临时目录未删除
11. **CI 覆盖率条件错误** - 条件检查 `'1.23'` 但实际是 `'1.26'`
12. **Thrift 强制降级** - 使用 2019 年的 v0.13.0 版本

### 2.4 低优先级问题 (LOW)

13. **缺少 LICENSE 文件** - README 引用但文件不存在
14. **缺少 Dockerfile** - ARCHITECTURE.md 文档化但未实现
15. **README 端口错误** - 文档说 8080，实际是 8890
16. **Service 是 God Object** - 需要拆分为多个专注的服务

## 3. 解决方案

### 3.1 严重问题解决方案

#### 3.1.1 API 认证中间件

**问题**: 所有 API 端点都没有认证中间件

**解决方案**:
- 在 `biz/router/git_sync/middleware.go` 中实现 API Key 认证
- 使用 `Service.GetAPIKey()` 获取配置的 API Key
- 在请求头中检查 `X-API-Key` 或 `Authorization` 头
- 返回 401 Unauthorized 如果认证失败

**实现步骤**:
1. 修改 `middleware.go` 中的所有中间件函数
2. 添加认证逻辑
3. 测试认证功能

#### 3.1.2 Lock context 值丢失

**问题**: `Unlock` 无法找到锁值

**解决方案**:
- 修复 `internal/lock/lock.go` 中的 context 值传递
- 确保 `LockWithTTL` 返回包含锁值的 context
- 修改 `Unlock` 方法使用正确的 context

**实现步骤**:
1. 分析 `lock.go` 中的 context 值传递逻辑
2. 修复 context 值丢失问题
3. 测试锁功能

### 3.2 高优先级问题解决方案

#### 3.2.1 Goroutine context 取消问题

**问题**: `ReceiveWebhook` 中的 goroutine 使用会被取消的 context

**解决方案**:
- 在 `internal/service/webhook.go` 中使用 `context.Background()`
- 为 goroutine 创建新的 context
- 确保 goroutine 不会因为 HTTP 请求结束而被取消

**实现步骤**:
1. 修改 `ReceiveWebhook` 函数
2. 使用 `context.Background()` 替换请求 context
3. 测试 webhook 处理

#### 3.2.2 Semaphore 非原子操作

**问题**: `Semaphore.Acquire` 存在竞态条件

**解决方案**:
- 使用 Lua 脚本实现原子操作
- 在 `internal/lock/lock.go` 中添加 Lua 脚本
- 确保 `ZADD` 和 `ZRank` 操作原子执行

**实现步骤**:
1. 编写 Lua 脚本
2. 修改 `Semaphore.Acquire` 方法
3. 测试并发场景

#### 3.2.3 字符串比较错误

**问题**: 使用 `err.Error() == "repo not found"` 而不是类型化错误

**解决方案**:
- 在 `internal/service/repo.go` 中定义哨兵错误
- 使用 `errors.Is()` 替换字符串比较
- 更新所有错误处理逻辑

**实现步骤**:
1. 定义 `ErrRepoNotFound` 等哨兵错误
2. 修改错误处理逻辑
3. 测试错误处理

#### 3.2.4 NewRepoDAO 使用 panic

**问题**: 构造函数中使用 panic 而不是返回错误

**解决方案**:
- 修改 `internal/dao/repo_dao.go` 中的 `NewRepoDAO` 函数
- 返回 `(*RepoDAO, error)` 而不是 `*RepoDAO`
- 更新所有调用点

**实现步骤**:
1. 修改 `NewRepoDAO` 函数签名
2. 更新所有调用点
3. 测试 DAO 功能

#### 3.2.5 测试覆盖不足

**问题**: Executor、Task、Webhook、Cron 等核心功能无测试

**解决方案**:
- 为以下模块添加单元测试：
  - `internal/executor/executor.go`
  - `internal/service/task.go`
  - `internal/service/webhook.go`
  - `internal/service/cron.go`
  - `internal/lock/lock.go` (RedisLock 和 Semaphore)

**实现步骤**:
1. 创建测试文件
2. 编写测试用例
3. 运行测试验证

### 3.3 中优先级问题解决方案

#### 3.3.1 Webhook 速率限制未生效

**问题**: 配置存在但未使用

**解决方案**:
- 在 `biz/handler/git_sync/webhook_receive.go` 中实现速率限制
- 使用 `cfg.Webhook.RateLimit` 配置
- 返回 429 Too Many Requests 如果超过限制

**实现步骤**:
1. 实现速率限制逻辑
2. 使用 Redis 或内存存储计数器
3. 测试速率限制

#### 3.3.2 日志配置未使用

**问题**: LogConfig 定义但未应用

**解决方案**:
- 在 `main.go` 中配置 slog
- 使用 `cfg.Log.Level` 和 `cfg.Log.Format`
- 应用日志配置

**实现步骤**:
1. 修改 `main.go` 初始化日志
2. 应用日志配置
3. 测试日志输出

#### 3.3.3 临时目录未清理

**问题**: 同步执行后临时目录未删除

**解决方案**:
- 在 `internal/executor/executor.go` 中添加清理逻辑
- 在 `Execute` 函数完成后删除临时目录
- 使用 `defer` 确保清理

**实现步骤**:
1. 修改 `Execute` 函数
2. 添加清理逻辑
3. 测试目录清理

#### 3.3.4 CI 覆盖率条件错误

**问题**: 条件检查 `'1.23'` 但实际是 `'1.26'`

**解决方案**:
- 修改 `.github/workflows/ci.yml` 中的条件
- 将 `'1.23'` 改为 `'1.26'`

**实现步骤**:
1. 修改 CI 配置文件
2. 测试 CI 流程

#### 3.3.5 Thrift 强制降级

**问题**: 使用 2019 年的 v0.13.0 版本

**解决方案**:
- 重新生成 Thrift 代码
- 使用兼容的 thriftgo 版本
- 移除 `replace` 指令

**实现步骤**:
1. 安装兼容的 thriftgo
2. 重新生成代码
3. 移除 `replace` 指令
4. 测试编译

### 3.4 低优先级问题解决方案

#### 3.4.1 缺少 LICENSE 文件

**问题**: README 引用但文件不存在

**解决方案**:
- 创建 `LICENSE` 文件
- 使用 MIT 许可证（与 README 一致）

**实现步骤**:
1. 创建 `LICENSE` 文件
2. 添加 MIT 许可证内容

#### 3.4.2 缺少 Dockerfile

**问题**: ARCHITECTURE.md 文档化但未实现

**解决方案**:
- 创建 `Dockerfile`
- 使用多阶段构建
- 包含所有依赖

**实现步骤**:
1. 创建 `Dockerfile`
2. 编写构建步骤
3. 测试 Docker 构建

#### 3.4.3 README 端口错误

**问题**: 文档说 8080，实际是 8890

**解决方案**:
- 修改 `README.md` 中的端口引用
- 将 8080 改为 8890

**实现步骤**:
1. 修改 `README.md`
2. 验证端口配置

#### 3.4.4 Service 是 God Object

**问题**: 需要拆分为多个专注的服务

**解决方案**:
- 拆分 `internal/service/service.go` 中的 `Service` 结构体
- 创建 `RepoService`、`TaskService`、`WebhookService` 等
- 重新组织代码结构

**实现步骤**:
1. 分析依赖关系
2. 创建新的服务结构
3. 重构代码
4. 测试功能

## 4. 实现计划

### 4.1 第1批：严重问题 + 高优先级问题

**时间**: 2-3 天

**任务**:
1. 实现 API 认证中间件
2. 修复 Lock context 值丢失
3. 修复 Goroutine context 取消问题
4. 实现 Semaphore 原子操作
5. 修复字符串比较错误
6. 修改 NewRepoDAO 返回错误
7. 添加核心功能测试

### 4.2 第2批：中优先级问题

**时间**: 1-2 天

**任务**:
1. 实现 Webhook 速率限制
2. 应用日志配置
3. 实现临时目录清理
4. 修复 CI 覆盖率条件
5. 处理 Thrift 强制降级

### 4.3 第3批：低优先级问题

**时间**: 1 天

**任务**:
1. 创建 LICENSE 文件
2. 创建 Dockerfile
3. 修复 README 端口错误
4. 拆分 Service God Object

## 5. 风险评估

### 5.1 技术风险

- **Lock 修复风险**: 修改锁逻辑可能影响并发安全
- **Semaphore 原子操作风险**: Lua 脚本可能引入新的边界条件
- **Service 拆分风险**: 大规模重构可能引入回归问题

### 5.2 缓解措施

- 充分测试每个修改
- 使用 feature flag 控制新功能
- 逐步 rollout 变更

## 6. 成功标准

### 6.1 功能标准

- 所有 API 端点都有认证保护
- Lock 和 Semaphore 功能正常
- 所有核心功能有测试覆盖
- 文档准确完整

### 6.2 质量标准

- 代码通过所有 lint 检查
- 测试覆盖率达到 80%+
- 无严重或高优先级问题

### 6.3 性能标准

- API 响应时间无显著增加
- 内存使用无显著增加
- 并发性能无显著下降

## 7. 监控和告警

### 7.1 关键指标

- API 认证成功率
- Lock/Semaphore 操作成功率
- 测试覆盖率
- 错误率

### 7.2 告警规则

- 认证失败率超过 5%
- Lock 操作失败率超过 1%
- 测试覆盖率低于 70%
- 错误率显著上升

## 8. 总结

本设计方案详细描述了 git-sync-service 项目的全面改进方案。通过按优先级分批处理，可以安全、高效地完成所有改进，同时保持系统的稳定性和可靠性。