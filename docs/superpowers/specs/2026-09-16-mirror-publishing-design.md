# 镜像中心:开源发布与仓库备份 前端设计

> 2026-09-16 · 状态:待评审
> 后端基座:git-sync-core/mirror(已落地,commit 5b44717)· 本设计覆盖壳层 API + 前端页面

## 0. 概念与命名

一级实体:**镜像通道(MirrorChannel)** = 一条"源仓库 → N 个远端仓库"的配置。按用途分两种模式:

| | 🚀 开源公开发布(publish) | 🗄 仓库备份(backup) |
|---|---|---|
| 目的 | 供第三方 `go get`,走 proxy | 灾备/多托管,代码保真 |
| 目标可见性 | 公开仓库 | 通常私有仓库 |
| 身份改写 | **是**(module 全量改写,孤儿快照) | **否**(tag 原样指针) |
| 编译门禁 | 需要(改写可能破坏编译) | 不需要(零代码变更) |
| 确认卡 | 需要(改写清单/分歧覆盖) | 简化(常规操作,分歧仍拦截) |
| 验证 | `go list -m` 四项证明 | ref hash 本地/远端比对 |
| 典型节奏 | 手动发版后触发 | 自动跟随新 tag / cron |

> 私有备份但也要能 go get 的场景(私有 GitHub + GOPRIVATE):用 publish 模式指向私有仓库即可——两种模式按"是否改写身份"区分,与公私无关。

UI 术语:通道、映射(仅 publish 模式)、版本、发布记录。菜单名:**「镜像中心」**,通道卡片带模式徽标。

| 概念 | UI 名称 | 说明 |
|------|---------|------|
| 镜像通道 MirrorChannel | **镜像通道** | 一条"源仓库 → N 个远端"的配置,含模式 |
| 映射 Mapping | **映射**(publish 模式) | `源 module → 目标 module`,一个通道 N 条 |
| 备份范围 Scope | **备份范围**(backup 模式) | 备份哪些 refs:全部 tag / 自选 tag / 附加分支 |
| 版本 Version | **版本** | tag × 目标 的状态矩阵单元格 |
| 发布 Run | **执行记录** | 一次发布/备份动作的异步执行记录 |

## 1. 信息架构

```
🗂 镜像中心                 → /mirror             通道列表(双模式)
   └─ 通道详情              → /mirror/:id         映射/备份范围 + 版本矩阵 + 执行记录
```

- 新增一级菜单「镜像中心」(与 8-25 重构的精简侧边栏并存,排在「仓库管理」之后)。
- **不**做成仓库配置页的 tab:发布/备份是跨仓库工作流(源仓库只是通道的一个属性),且有独立的记录视图;但仓库配置页放一个「发布/备份」入口按钮,携带 repoKey 跳转新建通道表单预填。
- 新建通道第一步选模式,两张模式卡片(图 + 一句话差异),选中后表单按模式变化。

## 2. 页面设计

### 2.1 通道列表 /mirror

```
┌──────────────────────────────────────────────────────────────┐
│ 镜像中心                    [模式:全部▼]        [+ 新建通道]   │
├──────────────────────────────────────────────────────────────┤
│ ┌────────────────────────────┐ ┌────────────────────────────┐ │
│ │ 🚀 agentkit → GitHub 开源   │ │ 🗄 agentkit → 备份托管       │ │
│ │ 源: agentkit (enjoye)      │ │ 源: agentkit (enjoye)      │ │
│ │ git.enjoye.top/…/agentkit  │ │ 全部 tag · 含 main          │ │
│ │   → github.com/yi-nology/… │ │ 已备份 12 个版本 · 全部一致   │ │
│ │ 已发布 6 个版本 · 全部通过验证│ │ 最近备份: v0.10.3 · 2h 前   │ │
│ │ 最近发布: v0.10.3 · 2h 前   │ │                            │ │
│ └────────────────────────────┘ └────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

卡片要素:模式徽标、通道名、源仓库、映射摘要(publish)/备份范围(backup)、版本数、验证状态、最近执行时间。空态给引导文案 + 新建按钮。

### 2.2 通道详情 /mirror/:id(核心页面)

自上而下四块:

```
┌─ ① 通道信息卡 ────────────────────────────────────────────────┐
│ agentkit → GitHub 开源镜像      源仓库: agentkit  [编辑] [⚠删除] │
│ module: git.enjoye.top/enjoydream/agentkit                    │
│ 健康: ● 验证通过 (v0.10.3)      最近发布: v0.10.3 · 2h 前       │
├─ ② 发布映射(module 映射表)────────────────────────────────────┤
│ 源 module(只读,取自 go.mod) → 目标 module    远端   凭据   操作  │
│ git.enjoye.top/…/agentkit → github.com/yi-nology/agentkit     │
│                            github   已配置  测试连接/编辑/删除  │
│                                            [+ 添加映射(多仓)]  │
├─ ③ 版本矩阵 ──────────────────────────────────────────────────┤
│ [选择 tag ▼][发布选中(2)]           [批量补历史] [刷新]          │
│ tag      状态        发布时间   commit  验证    操作             │
│ v0.10.3  ●已发布·验证通过 2h前  6cfe48b  ✓通过   验证|复制go get │
│ v0.10.2  ●已发布·未验证   3d前  f8af472  –      验证|重新发布    │
│ v0.10.1  ●已发布·分歧⚠    5d前  4a93f96  ✗tree不一致 重新发布   │
│ v0.9.9   ○未发布          –     –        –      [√]发布         │
├─ ④ 发布记录 ──────────────────────────────────────────────────┤
│ 时间     tag     目标          结果        耗时   详情           │
│ 22:41   v0.10.3  github ✓成功  门禁✓推送✓  38s   [drawer]      │
│ 22:39   v0.10.2  github ✗门禁失败 类型检查错误    12s  [drawer]   │
└──────────────────────────────────────────────────────────────┘
```

**② 发布映射 / 备份范围**(抽屉,按模式二选一):

- *publish 模式 — 映射编辑器*:源 module 只读自动带出(后端读 go.mod);目标 module 输入时给出建议(host 识别后替换为 `github.com/<org>/<repo>`);source==target 前端直接禁用;remote 名默认 `github`;凭据三选一:**使用本机凭据**(默认,gitbackend native)/ Token / SSH 私钥(明文只在创建时提交,后端加密存储,回显仅"已配置")。每条映射有「测试连接」。
- *backup 模式 — 备份范围编辑器*:目标列表(N 个远端,每项 = remote 名 + URL + 凭据,同上三选一);范围选择:**全部 tag**(默认,含未来新 tag)/ 自选 tag;附加分支(默认 main,可加其他分支);开关「自动跟随新 tag」(P2 实现,先存配置)。每项远端同样有「测试连接」。

**③ 版本矩阵**(两种模式共用,状态语义随模式):
- 数据 = `git tag 列表 ∪ 历史执行记录` 合并;publish 状态机:`未发布 → 预检中 → 发布中 → 已发布 / 门禁失败 / 远端分歧拒绝 / 已覆盖`;backup 状态机:`未备份 → 备份中 → 已备份 / 分歧拒绝 / 已覆盖`(无门禁步骤)。
- **单发**:勾选若干 tag → 「发布/备份选中」→ 走确认卡(见 §3)。
- **批量补历史**:多选历史 tag 队列逐个执行,行级结果(成功/失败互不阻塞)。
- 「重新发布」对已发布 tag 重跑(幂等,tree 应一致);publish 模式的「验证」对该行每个映射调 VerifyModule,backup 模式为 ref hash 比对。
- 行内「复制 go get」仅 publish 模式显示。

**④ 执行记录**:表 + 详情 drawer。drawer 内容复用执行记录页的**步骤时间线**样式——publish:`校验 → 身份改写(N 个文件)→ 类型检查门禁 → 远端一致性 → 推送 → 完成`;backup:`校验 → 一致性检查 → 推送 → 完成`,附报告 JSON(tree/commit 或 ref hash、覆盖标记;publish 另有 ReplacedFiles 清单),失败步骤展开错误摘要。进行中的 run 每 2s 轮询(vue-query `refetchInterval`),与同步记录模式一致。

## 3. 执行动作的状态机与确认卡

```
选择 tag → [预检 dry-run] → 确认卡 → [执行 run(异步)] → 结果 → [验证(可选)]
                │失败(publish:门禁不过)→ 展示错误,不可执行
                │分歧:远端同名 tag 指向不一致 → 分歧确认卡
```

**确认卡**(Modal,执行前必经):
- publish:映射可视化(`git.enjoye.top/…/agentkit ──▶ github.com/yi-nology/agentkit`)、将改写文件清单(可折叠,如 `11 个文件: go.mod、README.md、…`)、合成 commit 摘要(孤儿快照、继承源身份)
- backup:目标列表、将推送的 ref 清单(tag 原样指针 + 附加分支)、预估对象传输量级
- 两种模式都展示远端既有 tag 状态:「远端无此 tag,全新执行」/「远端已有,内容一致(幂等重跑)」/「⚠️ 远端内容不一致」
- 分歧时必须**勾选「我确认覆盖远端已存在的版本」**才允许提交(后端对应 `AllowOverwrite`),卡片显示双方 hash 并列。backup 模式通常配自动执行,故**仅手动触发时出确认卡**;自动跟随/cron 触发的 run 遇分歧一律拒绝并在记录中标记,等待人工处理。

**防呆清单**(前后端双重):
- publish:source == target 禁止;module 格式校验(复用 core 的字符集规则)
- backup:附加分支不得为空(至少 main);同一通道内目标远端不得重复
- 删除通道二次确认(输入通道名)
- 已执行版本重跑默认按幂等对待,只有内容不一致才升级为分歧确认

## 4. 壳层 API 契约(git-sync-service,复用现有 API Key 中间件)

corebridge 内包 `git-sync-core/mirror`;通道配置落库(新表 `mirror_channel` + `mirror_run`),凭据走现有加密存储。**通道实体带 `mode` 字段**,两种模式共用同一套端点,语义随模式变化。

```
GET    /api/mirror/channels                      通道列表(含统计摘要,可按 mode 过滤)
POST   /api/mirror/channels                      创建{mode, name, repoKey, mappings[]|targets[], scope?}
GET    /api/mirror/channels/:id                  详情
PUT    /api/mirror/channels/:id                  更新
DELETE /api/mirror/channels/:id                  删除(需 ?confirm=<通道名>)
POST   /api/mirror/channels/:id/targets/:tid/test   测试连接(ls-remote 探活)
GET    /api/mirror/channels/:id/versions         版本矩阵(git tags ∪ 执行历史合并)
POST   /api/mirror/channels/:id/preview          预检 {tag} → dry-run 报告(不推)
POST   /api/mirror/channels/:id/runs             执行 {tags[], allowOverwrite} → {runId}
GET    /api/mirror/runs?channel_id=&page=        执行记录(服务端分页)
GET    /api/mirror/runs/:id                      记录详情(步骤+报告)
POST   /api/mirror/verify                        publish:{module, version} 四项证明(服务端带重试)
                                                backup:{channelId, tag} ref hash 比对
```

关键响应示例:

```jsonc
// GET /api/mirror/channels/:id/versions
{ "mode": "publish",
  "module": "git.enjoye.top/enjoydream/agentkit",
  "versions": [{
    "tag": "v0.10.3", "commit": "0ecf06e…",
    "targets": [{ "targetId": 1, "target": "github.com/yi-nology/agentkit",
      "state": "published",            // unpublished|published|divergent|gate_failed(backup: unbackedup|backedup|divergent)
      "executedAt": "…", "tree": "f8af47…",
      "verify": "passed"               // unverified|verifying|passed|failed
    }]}]}

// POST /api/mirror/channels/:id/preview (dry-run,publish 模式)
{ "tag": "v0.9.9", "tree": "…", "replacedFiles": ["go.mod", "README.md", "…"],
  "gate": "passed",
  "remote": { "exists": false }       // 或 { "exists": true, "treeMatch": true|false } }

// POST /api/mirror/channels/:id/preview (backup 模式)
{ "tag": "v0.9.9", "refs": ["refs/tags/v0.9.9", "refs/heads/main"],
  "remote": { "exists": true, "hashMatch": true } }

// POST /api/mirror/verify
// publish:
{ "ok": true, "checks": { "reachable": true, "versionExists": true,
                          "zipFetch": true, "identityMatch": true }, "attempts": 1 }
// backup: { "ok": true, "checks": { "remoteReachable": true, "hashMatch": true } }
```

实现注意:
- publish 模式的执行为异步 run(类型检查 + 推送可能数十秒),复用现有 run/step 落库与分页查询模式;backup 模式同样走 run(推送量更大,更需异步);
- dry-run 与执行共用同一条 core 调用路径,publish 的 `preview` 即 `Publish` 的前四步(校验→改写→门禁→一致性)不推送;backup 的 `preview` 仅做 ls-remote 一致性检查与 ref 清单;
- **backup 模式的执行不需要 core/mirror 的改写逻辑**:壳层直接用 `gitbackend.Push`(refspecs = tag/分支,Force)实现即可,后续如需批量/一致性增强再下沉 core;
- 凭据仅创建/更新时提交,响应只回 `has_credential`(沿 platform 的 has_token 惯例);
- 操作日志埋点:创建/删除通道、执行、覆盖、验证。

**与「同步任务」的边界**(避免功能重叠):现有同步任务做的是平台级仓库持续同步(clone/fetch/push,webhook/cron 驱动,面向分支工作副本)。备份通道的差异在 **tag 快照维度**:原样 ref 指针、幂等 force 重跑、批量补历史 tag、与发布同源的执行记录体验。分支级持续容灾仍应使用同步任务;备份通道聚焦 tag 维度 + 少量附加分支。两个入口在 UI 文案中互相指引。

## 5. 前端实现落点

```
frontend/src/
  api/mirror.ts                  # MirrorApi:channels/versions/preview/runs/verify
  composables/useMirror.ts       # vue-query:useChannels/useVersions/useMirrorRun(轮询)/useVerify
  types/api.ts                   # MirrorChannel/MirrorTarget/VersionMatrix/MirrorRun(沿 snake+camel 惯例)
  views/mirror/
    MirrorChannelList.vue        # /mirror(模式徽标 + 类型过滤)
    MirrorChannelDetail.vue      # /mirror/:id(信息卡+映射/备份范围+矩阵+记录 四区块)
    components/
      ChannelFormDrawer.vue      # 新建/编辑通道(模式选择卡 → 映射行编辑器 / 备份范围编辑器)
      RunConfirmModal.vue        # 确认卡 / 分歧确认卡(双模式共用)
      MirrorRunDrawer.vue        # 记录详情(步骤时间线,复用同步记录样式)
  router/index.ts                # + /mirror, /mirror/:id
  components/layout/AppLayout.vue# + 菜单项「🗂 镜像中心」
```

## 6. 分期

- **P0(核心闭环,publish 模式)**:通道 CRUD + 版本矩阵 + 单/多 tag 发布(确认卡、轮询、分歧确认)+ verify + 执行记录列表
- **P1**:backup 模式(壳层 gitbackend.Push 实现:备份范围、ref hash 比对验证)、批量补历史队列、记录 drawer 时间线、测试连接、复制 go get
- **P2**:自动跟随新 tag(backup 优先:webhook tag-push / cron 触发通道)、发布模板、定时 re-verify、统计进仪表盘

## 7. 待确认

1. 菜单命名:「镜像中心」vs「开源发布」保留旧名仅做 publish?(倾向前者,backup 加入后旧名覆盖不住)
2. backup 模式放 P1 是否合适?若备份诉求更急,可与 P0 对调(backup 实现更薄,不依赖 core 新版本)。
3. publish 凭据 v1 是否接受"本机凭据 + 可选 Token/SSH"?(生产 72 无公网出口,GitHub 推送本就走不通,主要用于有出口的部署)
4. backup 的"附加分支"默认仅 main,是否需要默认全部分支?(倾向仅 main,全分支容灾归同步任务)
