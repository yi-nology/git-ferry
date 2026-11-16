# GitFerry 可靠性演进路线（2026-11）

> 起点：2026-11-17 评审（主服务 v3.2.0 / core v0.50.0 / SDK v0.143.0）。
> 上一轮（2026-11-12）的七项问题已基本消化：持久化队列、Outbox、
> `schema_migrations`、Clone Cache、对账/恢复点/策略快照均已落地。
>
> **本轮判断**：不再以「引入分布式调度框架」为目标。队列与租约的存在
> 不等于故障场景已被验证。接下来 6–8 周只做一件事——
> **把「触发 → 执行 → 投递 → 调度」四段语义钉死，并给出可重复的故障验收**。

## 0. 原则

1. **不加框架**：不引入 Kafka / Temporal / 微服务拆分。Worker 与 API 是
   同一二进制的不同运行模式。
2. **状态机优先于功能**：任何「触发丢失 / 双属主 / 事件蒸发」都必须由
   数据库约束或 CAS 证明，不能靠日志和取消 context 兜底。
3. **验收先于实现**：每个阶段带确定性并发测试；没有可复现的故障注入，
   不算完成。
4. **fail-closed 只对关键写入**：发布、推送、迁移互斥宁可失败重试，
   不可「降级为进程内互斥」。

## 1. 现状与 issue 映射

9 项发现已提交至 [git-ferry-core](https://github.com/yi-nology/git-ferry-core/issues)，
均带源码行号与（可复现项的）确定性 probe：

| Issue | 优先级 | 主题 | 归属阶段 |
|---|---|---|---|
| [#36](https://github.com/yi-nology/git-ferry-core/issues/36) | P0 | 执行中到达的触发被合并丢弃（无代次承接） | R1 |
| [#37](https://github.com/yi-nology/git-ferry-core/issues/37) | P0 | Lease 缺 fencing：毒丸终态可被覆写 / 续租被 reaper 无视 | R1 |
| [#38](https://github.com/yi-nology/git-ferry-core/issues/38) | P0 | 直跑路径无条件闭环 outbox（#15 的遗漏入口） | R1 |
| [#43](https://github.com/yi-nology/git-ferry-core/issues/43) | P1 | MySQL `GET_LOCK`/`RELEASE_LOCK` 跨连接池连接 | R1 |
| [#39](https://github.com/yi-nology/git-ferry-core/issues/39) | P1 | 重试冷却期无延迟调度闭环 | R2 |
| [#40](https://github.com/yi-nology/git-ferry-core/issues/40) | P1 | 每实例本地 cron 无统一配置视图 | R2 |
| [#42](https://github.com/yi-nology/git-ferry-core/issues/42) | P1 | 缺少部署模式校验（扩副本不拒绝启动） | R2 |
| [#41](https://github.com/yi-nology/git-ferry-core/issues/41) | P1 | 元数据迁移以标题为身份键 | R4 |
| [#44](https://github.com/yi-nology/git-ferry-core/issues/44) | P1 | 对账状态语义（错误 vs 不存在、SHA 单一判据） | R4 |

## 2. 阶段计划

### R0 — 版本收口（已完成，2026-11-17）

- 壳 **v3.2.0** 已发布并推送 tag：包含 core v0.50.0 + SDK v0.143.0
  四层布局迁移；v3.1.0 与 core v0.49.0 的版本差距已收口。
- intranet `71a71e3` 已同步 core v0.50.0 / SDK v0.143.0（随下一次 tag 发布）。
- 遗留动作：跑一次 GHCR 镜像流水线，核对 v3.2.0 的五个 tag 对齐
  （`:v3.2.0` / `:3.2.0` / `:3.2` / `:latest` / `sha-<commit>`）。

### R1 — 正确性收敛（第 1–2 周）

**目标：消除已识别的触发丢失与执行权竞态。**

1. **代次合并（#36）**
   - 新增 `execution_task_states(task_key PK, desired_generation, completed_generation, active_job_id)`。
   - 触发 = 事务内 `desired_generation += 1`；`active_job_id` 为空才插作业。
   - 收尾 CAS 推进 `completed_generation`；未追平则必须再入一个作业。
   - 活动槽由唯一约束表达，`INSERT ... WHERE NOT EXISTS` 降级为快路径。

2. **Fencing（#37）**
   - `execution_jobs` 增 `lease_version`（Claim/Renew +1）。
   - `Complete/RenewLease/Reschedule` 条件补 `status=claimed AND lease_until>now AND lease_version=?`。
   - `RequeueExpired` 改单语句条件更新，去掉「先查后改」窗口；毒丸分支清 owner。
   - 推送侧补远端 ref 条件保护（不能只靠取消 context）。

3. **Outbox 入口一致（#38）**
   - `RunTaskWithTrigger` 与 `finishRun` 共用同一个收尾函数；
   - 只有 `delivered` 才闭环；补直跑路径的回归测试。

4. **MySQL 迁移锁（#43）**
   - `sqlDB.Conn(ctx)` 固定连接持有 `GET_LOCK`/`RELEASE_LOCK`；
   - 迁移语句可重复执行，`schema_migrations` 在成功后记录；
   - 补 MySQL 双实例并发启动测试。

**R1 验收**

- 执行中连发 N 次 webhook → 最终恰好执行 N+1 次，最后一次提交必被同步。
- renew 与 reaper 交错（`-race`）→ 无双属主；毒丸终态不可被旧属主覆写。
- 无订阅者直跑 → 事件保留在 outbox，订阅者注册后被重投。
- MySQL 双实例启动迁移互斥；持锁进程被 kill 后锁随连接释放。

### R2 — 调度闭环（第 3–4 周）

**目标：作业可追踪、可重试、可恢复。**

1. **延迟重试（#39）**：`Decision.RetryAfter` + `next_run_at` 落库；
   冷却期不消耗预算；`run_id` 唯一防重复排程。
2. **统一 Cron（#40）**：短期用配置版本号同步本地 entry；
   目标态改为 `next_fire_at` 条件更新抢占，调度器无状态化。
3. **消费者确认**：outbox 的「已投递」升级为 `event_deliveries`
   （consumer/status/attempt/next_attempt_at），通知渠道失败可重试。
4. **运行状态机**：`execution_attempts(job_id, attempt_no, worker_id, started_at,
   finished_at, result)` 独立表达每次尝试；运行记录不再兼任作业状态。
5. **部署模式（#42）**：`standalone / distributed / worker` 三模式，
   `distributed` 缺关键配置直接拒绝启动。

**R2 验收**

- MaxAutoRetries=3 且失败均秒回 → 恰好 3 次重跑；重启后延迟重试仍生效。
- 任一实例改 cron，其余实例 30s 内同视图；杀掉持有 entry 的实例，任务由他人接手。
- 通知渠道 500 → `event_deliveries` 重试到成功或人工可见的终态。

### R3 — 多实例验收（第 5–6 周）

**目标：把「分布式能力具备」变成「分布式故障已验证」。**

- K8s 多副本（API × N + Worker × M），`execute_jobs` 分离运行。
- 故障注入矩阵：
  | 注入 | 期望 |
  |---|---|
  | Worker SIGKILL | lease 过期后自动接管并完成 |
  | Redis 抖动/不可用 | 发布类操作 fail-closed；队列不受影响 |
  | MySQL 主从切换/瞬断 | 作业不丢、不重复执行；重试后收敛 |
  | 时钟偏移（±30s） | lease/reaper 不误判 |
  | 网络分区（Worker↔MySQL） | 旧 Worker 停手，不产生双写 |
- Worker 资源约束：磁盘水位检查、单机并发配额、优雅退出（先停止认领，再等在跑作业）。
- Clone Cache 保持 **per-Worker 本地**，不共享 PVC。

**R3 验收**：上述矩阵全部做成可重复的集成测试（或脚本 + CI job），
产出 `docs/design/multi-instance-verification.md` 记录实测数据。

### R4 — 治理增强（第 7–8 周）

1. **对账语义（#44）**：6 值状态机（`in_sync/drift/source_missing/
   target_missing/unknown/unsupported`）；错误分类下沉 SDK；
   重写历史仓库支持内容指纹判据；`unknown` 单独计数不污染漂移率。
2. **迁移映射（#41）**：`migration_mapping` 以
   `(source_platform, source_repo, object_type, source_number)` 为唯一键；
   支持增量与断点续迁。
3. **恢复验证**：恢复点不再只查文件存在——周期做完整性校验
   （bundle verify / 隔离目录 restore + refs 比对）并记录演练结果。
4. **容量基准**：100 / 1,000 / 10,000 任务三档，记录吞吐、队列延迟、
   lease 续租开销、DB 增长；基线单独成文（可复现）。

## 3. 目标数据模型

```
execution_task_states
  task_key (PK), desired_generation, completed_generation,
  active_job_id, updated_at

execution_jobs
  id, task_key, generation, status, owner, lease_version,
  lease_until, next_run_at, queued_at, finished_at

execution_attempts
  id, job_id, attempt_no, worker_id, started_at, finished_at, result

outbox_events
  id, topic, aggregate_id, payload, created_at

event_deliveries
  event_id, consumer_name, status, attempt, next_attempt_at
```

关键取舍：`ExecutionJob` 不再同时承担「触发去重 / 执行状态 / 接管信息」；
重试由 `execution_attempts` 表达，避免运行记录与作业状态互相污染。

## 4. 部署拓扑与节点维度

```
            ┌────────────┐        ┌────────────┐
  webhook → │ API × N     │ enqueue│ MySQL      │
  UI/CLI  → │ execute=false│──────→│ (共享)      │
            └────────────┘        └─────┬──────┘
                                        │ claim/lease
            ┌────────────┐              │
            │ Worker × M  │←─────────────┘
            │ execute=true│  per-worker Clone Cache（本地盘）
            └────────────┘
            Scheduler：DB CAS（next_fire_at），无 leader election
```

下一版补充节点调度维度：`region / network_zone / capabilities /
platform_access`。内网 GitLab 的同步任务由内网 Worker 主动领取，
公网控制面不直接访问内网 Git 服务。

## 5. 发布验收标准

| 指标 | 目标 |
|---|---|
| Webhook 不丢提交 | 100% 通过确定性并发测试 |
| 任务重复投递 | 无无保护的重复目标写入 |
| Worker SIGKILL 后恢复 | lease 过期自动接管并完成 |
| Redis 异常 | 关键发布操作拒绝不安全降级 |
| MySQL 多实例启动 | Schema 迁移互斥且可重试 |
| Outbox 故障恢复 | 未确认事件可再次投递 |
| 备份恢复验证 | 恢复后 Git 对象与 refs 校验通过 |
| 规模基准 | 100 / 1,000 / 10,000 任务三档有实测记录 |

## 6. 本轮明确不做

- Kafka / Temporal / 完整微服务拆分（运维复杂度换不来已识别竞态的修复）。
- 扩充更多 Git 平台支持与更多 Web 页面（等 R1–R3 验收通过后再评估）。
- 扩大 Agent 功能范围。
- 恢复点只做「文件存在性检查」——必须有完整性校验与隔离恢复演练。

## 7. 版本基线

| 组件 | 当前 | 本轮目标 |
|---|---|---|
| git-ferry（壳） | v3.2.0（core v0.50.0 + SDK v0.143.0） | R1 起按阶段发版 |
| git-ferry-core | v0.50.0 | R1 完成后 v0.51.0（状态模型变更，注明迁移） |
| go-git-platform | v0.143.0 | 跟随上游，不跨 major |
| git-ferry-intranet | `71a71e3`（core v0.50.0 / SDK v0.143.0） | 随下次 tag 发布 |