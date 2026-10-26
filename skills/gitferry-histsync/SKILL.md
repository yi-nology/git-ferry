---
name: gitferry-histsync
version: 1.0.0
description: "GitFerry 历史同步(双仓内容差异发布):生成同步计划、裁决冲突、内容移植与推送、审计校验。当用户要把内网仓内容发布到公开仓、或问「两棵树怎么同步 / 公开仓少了什么 / 哪些文件该带过去」时触发。"
metadata:
  requires:
    bins: ["gitferry"]
  cliHelp: "gitferry histsync --help"
---

# gitferry-histsync(历史同步)

**前置：** 先读 [`../gitferry-shared/SKILL.md`](../gitferry-shared/SKILL.md)。
**所有命令加 `--format json`。**

历史同步把**内网仓**的内容差异整理成计划，叠加到**公开仓**上。
公开仓的既有提交**原样保留(hash 不变)**，新内容以新提交追加在末尾 ——
所以公开仓上的 tag、分支、PR 引用全部保持有效。

## 0. 先分清两种模式（名字容易撞车）

| | 身份镜像（镜像中心） | **历史同步（本技能）** |
|---|---|---|
| 命令 | `gitferry mirror …` | **`gitferry histsync …`** |
| 场景 | 同一仓库换个 module path 发布快照 | 两个**各自有完整历史**的仓库之间同步 |
| 产物 | 每 tag 一个孤儿快照 | 在目标树历史**之上**叠加内容差异 |
| 目标侧历史 | 无（快照无父提交） | **保留，不重写** |

> ⚠️ CLI 里 `history` 已被「同步执行历史」占用，历史同步的命令组是 **`histsync`**。

## 1. 核心概念

| 概念 | 含义 |
|---|---|
| **通道**(channel) | 一条同步配置：两侧仓库 + 身份映射 + 路径/痕迹规则 + 推送目标。**配置只存在通道里** |
| **计划**(plan) | 逐文件的处置：`same` / `port` / `port_with_scrub` / `exclude` / `hold` / `preserve` |
| **解法**(resolution) | `hold` 文件怎么解：`take_theirs` / `take_ours` / `merge_clean` / `conflict` / `no_base` |
| **执行记录**(run) | 一次 apply 的产物：计划快照 / commit 映射 / 检测报告 / ref 对照 + **哈希链** |
| **裁决**(override) | 人工或 AI 对某个 `hold` 文件的拍板结果，写回通道后参与下次 resolve |

**两条哈希链，职责分明：**

| 链 | 回答的问题 |
|---|---|
| 全局审计链（`OperationLog`） | **谁**在什么时候对**什么**做了什么 |
| 通道执行链（`HistoryRun`） | 这批同步的**审计材料**有没有被改 |

## 2. 标准流程

```bash
CH=1   # 通道 ID

# ① 看通道（首次用先建）
gitferry histsync +channel-list --format json

# ② 生成计划（纯读，不写任何东西）
gitferry histsync +plan --channel-id $CH --format json

# ③ 算解法 + 取冲突卷宗（纯读）
gitferry histsync +resolve --channel-id $CH --format json
```

④ **裁决冲突**（只有 `conflict` / `no_base` 需要，见 §3）

⑤ **预检**（不推送）—— 摊开这次同步的全部影响面
```bash
gitferry histsync +preview --channel-id $CH --format json
```
预检会告诉你：**推出去能不能编译**（门禁）、**将产生哪些提交**、
**每个远端的 ref 会怎么变**、**要不要强推**、**还有几个冲突没裁**。
**先看预检再决定要不要 apply** —— 预检与执行走同一套代码路径，结论一致。

⑥ **落地移植**（本地，不推送）→ 产出执行记录
```bash
gitferry histsync +apply --channel-id $CH --format json
```
⑦ **推送**（对外可见动作，需确认）
```bash
gitferry histsync +push --channel-id $CH --format json
```
**推送前服务端会做分叉检查**：远端出现了本次移植不知道的提交时，推送**直接被拒**
（报「已分叉」），不会把远端的提交覆盖掉。
确实要覆盖时，显式加 `--force-push`（**这会丢掉远端的提交，必须先向用户确认**）。

> `--force-push` **只允许 admin 角色**（HTTP / MCP / AI 助手三条入口一致）——
> 覆盖远端别人的提交是破坏性动作，operator 也不行。
⑧ **校验与审计**
```bash
gitferry histsync +channel-audit --channel-id $CH --format json   # 通道执行链
gitferry histsync +runs --channel-id $CH --format json             # 执行记录
gitferry histsync +run-detail --run-id <RUN_ID> --format json     # 审计材料
```

## 3. 你的职责边界（最重要）

**你只做语义判断。机械变换由确定性代码完成，不要插手。**

| 环节 | 谁做 | 你能不能碰 |
|---|---|---|
| 路径改写（module path 换身份） | 确定性代码 | ❌ |
| gofmt 归一化 | 确定性代码 | ❌ |
| 差异判定（谁领先、谁超集） | 确定性代码 | ❌ |
| 类型门禁（改写后能不能编译） | 确定性代码 | ❌ |
| 痕迹检测命中 | 确定性代码 | ❌ |
| **改动能否公开** | **你** | ✅ |
| **痕迹要不要脱敏** | **你** | ✅ |
| **冲突怎么解** | **你**（提建议，人确认） | ✅ |

> 这些机械环节已经有确定性实现，LLM 参与只会引入不确定性。
> 你看到 `same` / `port` / `port_with_scrub` / `exclude` / `preserve` —— **这些是结论，不要重判**。
> 你只需要处理 `conflict` 与 `no_base`。

### 冲突判据（先看有没有自动解掉）

通道可以开启**冲突自动判据**（`conflict_rules`），它只在确定性判定解不了时兜底：

| 判据 | 命中条件 | 结论 |
|---|---|---|
| `internal_trace` | 源侧独有行含内网形态（内网 registry / 内网域名 / 跳过证书校验 / 内网措辞） | 保留目标侧 |
| `public_asset` | 目标侧含公开资产（license 副本 / badge / 公共镜像源 / 多平台）而源侧没有 | 保留目标侧（源侧是旧版） |
| `placeholder_domain` | 源侧用真实对外域名，目标侧已脱敏为示例域名 | 保留目标侧 |

**开启判据后先 `resolve` 看还剩几个** —— 实测 11 个真冲突里 9 个被自动解掉。

> 基线是 11 不是 9：合并算法修好之后，有两个配置文件
> （`etc/config-dev.yaml` / `etc/config-prod.yaml`）从「假合并干净」变成了**真冲突** ——
> 之前源侧的真实域名会覆盖目标侧已脱敏的域名，然后跟着发布到公开仓。
> 这两个文件需要人裁决：保留目标侧的脱敏域名（`keep_target`）。
剩下的是「两版功能都全、只是新旧不同」的真语义判断，**那才是你该管的**。

### 冲突裁决

`resolve` 返回的 `dossiers` 里每个文件带：`base`（共同基点）、`ours`（目标侧现状）、
`theirs`（源侧现状）、`merged`（带冲突标记的合并结果）、
`ours_only` / `theirs_only`（各自独有的行）。

**判定顺序：**

1. 先看 `theirs_only` 的**性质**，不是数量：
   - 内网特有内容（内网地址、内部部署脚本、只对内网有意义的配置）→ **公开树不该有** → `keep_target`
   - 公开化改造（去内网痕迹、补社区文件、license 章节、CI）→ **该保留** → `take_source`
   - 同义改写（措辞不同、意思一样）→ 取更适合公开树的措辞 → `merged`
2. `ours_only` 同理反向判断。
3. 两侧改的是**同一处** → 看哪边语义更新。

完整词表与实例见 [`references/conflict-adjudication.md`](references/conflict-adjudication.md)。

### 写回裁决

裁决**改变实际发布的内容**，必须让用户确认：

```bash
# keep_target / take_source
gitferry histsync +override-set --channel-id $CH \
  --path "internal/service/provider/service.go" \
  --verdict keep_target --reason "源侧新增的是内网专有的 provider 适配" \
  --decided-by "agent:<你的标识>"

# merged：把裁决后的内容写进文件再提交
gitferry histsync +override-set --channel-id $CH \
  --path "README.md" --verdict merged \
  --content-file /tmp/merged-README.md \
  --reason "两侧都要：开源章节取目标侧,新特性说明取源侧" \
  --decided-by "agent:<你的标识>"
```

写回后**重新 resolve** 确认解法已生效，再 apply。

## 3.5 一条命令（预检 → 移植 → 推送）

```bash
gitferry histsync +sync --channel-id $CH --format json
```

服务端会**先预检再决定要不要往下走**。有下列阻塞项时**停下并报告**：

- 门禁不过（推出去编译不过）
- 还有待人工裁决的冲突
- 需要强推（可能覆盖远端别人的提交）

这三件都不是程序能替用户决定的 —— **停下让人看，比推上去再回滚便宜**。
`--force` 可以让阻塞项也继续（推送本身仍需确认）。

### 3.6 门禁：哪些是 CI 会跑的，哪些要手工跑

| 门禁 | 什么时候跑 | 守什么 |
|---|---|---|
| `merge3_fuzz_test.go`（随机对拍） | `go test ./...`（CI 会跑） | 合并结果不丢内容、不重复内容 |
| `merge3_oracle_test.go`（与 `git merge-file` 逐字对拍） | `go test ./...` | 合并语义与 git 一致 |
| `history_e2e_test.go`（端到端演练） | `go test ./...` | 建通道→裁决→预检→移植→推送→回滚→再推 整条流水线 |
| `live_test.go` / `merge3_live_test.go`（实仓） | **手工**：设 `HISTORY_LIVE_SRC` / `HISTORY_LIVE_TGT` | 真实双仓的基线与「公开树不得含真实域名」 |

实仓那两条**需要环境变量才会跑**（没设就 skip）——不是 CI 门禁，
但它是唯一能在**真实仓库**上验证「公开树不含内网真实域名」的门禁，值得定期手工跑。

### 3.7 凭据轮换

```bash
gitferry histsync +target-rotate --target-id <ID> \
  --cred-type ssh_key --credential-file ~/.ssh/id_ed25519
```

**轮换只换密钥，不动 URL 与目标名** —— 换密钥是常规运维动作，
不该有机会顺手改掉推送到哪里。旧接口 `+target-create` 改的是整个目标。

## 4. 安全规则

1. **`+push` 是对外可见动作** —— 公开仓会被改动。**必须先向用户确认**。
   推送顺序固定为「先留底后发布」：留底失败则不发布。
2. **`+rollback` 会强推目标分支** —— 会把公开仓退回某个 ref。必须先确认，并展示影响面
   （当前 HEAD → 目标 ref）。
3. **`+override-set` 改变发布内容** —— 需用户确认。
   CLI 侧（`+override-set` / `+channel-create` / `+channel-update` / `+target-create` / `+apply`）
   由服务端要求确认，脚本/自动化场景统一加 `--yes` 跳过。

   **各入口的确认口径**（确认在哪一侧做、谁负责角色）：

   | 入口 | 确认由谁做 | 服务端的闸门 |
   |---|---|---|
   | CLI | 服务端（`--yes` 跳过） | 角色 + `force_push` 要 admin |
   | 内置 AI 助手 | 服务端（会话内登记待确认） | 角色 + `force_push` 要 admin |
   | MCP（Claude Code / Cursor 等） | **客户端**（MCP 无状态，服务端登记的令牌无处可去） | 角色 + `force_push` 要 admin |
   | HTTP API | 调用方自理 | 角色 + `force_push` 要 admin |

   MCP 路径曾经**危险工具一律不可用**（缺会话 scope → 直接拒绝），
   现已可用：确认交给客户端，服务端只管角色与强推授权。

   **另有三类工具也在服务端闸门内**（与历史同步无关，但同属 agent 工具面）：
   - `remember` / `forget_memory`：记忆会进**每次会话的 system prompt** ⇒
     跨会话持久化的提示注入通道，故按危险工具处理（需确认 + 只读角色一律拒绝）。
   - `set_permission_mode`：切换工具权限档是**治理动作** ⇒ 需确认 +
     **只有 admin 能拨**，且档位是**会话级**的（一个会话切只读，不影响其他会话）。
   - `ai_code_delegate`：把任务委派给**外部 coding agent**（跑在本机上的子进程，
     拥有该用户的文件与网络权限）⇒ 按危险工具处理（需确认 + 只读角色一律拒绝）。
     注意：委派会**占住那条确认请求**直到外部 agent 跑完（确认走 SSE，连接保持打开）。
4. **凭据不落命令行** —— 推送目标的密钥用 `--credential-file` 传，不要用 `--credential`
   （会进 shell 历史）。服务端的凭据是**加密落库**的，响应里不回显。
5. **不要自己改 `preserve` 的文件** —— 它们是公开树独有的资产（截图、license、Gerrit 集成），
   覆盖会丢东西。
6. **推送被「已分叉」拦下时，先查清远端多出来的提交是谁的** —— 不要顺手 `--force-push`。
   强推会把远端的提交**静默丢掉**（没有别的痕迹）。

## 5. 常见陷阱

| 现象 | 原因 | 处理 |
|---|---|---|
| 预检报 `needs_force` | 远端与本地基线已分叉 | **停下来问用户**：要强推覆盖远端，还是先重新计划 |
| 预检报门禁不通过 | 物化结果没通过 gofmt / 类型检查 | **不要 apply** —— 把门禁错误原样给用户，它指出具体文件与行号 |
| apply 报「门禁未通过」 | 产物那棵树编译不过 | **不要 push** —— 门禁结论已落进执行记录，推送侧会独立核对并拒绝；改完源仓重新 apply |
| push 报「门禁未通过」/「产物 HEAD 与执行记录不一致」 | 产物在 apply 之后被改过，或执行记录里没有通过的门禁结论 | 重新 apply 生成新的产物，**不要**手改产物目录 |
| 预检的 new_head 与 apply 后不一致 | 正常：预检是快照 | 以 apply 的实际结果为准；不一致说明期间仓库变了，重新预检 |
|---|---|---|
| `hold` 全是「措辞差异」 | 两棵树是同源改写副本，措辞不同但意思一样 | 多数应在 `take_ours` / `take_theirs` 里自动解掉；仍冲突才需你判 |
| apply 报编译不过 | port 的文件依赖某个还在 `hold` 的文件 | `resolve` 看 `blocked`/依赖关系；先解掉那个 hold |
| push 报认证失败 | 目标的凭据不对 | `histsync +target-list` 看 `has_credential`；更新目标后重试 |
| push 报「已分叉」 | 远端比本次移植的基线新（有别人推的提交） | **不要直接 `--force-push`** —— 先更新目标仓、重新 `+resolve` / `+apply`；确实要覆盖再问用户 |
| rollback 报「解析回滚目标失败」 | 给的 ref 既不是留底上的分支，也不是仓库里的提交 | 用 commit hash（`+runs` 里的 `base_head` / `head`）或留底上的分支名 |
| 想「撤销」一次发布 | 误推了 | `+rollback --run-id <ID> --ref <commit>`（留底里仍有内容，可再推回） |
| 通道改了但计划没变 | 改了配置没重新 resolve | 计划是**快照**，每次都重新 resolve |

## 6. 首次配置一个通道

```bash
gitferry histsync +channel-create \
  --name code-review \
  --source-dir /srv/repos/review-service \
  --target-dir /srv/repos/code-review-service \
  --module-from git.enjoye.top/enjoydream/code-review-service \
  --module-to github.com/yi-nology/code-review-service \
  --host-from git.enjoye.top --host-to github.com \
  --path-rules "docs/internal=docs/archive" \
  --target-ref main \
  --author-name "Zhangyi" --author-email "zhangyi@example.com" \
  --format json

# `--source-dir` / `--target-dir` 必须指向**真实的 git 仓库**
# （不能是 /etc、~/.ssh 这类位置 —— 那是凭据与系统配置，不是待发布的代码）。
#
# `--author-name` / `--author-email` 是**公开树里新提交的作者身份**。
# 不填则用中性身份 `git-ferry <git-ferry@users.noreply.github.com>` ——
# **源提交的作者不会被继承**（源侧是内网开发者，继承等于把内网人员信息发到公开仓）。

# 推送目标：第一个是留底（裸镜像），其后才是发布
gitferry histsync +target-create --channel-id $CH \
  --name backup --url /srv/mirrors/code-review.git --cred-type none --format json
gitferry histsync +target-create --channel-id $CH \
  --name publish --url git@github.com:yi-nology/code-review-service.git \
  --cred-type ssh_key --credential-file ~/.ssh/id_ed25519 --format json
```

## References

- [冲突裁决手册](references/conflict-adjudication.md) —— 判定词表与实例
- [gitferry-shared](../gitferry-shared/SKILL.md)
- [gitferry-ops](../gitferry-ops/SKILL.md) —— 全局审计链与运维面
- [gitferry-workflow](../gitferry-workflow/SKILL.md)
