// Package converter 负责把 core 层领域实体转成 thrift 生成的对外 DTO。
//
// # 搬运机制
//
// 本包不再逐字段抄写。核心是 mapper.go 里的 mapFields + fieldSpec:
//
//   - 字段按「归一化名」匹配:去掉 '_' 并转小写,于是 core 的 InstanceURL
//     与 thrift 的 InstanceUrl、core 的 GitLFS 与 thrift 的 GitLfs、
//     core 的 CommitSHA 与 thrift 的 CommitSha 都能对上。
//   - 类型转换内置:uint→int64 直转;int/int64→int32 区间钳制;
//     time.Time/*time.Time→string 按家族格式化(nil 指针保持空串);
//     *uint→*int64 逐层转换;[]struct→[]*struct 逐元素转换。
//   - fieldSpec 只声明「例外」:
//     aliases(两边概念同名但拼写不同构,如 DTO.Time ← core.CreatedAt)、
//     skip(secret 字段、列表视图刻意省略的字段)、
//     derived(值是算出来的,如 HasToken = AccessToken != "")。
//
// 因此 core 实体加字段、thrift DTO 同步加字段之后,本包通常不需要改;
// 真要改,也只是在对应 fieldSpec 里加一行例外,而不是抄一遍字段。
//
// 安全网在 mapper_test.go:
//
//   - TestFieldSpecsCoverEveryDTOField:每个 DTO 字段要么被自动搬走,
//     要么在契约里显式声明 skip/derived;漏掉一个,测试立刻红。
//   - golden_test.go:对外 DTO 的 JSON 形状(字段名、顺序、零值、omitempty)
//     逐字钉死,期望值取自重构前的实现。
//
// 本包仍然逐字段的部分只有 ApplyPlatformUpdate:那是「更新策略」而不是
// 「字段搬运」——哪些字段允许改本身就是业务口径,各字段的"未更新"哨兵
// 也不统一(空串 / nil 指针 / >0),反射搬运表达不了这套语义。
//
// # 三层字段名对照(core / thrift / agent)
//
// 同一个概念在三层里的名字并不一致。下表只列真正对不上的部分,
// 一致的字段不再罗列(core 字段名 → thrift json 名 → agent json 名)。
//
//	概念                    core (model)              thrift DTO (json)          agent 视图 (json)
//	----------------------  -------------------------  --------------------------  --------------------------
//	任务源仓库              SyncTask.SourceRepoKey    source_repo_key            source_repo        ⚠ 改名
//	任务目标仓库            SyncTask.TargetRepoKey    target_repo_key            target_repo        ⚠ 改名
//	仓库归属者              Repo.PlatformOwner        platform_owner             owner              ⚠ 改名
//	平台最近测试结果        Platform.LastTestResult   last_test_result           last_test          ⚠ 改名
//	平台仓库数              Platform.RepoCount (int)   repo_count (int32)         repo_count (int)   ⚠ 类型不同
//	运行触发来源            SyncRun.TriggerSource     trigger_source             trigger            ⚠ 改名
//	运行开始时间            SyncRun.StartTime         start_time                 started_at         ⚠ 改名
//	运行主键                SyncRun.ID (uint)         id (int64)                 id (uint)          ⚠ 类型不同
//	审计操作者              OperationLog.Actor        user                       (agent 不暴露操作日志)
//	审计时间                OperationLog.CreatedAt    time                       (agent 不暴露操作日志)
//	审计详情                OperationLog.Detail       details                    (agent 不暴露操作日志)
//	webhook 事件标识        WebhookEvent.EventID      event_id                   (agent 不暴露 webhook 事件)
//
// 不在表内、但值得知道的两点:
//
//   - agent 侧的 webhook 规则视图是用 map[string]any 拼的,字段名
//     (repo_key / event_type / action)与 thrift 一致,不是本包的问题。
//   - agent 侧有 get_audit_chain,但只返回哈希链校验结论,不暴露操作日志列表。
//
// 另外,core 内部的 json tag 风格本身也不统一:SyncTask / Repo / Platform /
// OperationLog 用 snake_case,而 SyncRun / SyncRunStep / WebhookRule /
// WebhookEvent 用 camelCase(json:"taskKey" 等)。thrift 与 agent 两层
// 则统一是 snake_case。
//
// # 已知差异与后续收敛(本包不改,留给上层决策)
//
// 上面带 ⚠ 的差异里,thrift 一侧是本包的既有对外契约,不能在本包内改;
// 真正待收敛的是 agent 一侧——它给 LLM 消费,改字段名会影响工具输出与 prompt。
// 建议(需上层确认后再动,本次未改):
//
//   - internal/agent/tools/task.go:taskSummary.SourceRepo → SourceRepoKey
//     (json source_repo_key)、TargetRepo → TargetRepoKey;
//     赋值处 listTasks / getTask 两处。
//   - internal/agent/tools/repo.go:repoSummary.Owner → PlatformOwner
//     (json platform_owner);赋值处 listRepos / getRepo 两处。
//   - internal/agent/tools/platform.go:platformSummary.LastTest → LastTestResult
//     (json last_test_result);赋值处 listPlatforms。
//   - internal/agent/tools/history.go:runSummary.Trigger → TriggerSource
//     (json trigger_source)、StartedAt → StartTime (json start_time)、
//     ID uint → int64;赋值处 toRunSummary。
//   - internal/agent/tools/memory.go:runView.ID uint → int64。
//
// 收敛方向是「agent 视图沿用 thrift 的 json 名」,这样同一概念在三层只有
// 一个名字;代价是 agent 工具输出对 LLM 而言是一次破坏性变更。
package converter
