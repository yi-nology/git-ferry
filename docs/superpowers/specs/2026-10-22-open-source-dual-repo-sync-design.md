# 历史同步:双仓内容差异发布 需求与设计 v2

> 2026-10-23 · 状态:待评审(v2,按实测基线修订)
> 形态:git-ferry 新增「历史同步」通道模式(git-ferry-core 扩展 + 壳层 API + CLI/MCP + Agent Skills)
> 关联:`2026-10-22-open-source-dual-repo-sync-design.md`(v1 设计稿)、`2026-10-23-history-sync-baseline.md`(实测基线)
> 首个落地仓:`git.enjoye.top/enjoydream/code-review-service` → `github.com/yi-nology/code-review-service`

---

## 0. v2 相对 v1 的变更

v1 的方向判断成立(「两个各自有完整历史的仓库」确实是镜像中心既有 publish 模式未覆盖的场景),
但**核心机制在真实数据上不成立**。v2 按实测基线修订三处,并新增三个拍板项。

| # | 变更 | 依据 |
|---|---|---|
| C1 | **主轴从「提交差异」改为「文件级内容差异」** | v1 的 `(subject, author, date)` 匹配键实测 18/18 失败;内容差异实测把待办量从 18 个提交收敛到 43 个文件 |
| C2 | **明确流水线顺序:改写 → gofmt 归一化 → 差异判定** | 不归一化会产生 46 个假差异(`hold` 虚高 86 vs 40) |
| C3 | **术语分层** | 镜像中心 README 已用「双仓」表达「同一 module 的两个身份」,与本需求语义冲突 |
| C4 | 新增「保护集」处置类型 | v1 的四类处置无法表达「目标侧领先,不得覆盖」 |
| C5 | 新增合并策略定义 | v1 的「逐提交移植」在两树分叉模型下未定义 |
| C6 | **门禁必须跑在「应用后的目标树」上,并把失败翻译成「谁挡住了谁」** | 实测:port 集合存在隐式依赖,门禁拦下一次会编译不过的移植 |
| C7 | **hold 的自动解法:内容级共同基点 + 三向合并** | 实测:两树无 git 共同祖先但有内容级共同基点,40 个 hold 里 **30 个可机械解决**,收敛人工面到 10 个 |

**术语约定**(C3):

| 术语 | 含义 | 既有对应 |
|---|---|---|
| **身份镜像**(identity mirror) | 同一仓库的多个身份,每个 tag 一个孤儿快照 | 既有 `mirror.Publish` 模式,产品文案已称「镜像中心」 |
| **历史同步**(history sync) | 两个各自有完整历史的仓库,按内容差异叠加 | 本需求 |

产品文案中「双仓/多仓」一词今后**专指身份镜像**;本需求一律称「历史同步」。
代码落点:`git-ferry-core/mirror/history` 包。

---

## 1. 背景与现状(实测)

### 1.1 问题的真实形态

内网与公网双树不是「同一个仓库加两个 remote」,而是**两个各自有完整历史的独立仓库**:

| | 内部树 | 公开树 |
|---|---|---|
| 路径 | `~/my_project/review-service` | `~/my_project/code-review-service` |
| module path | `git.enjoye.top/enjoydream/code-review-service` | `github.com/yi-nology/code-review-service` |
| 提交数 | 408 | 406 |
| tag | `v0.2.0` | `v0.1.0` `v0.2.0` `v0.2.1` |

### 1.2 两树的真实差异形态(v1 判断错误处)

v1 写「两侧 subject 序列逐条对应,仅在开源目录卫生这类条目上措辞不同」。**实测不成立**:

- 按 `(subject, author, authorDate)` 全量匹配:共同 390、**仅内部 18**、**仅公开 16**
  (v1 写的是「内部独有 2 项 / 公开独有 3 项」)
- 最近 8 条提交里只有 **3 对**对应 —— Gerrit 移植与 release CI 的提交**插在序列中间**,不在尾部

两树的真实关系是「**同一工作、不同提交切分/措辞**」:

| 内部 | 公开 | 差异 |
|---|---|---|
| `f13f2d68` docs: 补「预编译二进制」**构建**说明 | `583d4efc` docs: 补「预编译二进制」**发布**说明 | 同日同人同工作,subject 差一个字 |
| `86f9ba9e` fix(ci): 格式检查步骤报出违规文件名 | `55fc7cf6` fix(ci): 修正 worker.go 的 import 排序 **+** 检查步骤… | 公开侧把两个改动合成一个提交 |
| `336ab6ea` fix(security): OCR 子进程不再继承…凭据 | `d879e772` fix(security): …**+ 修正 import 排序** | 同上 |

**决定性实测**:把判为「内部独有」的 18 个提交逐个 cherry-pick 到公开树 HEAD ——

```
成功 0    空提交 0    冲突 18
```

冲突不是「补丁不适用」,是**重复应用**:这些改动内容早已在公开树里,只是提交形态不同。
若沿用 v1 的 F6(「冲突时不自动解,进入 hold」),这 18 个会**全部 hold**,需求卡死在 M3。

### 1.3 路径改写的真实规模

| 项 | 数量 | 核对 |
|---|---|---|
| `go.mod` 的 `module` 行 | 1 | ✅ |
| 唯一 import 目录 | 98 | ✅ |
| import 引用总数 | 688 | ✅ |
| **引用模块路径的 `.go` 文件** | **262** | ❌ v1 写「30+」,低估 8 倍 |
| `biz/model/*` 生成物 | 0(不含模块路径) | ✅ 无需重新生成 |
| `idl/*.thrift` | 0(不自引用) | ✅ 无需重新生成 |

后两行是关键:改写不需要重新生成 codegen 产物,改动只落在手写代码的 import 上。

### 1.4 既有手工作法的三个缺陷

1. **改写用 `sed`/`ReplaceAll`,无边界保护** —— 会把 `github.com/a/b-extra` 中的 `github.com/a/b` 也换掉。
2. **改写后校验靠人肉** —— 模块路径前缀变化打乱 import 分组字典序,`gofmt` 报 diff。
3. **内网痕迹清理靠人工扫** —— 无门禁。

### 1.5 真实待办量(基线)

实测基线(详见 `2026-10-23-history-sync-baseline.md`):

| 处置 | 数量 | 说明 |
|---|---|---|
| `same` | 609 | 经改写 + gofmt 归一化后两侧一致 |
| `hold` | 40 | 两侧各有独有内容,需人工/AI 判定 |
| `preserve` | 39 | 受保护,移植时不得覆盖(27 仅公开树存在 + 12 目标侧内容领先) |
| `port` | 3 | 源侧为超集,以源侧内容覆盖 |
| `port_with_scrub` | 0 | — |
| `exclude` | 0 | — |

**结论:待办量是 3 个文件直接移植 + 40 个文件需判定 + 39 个文件受保护**,
不是 v1 估计的「18 个提交待移植」。

---

## 2. 与既有身份镜像模式的关系(边界澄清)

| | 既有 publish 模式(**身份镜像**) | 本需求(**历史同步**) |
|---|---|---|
| 目的 | 同一模块两个身份都可 `go get` | 两个各自有完整历史的仓库 |
| 目标形态 | 每 tag 一个**孤儿快照**(无父提交) | **在目标树上叠加内容差异**,保留目标树历史 |
| 历史关系 | 各版本互不依赖,可重跑任一历史 tag | 目标树既有提交原样保留(hash 不变) |
| tree 一致性 | 同 tag 重跑 tree 必须逐字节一致 | 按内容差异推进,允许目标树有额外内容 |
| tag | 源 tag 全量映射 | 两树 tag 可不一致 |
| 目录差异 | 不涉及 | 需要(`docs/internal/` ↔ `docs/archive/`) |
| 典型用途 | 依赖方免改 import | 开源发布 + 保留发布历史 |

**实现取向**:复用 `core/mirror` 的**改写器**与**门禁**,但不复用其「合成孤儿 commit」的发布模型。

> v1 写「复用 `rewriteTree`/`rewriteModulePath`,不做二次实现」——**代码层面不成立**:
> `mirror` 包的导出面只有 `Publish` 与 `ReadModulePath` 两个函数,上述两个函数与 `builder`
> 全是未导出的小写标识符,`service` 包调不到。详见 §7 的 D6。

---

## 3. 核心数据模型

### 3.1 历史同步通道(HistoryChannel)

一条「内部源仓 → 公开目标仓」的同步配置,**DB 持久化**。四张表:

```
HistoryChannel                通道:两侧仓库 + 身份映射 + 规则
  ├─ name                      通道名(唯一)
  ├─ sourceRepoKey / targetRepoKey   两侧仓库(取自 Repo 表)
  │  └─ sourceDir / targetDir        或本机已有克隆时直连
  ├─ moduleSource / moduleTarget     身份映射(module 必填)
  ├─ hostSource / hostTarget         主机名映射(可选)
  ├─ pathRules    路径规则 JSON:[{from,to}](to 为空 = 排除)
  ├─ scrubRules   痕迹规则 JSON:[{name,category,pattern,severity,replacement}]
  ├─ conflictRules 冲突自动判据 JSON:[{kind,enabled,patterns}]
  │                kind: internal_trace | public_asset | placeholder_domain
  ├─ sourceBranch / targetBranch / enabled
  └─ Targets []*HistoryTarget  (关联)

HistoryTarget                 推送目标(凭据加密落库)
  ├─ name / repoUrl
  ├─ credType / credential(密文) / username
  └─ 顺序 = 推送优先级:第一个是留底,其后才是发布

HistoryOverride               裁决覆盖(人工/AI 对某个 hold 文件的拍板)
  ├─ path / sourcePath
  ├─ verdict:keep_target | take_source | merged
  └─ content(仅 merged) / reason / decidedBy

HistoryRun                    执行记录(审计留痕 + 回滚锚点)
  ├─ kind: apply | push | rollback   一次动作一条记录
  ├─ pushedRunId              push/rollback 引用被操作的 apply 记录
  ├─ baseHead / head          动作前后的目标 HEAD
  ├─ workRepoPath             产物路径(推送时服务端自取)
  ├─ planSummary / commits / files / skipped / scrubReport / gateReport
  └─ refsBefore / refsAfter   推送前后 ref 对照
```

**接口是通道优先的**(v2.1 重做,见 §3.6):配置只在通道里,执行按 channel id 进行,
调用方**不重传规则、也不回传服务端的临时路径**。

**为什么通道化**:「有哪些双仓在同步、各自什么规则」成为**可查询的状态**。
执行记录同时是审计留痕与回滚锚点 —— 推送用哪个产物、能不能回滚,都有据可查。

### 3.2 身份映射(IdentityMap)

| 类型 | 源 | 目标 | 边界语义 |
|---|---|---|---|
| `module` | `git.enjoye.top/enjoydream/code-review-service` | `github.com/yi-nology/code-review-service` | 源标识后必须跟非模块路径字符或文本末尾 |
| `host` | `git.enjoye.top` | `github.com` | 源标识后必须跟非主机名字符或文本末尾(点算主机名字符,防半改写域名) |
| `literal` | `215` | *(剥离或替换)* | 前后都必须是词边界 |

**执行顺序固定为 `module → host → literal`**(同类保持声明顺序)。这不是可选约定:
若先执行 host 规则,`git.enjoye.top/enjoydream/x` 的前缀会先被改掉,module 规则随后找不到源标识,
**静默失效** —— 路径改写看起来成功了,实际上 import 全部指向不存在的模块。

**「剥离」与「替换」必须区分建模**:目标为空表示删除源标识本身,而不是替换成占位符。
占位符只是把内网痕迹换了个形式留在公开树里,反而更难发现。

### 3.3 计划模型(Plan)—— 主轴是文件,不是提交

```
Plan
  ├─ Files[]        逐文件处置(主轴),按路径升序
  ├─ Summary        统计
  └─ Commits[]      按源侧提交分组的回滚视图(辅助标注)
```

单个文件的处置(六类):

| 处置 | 含义 | 判定依据 |
|---|---|---|
| `same` | 两侧一致 | 改写 + gofmt 归一化后内容逐字节相同 |
| `port` | 以源侧内容覆盖 | 源侧为目标的**严格超集**(行级 diff 只有新增,没有删改) |
| `port_with_scrub` | 清理后移植 | 源侧新增且命中痕迹规则 |
| `exclude` | 不参与同步 | 命中路径排除规则 |
| `hold` | 待人工/AI 判定 | 两侧各有独有内容,或差异不在行级 |
| `preserve` | **受保护,不得覆盖** | 仅目标侧存在,或目标侧为超集 |

`preserve` 是 v1 缺失的一类(见 C4)。没有它,「目标侧领先的内容」在计划里无法表达,
单向同步会误伤 Gerrit 移植与 release CI 的成果。

每个非 `same` 处置必须附**可解释的理由**(哪条规则 / 哪个判定依据)与**来源提交标注**。

### 3.4 port 集合的隐式依赖(C6,实测)

**提交是自洽的,文件不是。**

引入某个文件的提交通常同时改动它依赖的文件;按文件粒度移植时,前者可能进 `port`,
后者因两边内容混合而停在 `hold` —— **单独移植前者会编译不过**。

实测(review-service):

| | 内容 |
|---|---|
| port 文件 | `internal/modules/server.go`(源侧 +4 行) |
| 缺失符号 | `readiness.LLMAvailabilityCheck` |
| 定义在 | `internal/readiness/checks.go` —— **hold 文件**(源侧 +129 / 目标侧 +6) |

**处理方式**:
1. **门禁是裁判** —— 类型门禁在「应用后的目标树」上跑,拦住这类移植(见 F4)。
2. **诊断要可执行** —— 门禁失败必须翻译成「要移植 A,得先处理 B」。
3. **计划阶段的显式建模(后续)** —— 当前靠门禁在推送前发现;
   后续应把「port 文件依赖哪些 hold 文件」在计划阶段就标出来,让审阅者提前看到。

**推论**:文件级主轴换来的「不被提交切分误导」的收益,代价是必须显式处理文件间依赖。
这个代价是可控的(门禁能查出来),但不能装作不存在。

### 3.5 hold 的自动解法:分层判定 + 三向合并(C7,实测)

v2 把 40 个 hold 全部交给人工裁决。实测发现**其中 31 个可以机械解决**。

**前提事实**:两棵树**没有 git 共同祖先**(`git merge-base` 为空)——
公开树不是内部树推上去的同一批 commit,而是**逐提交改写后的重做**:

| 证据 | 实测 |
|---|---|
| 公开树根提交 vs 内部树根提交 | subject 相同,文件数同为 **153** |
| 对应提交的文件内容 | 公开树 `74997f3:internal/readiness/checks.go` 与内部树 `77c5d48` 同文件**改写后逐字节相同**(md5 一致) |

⇒ 虽无 git 共同祖先,却有**内容级共同版本**可用于判定。

**判定分三层,从简单到复杂** —— 大多数文件在第一、二层就定论,
只有「两边都改过」的文件才需要找共同基点做真正的三向合并:

| 层 | 判据 | 解法 | 实测数量 |
|---|---|---|---|
| 一 | 源侧当前内容出现在**目标侧的历史版本**里 | 目标侧已在此基础上前进 → 保留目标侧 | **26** |
| 二 | 目标侧当前内容出现在**源侧的历史版本**里 | 源侧已在此基础上前进 → 取源侧 | **3** |
| 三 | 两边都前进过 | 找共同基点 → 三向合并:无冲突则自动合并,否则真冲突 | 自动合并 **2** / 真冲突 **5** |
| — | 两边都改过且无共同版本 | 无基点,需人工 | **4** |

⇒ **40 个 hold 里 31 个(77.5%)可机械解决**,需人工的收敛到 **9 个**
(5 真冲突 + 4 无基点),且这 9 个全是**配置/文档/部署/测试**文件,不是核心代码。

**端到端实测**:应用 31 个自动解法 + 3 个 port 文件后,物化出的目标树
**gofmt 0 违规、类型门禁通过** —— M2 拦下的那个依赖
(`internal/modules/server.go` 依赖 hold 的 `internal/readiness/checks.go`)
被「取源侧」解法一并解决。

**实现要点**:
- **版本集合从 diff 顺带取出**(blob hash),不必读内容;源侧只需把改写后的内容算成 hash。
- **历史遍历必须走全部父提交**:只走第一父会漏掉合并分支引入的中间版本,
  实测会多判 18 个冲突。
- 三向合并是**纯 Go 的 diff3 实现**(`mirror/history/merge.go`),不依赖外部 git 进程。

**为什么不用提交级原子组**:提交组确实自带依赖自洽
(实测 `d9bb3a3` 正好改了那 3 个文件),但它只是**移植单元**的选择,
解决不了「两边内容混合」的文件(源侧 +129 / 目标侧 +6 的 `checks.go` 仍需合并)。
内容级判定粒度更细、结论更准。

### 3.6 接口重做:通道优先(v2.1)

M3.5 的接口有**两套并行的同义接口**(传参版 + 通道版),这本身就是设计缺陷:
传参版绕过了通道配置,通道改了它不知道;而且传参版要求调用方知道**服务端的磁盘路径**。

重做后的接口只有一套:

| | 重做前 | 重做后 |
|---|---|---|
| 配置 | 每次调用传一整套规则 | **只在通道里**(DB 持久化) |
| 执行 | `POST /histsync/plan` + body | `POST /histsync/channels/:id/plan`,**无请求体** |
| 推送产物路径 | 客户端回传 `repo_path`(服务端的临时路径) | **执行记录里服务端自己记着** |
| 推送目标 | 通道里一个 JSON 数组 | **独立表**,凭据加密落库,顺序 = 推送优先级 |
| 审计 | 无 | **执行记录**:计划统计 / commit 映射 / 检测报告 / ref 对照 |
| 回滚 | 无 | **按执行记录回滚**到指定 ref(留底提供保障) |

**回滚语义**:目标是 **commit hash**(通常是某次同步前的 `base_head`),
不是分支名 —— 分支名指向的是同步**之后**的 HEAD。
实测:回滚到 `base_head` 后,发布目标回到同步前状态,**留底仍保留同步后的 HEAD**
(留底的意义:发布能回滚,新内容不会丢)。

### 3.7 裁决闭环:AI 建议 → 人工确认 → 写回 → 重算

「AI 只做语义判断」要落地,必须有**能把语义判断送回管道**的通路,否则 AI 只能给建议。

```
resolve → 冲突卷宗(dossiers)
   ↓  AI/人工判定:keep_target / take_source / merged + 理由
override-set → HistoryOverride(DB)
   ↓
resolve(重算) → 该文件的解法被裁决覆盖,summary.resolved 计数
   ↓
apply → 用裁决内容
```

**裁决只对 `hold` 生效**;其余文件仍走确定性判定。
**裁决改变实际发布内容** ⇒ MCP 工具侧标为危险操作(`set_history_override`),需用户确认。

**实测**:写回一条 `keep_target` 裁决后,重新 resolve:
`summary.resolved` 0 → 1,该文件解法 `conflict` → `take_ours`,
理由带上裁决理由与裁决者,待裁决数 9 → 8;
apply 后的新 HEAD **保留**了目标侧的 Gerrit 集成(未被源侧覆盖)。

**踩到的两个真坑**:

1. **GORM `default:true` 吞掉「显式禁用」** —— 零值 `false` 会被 GORM 省略,
   让数据库默认值 `true` 生效,于是「禁用通道」被悄悄改成「启用」。
   修法:model 不用 `default` 标签,DTO 用 `*bool` 区分「未指定」与「显式 false」。
2. **DTO 的 JSON tag 风格必须与同域既有 DTO 一致** —— camelCase vs snake_case 不一致时,
   字段会**静默忽略**:接口返回 200,但规则从未生效。
   修法:新增 DTO 前先 grep 同域既有 DTO;配置落库后加「经 DB 往返 == 直接传参」的等价性断言。

---

## 4. 功能需求

### F1 同步计划生成(Plan)

**输入**:内部树与公开树的本地克隆 + 身份映射 + 路径规则 + 痕迹规则。
**输出**:一份可审阅的同步计划。

**验收标准**:
- 计划生成是**纯读操作**:不写目标仓、不改工作区、不产生任何推送。
- 同一输入两次生成的计划**完全一致**(确定性)。
- 每个非 `same` 处置附带**触发原因**与**来源提交**。

> **已实现**:`mirror/history.BuildPlan`。实仓 10.4 秒,691 个路径。

### F2 差异判定(Delta)

**主轴是文件级内容差异**,不是提交级差异。

判定流程(顺序不可调换):

```
克隆两仓 → 路径规则(重命名/排除) → 身份改写 → gofmt 归一化 → 逐文件比对 → 方向判定
```

**方向判定**(行级 diff):

| 判定 | 条件 | 处置 |
|---|---|---|
| 源侧超集 | 只有新增,没有删改 | `port` |
| 目标侧超集 | 只有删除,没有新增 | `preserve` |
| 两侧各有独有 | 既有新增又有删改 | `hold` |
| 差异不在行级 | 字节不同但行级无差异(行尾符/结尾换行) | `hold` |

**验收标准**:
- 判定在 **gofmt 归一化之后**执行 —— 实测未归一化会产生 46 个假差异。
- `preserve` 与 `port` 互斥:同一文件不得同时被判为待覆盖与受保护。
- 来源提交标注用**单遍历史扫描**:逐文件 `repo.Log(FileName)` 是 O(文件数 × 历史长度),
  实测 691 文件 / 408 提交跑不出来(>600 秒),单遍扫描 10.4 秒。
- 行数超过 60000 的文件退化为 `hold`(行模式 diff 的 rune 上限),不做方向猜测。

> **已实现**:`mirror/history` 包,含 `classifyDiff` / `originCommitMap` 与单测。

### F3 身份改写(Rewrite)

复用 `core/mirror` 的改写原语,**单一实现**。

**验收标准**:
- 边界用例:`github.com/a/b/pkg` 正确改写;`github.com/a/b-extra`、`github.com/a/b.cool` **不被误改**。
- `biz/model/*` 与 `idl/*.thrift` 不被改动(实测零命中)。
- 改写后自动执行 **gofmt 归一化**(实测:改写后 50 个文件违反 gofmt,两棵树原始都是 0)。
- 8MB 以上 blob 与二进制文件原样保留。

> **已实现**:`mirror.Rewriter`(module/host/literal 三种边界语义 + 剥离语义),
> 既有 `rewriteModulePath` 已改为薄封装,`TestRewriteIdentitiesMatchesLegacyRegex`
> 以原始正则锁住行为不漂移。

### F4 类型门禁(TypeCheckGate)

复用 `core/mirror/gate.go` 的门禁(已导出为 `mirror.TypeCheckGate`,publish 模式与历史同步共用同一实现)。

**验收标准**:
- 门禁在**推送前**执行,不通过则中止且**不产生任何推送**。
- 门禁在 `GOWORK=off` 下运行。
- 错误信息定位到 `file:line:col`。
- **门禁必须跑在「按计划应用后的目标树」上**,不是源树 —— 源树本来就编译通过,
  门禁要回答的是「应用后的目标树还能不能编译」。
- **门禁失败必须翻译成「谁挡住了谁」**:解析 `undefined: pkg.Symbol`,
  在 hold 文件的源侧声明里反查符号归属,给出
  「要移植 A,得先处理 B」,而不是一句「编译失败」。

> 实测:门禁拦下了 `internal/modules/server.go`(port)对
> `internal/readiness/checks.go`(hold)的隐式依赖 —— 见 §3.3 的依赖闭包说明。

### F5 痕迹检测与清理(Scrub)

对改写归一化后的待推送内容做检测。

**默认识别目标**(可配置):

| 类别 | 模式示例 |
|---|---|
| 内网域名 | `*.enjoye.top` |
| 密钥形态 | `Bearer `、`sk-`、`ghp_`、`api_key=` |
| 内网地址 | `10.x` / `100.64.x` / `192.168.x` 段 |

**验收标准**:
- 检测在**推送前**完成,产出**逐行定位**的报告(文件:行号:命中内容)。
- 命中密钥类 → **强制 hold**,不自动替换。
- 命中内网域名类 → 可按规则自动替换或剔除,但**必须在计划里列出**。

> 实测:内网域名只残留在 3 个文件(AGENTS.md / Dockerfile / deploy/Dockerfile.frontend),
> 共 10 处,**业务代码零残留**。检测规则可收窄,不必铺开。

### F6 内容移植(Apply)

把计划落到目标树。

**合并策略**(v1 缺失,C5):

```
目标树既有提交(hash 全部保留,不重写)
        +
移植内容:以源侧内容覆盖/新增的文件
        +
目标树独有的文件(原样保留,受保护)
```

移植**不重写目标树的既有提交**(不做 rebase、不改父提交),
因此目标树上的 tag、分支、PR 引用全部保持有效。

**验收标准**:
- 目标树既有提交的 hash **不变**。
- `preserve` 清单里的文件**逐字节不变**。
- 移植后重算新 hash(内容改写必然改 hash),新 commit message 保留原 subject 并追加改写说明。
- 冲突时不自动解,进入 `hold` 并产出冲突报告。

### F7 双远端推送(Push)

支持目标侧多远端:GitHub 真上游 + 本地裸镜像留底。

**验收标准**:
- 推送顺序可配置,默认**先留底后发布**。
- 任一远端失败时**不推送其余远端**。
- 推送使用 `gitbackend`,不执行动态参数子进程。
- 推送前**必须**通过 F4 门禁与 F5 检测。

### F8 预检与确认(DryRun / Confirm)

预检 = **把这次同步的全部影响面摊开,但不推送**。它与 plan/resolve 的差别在于
多回答了三个问题:**推出去能不能编译**、**将产生哪些提交**、**推送会不会覆盖别人的提交**。

**报告内容**:

| 项 | 说明 |
|---|---|
| 计划统计 | same / port / hold(已裁决数) / preserve / 痕迹命中 |
| 门禁 | 物化结果的 gofmt 违规 + 全量类型检查 —— **推出去能不能编译** |
| 将产生的提交 | 源提交 → 目标提交、subject、文件数 |
| 推送影响面 | 每个远端:当前 HEAD、快进/分叉/未知、**要不要强推** |
| 待裁决冲突 | 还没拍板的 hold 文件 |
| 警告 | 需强推、门禁未过、还有冲突未裁 |

**验收标准**:
- 预检**不推送、不改原始目标仓**(物化与门禁都在临时工作副本上做)✅
- 预检结论**与真正执行一致** —— 两者跑同一套代码路径 ✅
  (单测 `TestHistoryPreview` 断言:预检的 `new_head` == 真正 apply 后的 HEAD)
- 预检产出**完整差异报告**:将移植的文件数、清理命中数、待人工确认数、受保护文件数 ✅
- 危险操作(强推、覆盖已有 tag)需显式确认,**确认前展示影响面** ✅
  (`needs_force` + 每个远端的影响面说明)

### F9 审计与回滚(Audit / Rollback)

**两条链,职责分明**:

| 链 | 载体 | 范围 | 回答的问题 |
|---|---|---|---|
| **既有审计链** | `OperationLog`(全局) | 全系统写操作 | **谁**在什么时候对**什么**做了什么 |
| **通道执行链** | `HistoryRun.PrevHash/EntryHash` | 单个通道内 | 这批同步的**审计材料**有没有被改 |

两者同构(`EntryHash = SHA256(PrevHash || payload)`),但**不混在一起**:
不同通道的同步记录混在一条链里没有意义,而且链会随通道数无限增长。

**通道执行链的哈希覆盖全部审计材料**:基线 HEAD、新 HEAD、计划统计、commit 映射、
应用/未应用文件、痕迹检测报告、门禁结果、推送前后 ref 对照。
⇒ 篡改其中任何一项,校验立刻报 `entry_hash mismatch (tampered?)` 并指出断链的记录 ID。

**只在终态计算哈希**:执行中的记录字段还在变,提前算出的哈希必然失配。
续链取该通道最近一条**已入链**的记录 —— 执行中的记录没有哈希,必须跳过。

**一次动作一条记录(演练实弹出来的规则)**:链上的记录一旦入链,**任何字段都不能再改** ——
改了哈希就与入链时算的不符,链立刻报「被篡改」(实测:push 往 apply 记录里写
`RefsAfter`,通道链立刻 `entry_hash mismatch`,而其实没人篡改)。
⇒ `apply` / `push` / `rollback` 各写各的记录:`push` 记录引用被推的 apply 记录,
**不修改**它。这样链只增不改,永不误报。

**验收标准**:
- 每次同步留存:计划快照、门禁结果、检测报告、推送前后 ref 对照、commit 映射表 ✓(在 `HistoryRun` 上)
- 支持按同步记录**回滚目标树到指定 ref**(裸镜像留底提供保障) ✓
- 审计记录含**哈希链**,与 git-ferry 既有审计机制一致 ✓
  (既有 `OperationLog` 链 + 通道内 `HistoryRun` 链,同构不同域)
- 通道执行链的校验接口可单独调用(`/histsync/channels/:id/audit`)✓
- 通道执行链的校验**与全局审计链互不影响**(实测:篡改 run 记录时,
  全局链仍 `intact`,通道链报断链)✓

### F9.5 冲突自动判据(Conflict Rules)

有些「冲突」其实是**机械可判**的,不该浪费一次人工裁决。判据只在
**确定性判定解不了时兜底**(合并干净时优先用合并结果 —— 否则会把
「能干净合并」的文件降级成「保留目标侧」,丢掉源侧的非冲突改动)。

| 判据 | 命中条件 | 结论 |
|---|---|---|
| `internal_trace` | 源侧独有行含**内网形态**(内网 registry / 内网域名 / 构建期跳过证书校验 / 内网措辞) | 保留目标侧 |
| `public_asset` | 目标侧独有行含**公开资产**(license 副本 / badge / CI 徽章 / 公共镜像源 / 多平台集成)且源侧没有 | 保留目标侧(源侧是旧版) |
| `placeholder_domain` | 源侧用**真实对外域名**、目标侧已**脱敏成示例域名** | 保留目标侧 |

**实测**:review-service 的 9 个真冲突,开启判据后 **8 个被自动判定**,
只剩 `internal/service/provider/service.go` 需人工 —— 那是「两版功能都全、
只是新旧不同」的真语义判断,本来就该留给人。

判据是**通道级可配**的(可开关、可自定义正则),默认不启用。

### F10 Agent 接入(CLI / MCP / Skills)

**前提修正**:v1 写「沿用 git-ferry 既有接入面,不另造」——**不成立**。
镜像中心模式目前**只有 HTTP 路由**(`biz/router/git_sync/git_sync.go:67-75`),
**无 CLI 命令组、无 MCP 工具**(`internal/cli/` 与 `internal/agent/` 搜 mirror 零命中),
只有 `gitferry api` 逃生舱可透传。
⇒ 历史同步会是这套能力的**第一个** CLI/MCP 接入面,工作量需计入。

**命名**:命令行/API 用 **`histsync`** 而非 `history` ——
`history` 已被「同步执行历史」占用(`internal/cli/commands/domains.go` 的 `newHistoryCmd`)。

**已交付(M3.5 + v2.1 重做)**:

| 接入面 | 形态 |
|---|---|
| core 服务层 | `service/history_service.go` + `service/history_channel_service.go`:通道 CRUD、目标 CRUD、执行记录、回滚;DTO 与内部类型解耦(规则种类用字符串、痕迹模式用正则源码) |
| HTTP | `/api/v1/histsync/channels/...`(含 `targets`、`runs`)+ `/api/v1/histsync/runs/:id/rollback` |
| CLI | `gitferry histsync` 共 22 个子命令:`+channel-{list,create,update,delete}` / `+target-{list,create,delete}` / `+override-{list,set,delete}` / `+plan` / `+resolve` / `+apply` / `+push` / `+runs` / `+run-detail` / `+rollback` / `+channel-audit` / `+preview` / `+sync` / `+target-rotate` |
| Agent Skill | `skills/gitferry-histsync/` —— 标准流程、**AI 职责边界**、冲突裁决手册(词表 + 真实实例) |
| MCP 工具 | `list_history_channels` / `plan_history_channel` / `resolve_history_channel` / `list_history_runs` / `verify_history_channel_audit` / `list_history_overrides` / `set_history_override` / `preview_history_channel` / `sync_history_channel` / `rotate_history_target_credential`(共 41 个工具) |

**推送不在 MCP 工具面内** —— 推送要远端 URL 与凭据,是对外可见动作,
留在 CLI 与人工确认路径上。通道的建/删同样留在 CLI。

**执行与推送在 MCP 工具面内** —— apply 会产生本地提交(不推送),属于可审阅的中间态;
真正的推送必须走 CLI。

**通道是 DB 持久化的**(见 §3.1):`gitferry histsync +channel-create` 存好配置后,
后续按通道执行不必再传一整套规则。

**待交付(M5)**:凭据的到期提醒策略、判据的自定义正则 UI、通道的批量操作。

**两种输出格式分别对待**:CLI 走 `internal/cli/output.Envelope{ok,data,error,meta}`;
MCP 走 `internal/agent/tools` 注册表与服务端 `{code,message,data,timestamp}`。
v1 写「MCP 输出走既有 Envelope 格式」不准确,Skill 手册要分面写。

---

## 5. AI 辅助的定位(边界必须写死)

**AI 只做语义判断,不做机械变换。**

| 环节 | 执行者 | 理由 |
|---|---|---|
| 路径改写 | 确定性代码 | 文本变换,LLM 不可靠且不可复现 |
| gofmt 归一化 | 确定性代码 | `go/format` |
| 差异判定 | 确定性代码(行级 diff) | 方向判定必须是确定的 |
| 类型门禁 | 确定性代码(go/packages) | 语义级检查,LLM 无法替代 |
| 痕迹检测 | 规则优先 + AI 补充 | 正则能抓的确定性抓,AI 补语义型 |
| **改动能否公开** | **AI** | 需理解 diff 意图,规则做不到 |
| **痕迹要不要脱敏** | **AI** | 需判断上下文语义 |
| **hold 怎么解** | **AI 辅助 + 人工确认** | 40 个 hold 的判定 |
| **commit message** | AI 生成 + 人工确认 | 既有能力,直接复用 |

**AI 的输入是文件级差异,不是提交列表** —— 给 AI 看 18 个误判提交没有意义,
给它看 40 个 hold 文件的具体 diff 才有意义。

**AI 判定的输出必须可解释**:每条结论要给出理由与依据的 diff 片段。

**AI 不参与**:路径改写结果、gofmt 结果、差异判定结论、检测命中项。

---

## 6. 验收标准(端到端)

以 `review-service` 为首个验证对象:

| # | 验收项 | 判定方式 | 基线值 |
|---|---|---|---|
| A1 | 计划覆盖两树全部路径并正确分类 | 计划统计与基线一致 | 691 路径 / 3 port / 40 hold / 39 preserve |
| A2 | 引用模块路径的文件全部正确改写,零误伤 | 兄弟路径不被改 | 262 文件 / 98 目录 / 688 处引用 |
| A3 | 改写后 gofmt 违规为 0 | `gofmt -l` | 改写后 50 → 归一化后 0 |
| A4 | 类型门禁通过(或拦住并定位到具体 hold 文件) | 门禁日志 | 当前**拦住**:`internal/modules/server.go` 依赖 hold 文件 `internal/readiness/checks.go` |
| A5 | 公开树无 `*.enjoye.top` 残留 | 扫描报告 | 10 处 / 3 文件,自动清理后不留痕 |
| A6 | 目标树独有提交原样保留 | commit 映射表 | ✅ 406 个提交 hash 全部不变(旧 HEAD 仍是新 HEAD 的祖先) |
| A7 | 目录改名 `docs/internal/` → `docs/archive/` 生效 | 文件树比对 | 11 个文件由路径规则对齐 |
| A8 | 预检不产生任何推送 | 远端 ref 对照无变化 | ✅ plan/resolve/apply 全程不推送 |
| A9 | 同一输入两次生成的计划完全一致 | 计划 diff 为空 | ✅ 已有单测;两次移植的 HEAD 也一致 |
| A10 | 公开树 `go.mod` 的 module 行为 `github.com/yi-nology/code-review-service` | 文件核对 | ✅ |
| A11 | **受保护文件逐字节不变** | 移植前后哈希对照 | ✅ 39 个文件全部保留 |

---

## 7. 待决策项(评审时拍板)

### D1 程序形态:扩展 git-ferry 还是独立程序?

**推荐**:扩展 git-ferry。改写边界正则这类坑不值得踩两遍。
**成本表修正**:MCP/CLI **不是白拿** —— 镜像中心模式没有既有 CLI/MCP 接入面(见 F10)。

### D2 AI 介入深度?

**推荐**:语义判断层(§5)。机械变换层交给确定性代码。
**补充**:AI 的输入从「提交」改为「文件级差异」。

### D3 推广范围?

**推荐**:首期只做 `review-service` 跑通。推广是后续独立事项。
本地共 10 个内部路径 go 仓(`code-review-service` / `bianque` / `crawler` / `domain-manage` /
`moyuoctopus` / `openclash-service` / `user-service` / `wechat` / `review-service-export`)。

### D4 公开树独有提交如何回流?

**不回流**,本需求定义为单向。将来需要双向应另立需求。

### D5 【新增】M1 主轴确认为文件级内容差异

**推荐**:采纳。理由见 §1.2 与基线文档。
**代价**:提交级视图降级为辅助标注(`Commits[]` 回滚视图),不再作为判定依据。

### D6 【新增】core 侧改写原语的导出落点

**推荐**:在 `core/mirror` 导出多规则改写原语(`Rewriter` / `IdentityRule`),
单仓身份镜像与历史同步共用;**不要在 service 包重写正则**。

> 现状:`mirror` 包导出面只有 `Publish` / `ReadModulePath`,`rewriteModulePath` /
> `rewriteTree` / `builder` 全是未导出标识符,v1 的「复用且不做二次实现」在代码层不成立。
> 本次已落地:`mirror.Rewriter` 为唯一实现,旧 `rewriteModulePath` 改为薄封装。

### D7 【新增】合并策略与保护集

**推荐**:采纳 §3.3 与 F6 的定义 ——
内容移植叠加在目标树之上,**不重写目标树既有提交**(hash 全部保留);
`preserve` 清单(实测 39 个文件)是受保护集,移植时逐字节不变。

**子项(已拍板,并经 M3 实测修正)**:40 个 `hold` 的处置粒度 ——
**先机械解,剩下的人工裁决**;人工部分采用 **AI 批量出建议 + 人工整体确认**。

M3 实测后修正为两段式:

| 阶段 | 手段 | 结果 |
|---|---|---|
| 第一段 | **确定性分层判定 + 三向合并**(见 §3.5) | 40 个 hold 里 **31 个机械解决**(26 保留目标侧 / 3 取源侧 / 2 自动合并) |
| 第二段 | **AI 批量建议 + 人工整体确认** | 剩下 **9 个**(5 真冲突 / 4 无基点),AI 给「判定 + 依据 diff 片段」,人工批量确认(默认接受、逐个否决) |

依据:
1. 40 个 `hold` **混着三类**,不是同一类问题:
   - 源侧明显领先(如 `internal/readiness/checks.go` 源侧 +129 / 目标侧 +6)→ 取源侧
   - 目标侧明显领先(如 `internal/config/loader.go` 源侧 +13 / 目标侧 +47)→ 保留目标侧
   - 纯措辞差异 → 需判断
2. 逐文件人工裁决 = 40 次决策,成本高且后段易疲劳漏看。
3. **但先做机械解是有先例的**:D7 最初拍的是「全部交 AI」,
   实测发现 77.5% 的 hold 根本不需要语义判断 —— 三向合并是确定性的,没有理由让 LLM 去猜。
   AI 只该花在真冲突上(9 个)。

**这条决定直接影响 M3 的形态**:AI 建议器的输入是 **9 个** 文件的
「改写后源内容 vs 目标侧内容」diff,输出是逐文件的建议判定 + 依据。

---

## 8. 里程碑

| 阶段 | 内容 | 出口条件 | 状态 |
|---|---|---|---|
| **M1** | core 扩展:改写原语 + 差异判定 + 计划生成(F1/F2/F3) | 对 review-service 产出正确计划 | ✅ 已完成,基线已固化 |
| **M2** | 物化 + 格式校验 + 类型门禁(F4) | A2/A3 通过;A4 能拦下隐式依赖并定位到具体 hold 文件 | ✅ 已完成 |
| **M2.5** | hold 自动解法:分层判定 + 三向合并(§3.5) | 40 个 hold 里 31 个机械解决;应用后门禁通过 | ✅ 已完成 |
| **M3** | 痕迹清理 + 内容移植 + 双远端推送(F5/F6/F7) | A1/A5/A6/A7/A11 通过;**推出去的结果能编译** | ✅ 已完成(本地裸镜像验证) |
| **M3.5** | 接入面:core 服务层 + HTTP + CLI + MCP 工具(F10 前半) | CLI/HTTP/MCP 都能拿到计划与冲突卷宗 | ✅ 已完成 |
| **M3.6** | 通道 DB 化:模型 + DAO + 迁移 + 通道 CRUD/执行 | 按通道出的计划与直接传参完全一致 | ✅ 已完成 |
| **M3.7** | **接口重做:通道优先**(配置只在通道、执行按 id、产物路径服务端自记、推送目标加密成表、执行记录 + 回滚) | 通道优先流程端到端跑通(建通道→目标→plan→apply→push→回滚) | ✅ 已完成 |
| **M3.8** | **执行记录接审计哈希链**:通道内 `HistoryRun` 链 + 关键动作进既有 `OperationLog` 链 | 篡改任一审计材料能被检出;两条链互不影响 | ✅ 已完成 |
| **M3.9** | **裁决闭环 + Agent Skill 手册**:`HistoryOverride` 表 + `override-set` 写回 + `gitferry-histsync` 技能 | 裁决写回后重算生效;技能手册成文 | ✅ 已完成 |
| **M4** | **预检(F8)**:门禁 + 提交预演 + 推送影响面 | 预检结论与真正执行一致(HEAD 相同) | ✅ 已完成 |
| **M5** | 通道级冲突判据 + 凭据轮换 + 预检/推送一条命令 | 8/9 冲突自动判定;凭据只换密钥;+sync 端到端跑通 | ✅ 已完成 |

**M1 已交付**:
- `git-ferry-core/mirror/rewrite.go` —— 多规则身份改写原语(含三种边界语义 + 剥离语义)
- `git-ferry-core/mirror/history/` —— 差异判定 + 计划生成 + Markdown 报告 + 实仓 live test
- `git-ferry/docs/superpowers/specs/2026-10-23-history-sync-baseline.md` —— 实测基线(可复现 + 回归门禁)

**M2 已交付**:
- `mirror.TypeCheckGate` —— 门禁导出为共用实现(publish 模式改调同一份)
- `mirror.MaterializeTree` —— 树物化导出(并修掉「不创建根目录」的缺陷)
- `mirror/history/materialize.go` —— 物化「预期目标树」+ 发布前校验(gofmt + 类型门禁)
- `mirror/history/blocked.go` —— 把门禁失败翻译成「谁挡住了谁」

**M2 实测结论**:A2(改写彻底 + 零误伤)✅、A3(gofmt 干净)✅、
A4 **不通过且这是正确的** —— port 集合存在隐式依赖,已定位到具体 hold 文件。

---

## 9. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 目标树与内部树历史分叉加深,内容差异扩大 | 计划噪声上升 | 差异判定是内容级,不受提交切分影响;hold 逐条带 diff |
| 痕迹清理误伤 | 内容损坏 | 检测与清理分离;清理命中项全部进计划供审 |
| 覆盖目标树已有内容 | 不可逆 | `preserve` 受保护集(实测 39)+ 裸镜像留底 + F9 回滚 |
| `core/mirror` 若演进,历史同步被绑定 | 维护成本 | 依赖 core 的**导出 API**(`Rewriter`),不依赖内部实现 |
| 依赖 `go-git-platform` / `oekit` 家族版本 | 构建失败 | 走真实发布版本,不引 `replace` |
| 行级 diff 在超大文件上退化 | 方向误判 | 超过 60000 行退化为 `hold`,不做方向猜测 |

---

## 10. 参考

- `git-ferry-core/mirror/rewrite.go` —— 多规则身份改写原语(唯一实现)
- `git-ferry-core/mirror/history/` —— 差异判定 + 计划生成(M1 实现)
- `git-ferry-core/mirror/gitobj.go` —— 改写调用点(publish 模式)
- `git-ferry-core/mirror/gate.go` —— 类型检查门禁
- `git-ferry-core/service/mirror_service.go` —— 身份镜像模式编排
- `docs/superpowers/specs/2026-10-23-history-sync-baseline.md` —— **实测基线(本需求的数字来源)**
- `docs/superpowers/specs/2026-09-16-mirror-publishing-design.md` —— 镜像中心前端设计(本需求不改其页面结构)
