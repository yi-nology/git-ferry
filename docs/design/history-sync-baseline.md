# 历史同步 M1 实测基线

> 2026-10-23 · 状态:基线已固化,可作为回归门禁
> 产出方式:`git-ferry-core/mirror/history` 的实现跑实仓,非人工统计
> 关联:`history-sync-design.md`(设计稿 v2 的事实依据)

---

## 0. 一句话结论

**「内部树 → 公开树」的真实待办量是 3 个文件直接移植 + 40 个文件需人工判定 + 39 个文件受保护不可动**,
不是设计稿原先估计的「18 个提交待移植」。

原设计稿的核心机制(按 `subject+author+date` 匹配提交)在真实数据上 **18/18 失败**;
本基线用实现替换了它,并把待办量从「18 个提交」压缩到「43 个文件」。

---

## 1. 如何复现

```bash
cd git-ferry-core

HISTORY_LIVE_SRC=/Users/zhangyi/my_project/review-service \
HISTORY_LIVE_TGT=/Users/zhangyi/my_project/code-review-service \
HISTORY_LIVE_REPORT=/tmp/history-baseline.md \
HISTORY_LIVE_EXPECT="port=3,port_with_scrub=0,hold=38,exclude=0,preserve=39,same=611" \
go test -tags history_live -run TestLiveHistoryPlan -v ./mirror/history/
```

`HISTORY_LIVE_EXPECT` 是回归门禁:基线漂移即测试失败,需复核后显式更新。
实仓全量跑 **10.4 秒**。

---

## 2. 基线统计

| 处置 | 含义 | 数量 |
|---|---|---|
| `same` | 经身份改写 + gofmt 归一化后两侧一致 | **611** |
| `hold` | 两侧各有独有内容,需人工/AI 判定 | **38** |
| `preserve` | 受保护,移植时不得覆盖 | **39** |
| `port` | 源侧为超集,以源侧内容覆盖 | **3** |
| `port_with_scrub` | 需清理痕迹后移植 | 0 |
| `exclude` | 命中排除规则 | 0 |

- 参与比对的路径总数:**691**(源侧 ∪ 目标侧)
- 相对目标侧:**+1007 / -953** 行
- 痕迹命中:**10 处**,集中在 3 个文件

> **基线随实仓演进而变**(每次漂移都要复核,不能直接改数字):
> - `hold` 40 → 38、`same` 609 → 611:两侧都提交了同一批 oekit 依赖升级
>   (`go.mod` / `go.sum` 两边一致了 ⇒ 从 hold 变成 same)。
> - 「仍待人工」11 → 9:少了 `AGENTS.md` 与 `internal/service/provider/service.go`。
>   前者是**改写修复的收益**:源侧那行内网克隆 URL
>   (`ssh://git.enjoye.top/enjoydream/code-review-service.git`)以前改不掉
>   (`.git` 被当成 module path 的延续字符)⇒ 与目标侧不一致 ⇒ 假冲突;
>   改写修好后两边一致 ⇒ 自动合并。后者是上面的依赖升级带来的。

### 2.1 需要移植(3)

| 路径 | 增/删 | 来源提交 |
|---|---|---|
| `swagger/openapi.yaml` | +493 / -0 | `4893cfb7` fix: swagger 补 9 个 Argus 接口 |
| `internal/modules/server.go` | +4 / -0 | `d9bb3a37` feat(readiness): llm-usable 可用性探测 |
| `internal/readiness/checks_test.go` | 源侧新增 | `d9bb3a37` feat(readiness): llm-usable 可用性探测 |

这 3 个文件正好对应内部树那 4 个「readiness / swagger」提交的真实内容增量 ——
**提交级匹配误判出的 18 个提交里,真正有内容增量的是 3 个文件。**

### 2.2 受保护(39)

| 类别 | 数量 | 内容 |
|---|---|---|
| 仅目标侧存在(公开树独有资产) | **27** | 15 张 `docs/assets/screenshots/*.png`、`docs/assets/logo.svg`、`CODE_OF_CONDUCT.md`、`docs/FUNCTIONAL_OVERVIEW.md`、`internal/service/gerrit/*`、`internal/service/provider/commitstatus*`、`scripts/seed-demo.sql` 等 |
| 目标侧内容领先(不可覆盖) | **12** | `CHANGELOG.md`(+46)、`internal/ocr/runner_test.go`(+88)、`internal/service/provider/service.go`(+107)、`internal/service/gerrit/client.go`(+39)、`.github/workflows/build.yml`(+13)、`CONTRIBUTING.md`(+8)、`internal/domain/platform/platform.go`(+8)、`internal/strutil/strutil.go`(+6)、`frontend/src/views/GitPlatformSettings.vue`(+6)、`CONTRIBUTING.zh-CN.md`(+4)、`SECURITY.md`(+3)、`.gitignore`(+3) |

后者是 Gerrit 移植与 release CI 的成果。单向同步**不得覆盖**它们。

### 2.3 痕迹命中(10 处 / 3 文件)

| 文件 | 命中 |
|---|---|
| `AGENTS.md` | internal-host×1、internal-registry×1 |
| `Dockerfile` | internal-host×2、internal-registry×2 |
| `deploy/Dockerfile.frontend` | internal-host×2、internal-registry×2 |

规模远小于设计稿的设想:内网域名只残留在部署与文档文件里,**业务代码零残留**。
检测规则因此可以收窄到 `*.enjoye.top` 一类,不必铺开。

---

## 3. 实现过程中被实测推翻/修正的三处

### 3.1 差异判定的主轴必须是「内容差异」,不是「提交差异」

设计稿 F2 用 `(subject, author, authorDate)` 匹配两侧历史。实测:

- 三元组全量匹配:共同 390、仅内部 **18**、仅公开 **16**(设计稿写的是「仅内部 2 / 仅公开 3」)
- 两棵树是「**同一工作、不同提交切分/措辞**」,不是「同一提交序列」:

| 内部 | 公开 | 差异 |
|---|---|---|
| `f13f2d68` docs: 补「预编译二进制」**构建**说明 | `583d4efc` docs: 补「预编译二进制」**发布**说明 | 同日同人同工作,subject 差一个字 |
| `86f9ba9e` fix(ci): 格式检查步骤报出违规文件名 | `55fc7cf6` fix(ci): 修正 worker.go 的 import 排序 **+** 检查步骤… | 公开侧把两个改动合成一个提交 |
| `336ab6ea` fix(security): OCR 子进程不再继承…凭据 | `d879e772` fix(security): …**+ 修正 import 排序** | 同上 |

把判为「内部独有」的 18 个提交逐个 cherry-pick 到公开树 HEAD:
**成功 0 / 空提交 0 / 冲突 18** —— 因为这些改动内容早已在公开树里,只是提交形态不同。

⇒ 提交级匹配会把大量**已发布的同源工作**误判为待移植,产出重复移植与大面积冲突。
⇒ 内容差异不受切分方式影响,是唯一可靠的轴。

### 3.2 差异判定必须跑在 gofmt 归一化之后

目标侧有 gofmt 门禁(公开树 `.github/workflows/quality.yml:58` 有格式检查步骤),
而身份改写会改变 import 的字典序位置:

| 状态 | gofmt 违规文件数 |
|---|---|
| 内部树原始 | 0 |
| 公开树原始 | 0 |
| **内部树经身份改写后** | **50** |

不归一化就比对,每个引用了模块路径的文件都留下一处 1 行的假差异。
实测:归一化前 `hold=86`,归一化后 **`hold=40`**(该次实测的实仓状态)——
**46 个假差异被消除**。当前实仓状态为 `hold=38`(见 §2 的漂移说明)。

> 设计稿 F3 已要求「改写后自动执行 gofmt」,但 F2(差异判定)未提及,顺序未定义。
> 正确顺序:**改写 → gofmt 归一化 → 差异判定**。

### 3.3 来源提交标注必须单遍扫描历史

逐文件调 `repo.Log(FileName)` 是 O(文件数 × 历史长度) 次树遍历:
**691 个文件在 408 个提交上跑不出来(>600 秒未完成)**。

改为单遍回溯(从 HEAD 往回,merkletrie 差分,首个触及某路径的提交即其来源):
**10.4 秒**。

> 附带修掉一个真实缺陷:回溯到根提交时无条件覆盖,导致所有「根提交就存在」的文件
> 都被标成根提交(如 `swagger/openapi.yaml` 曾被标成 `init:`,实际来自 `4893cfb7`)。
> 已有单测 `TestOriginCommitMap` 锁住。

---

## 4. 沿用既有能力的情况(实测核对)

| 设计稿声称 | 实测 |
|---|---|
| `mirror/gitobj.go:23` 边界安全改写 | ✅ 一致;本次已收编为 `mirror.Rewriter`(唯一实现),旧 `rewriteModulePath` 改为薄封装 |
| `mirror/gate.go:18` 类型门禁(`GOWORK=off`) | ✅ 一致;错误信息含 `file:line:col` |
| 改写后自动 gofmt | ✅ 本次实现为 `history.normalizeSource`(`go/format.Source`) |
| `biz/model/*` 与 `idl/*.thrift` 不含模块路径 | ✅ 零命中,无需重新生成 |
| 8MB 以上 blob 与二进制原样保留 | ✅ 沿用 `maxRewriteSize` 与二进制判定 |
| 审计哈希链 | ✅ `model/operation_log.go` 有 `PrevHash` + `VerifyAuditChain` |
| 「沿用既有 CLI/MCP 接入面」 | ❌ mirror 只有 HTTP 路由(`biz/router/git_sync/git_sync.go:67-75`),**无 CLI 命令组、无 MCP 工具** |

补充实测:引用模块路径的 `.go` 文件是 **262 个**(设计稿写「30+」,低估 8 倍),
分布在 16 个目录;唯一 import 目录 98 个、引用总数 688 处 —— 这两项设计稿准确。

---

## 5. M2 实测:门禁拦住了 port 集合的隐式依赖

M2 把计划落到「预期目标树」并跑发布前校验。**门禁正确地拦下了一次会编译不过的移植**。

```
物化: 文件 691,应用 3,跳过(待清理) 0,待确认 40,受保护 39
类型检查失败: .../expected-target/internal/modules/server.go:103:67: undefined: readiness.LLMAvailabilityCheck
被挡住的 port 文件: internal/modules/server.go 缺符号 readiness.LLMAvailabilityCheck → 定义在 hold 文件 internal/readiness/checks.go
```

### 5.1 发现:文件级主轴引入了隐式依赖

**提交是自洽的,文件不是。**
引入 `internal/modules/server.go` 的提交同时改动了 `internal/readiness/checks.go`;
但按文件粒度移植时,前者进了 `port`,后者因两边内容混合(源侧 +129 / 目标侧 +6)
停在 `hold` —— **单独移植 server.go 编译不过**。

| | 内容 |
|---|---|
| port 文件 | `internal/modules/server.go`(源侧 +4 行) |
| 缺失符号 | `readiness.LLMAvailabilityCheck`(server.go:103 引用) |
| 定义在 | `internal/readiness/checks.go` —— **hold 文件**(源侧 +129 / 目标侧 +6) |
| 目标树现状 | 该函数不存在(只有 `DBCheck` / `OCRBinaryCheck` / `LLMEndpointCheck`) |

### 5.2 门禁必须跑在「应用后的目标树」上

跑在源树上没有意义 —— 源树本来就编译通过。门禁要回答的是
「**按计划应用后的目标树还能不能编译**」。

因此 M1 的差异判定、M2 的格式校验、M2 的编译门禁,三者必须针对**同一份产物**:
三者对不上,说明管道里有环节没串起来。

### 5.3 门禁失败必须翻译成「谁挡住了谁」

只给一句「编译失败」没有用。门禁知道目标侧缺哪个符号,只有源侧知道这个符号由谁定义,
把两者接起来,审阅者拿到的就是可执行的一步:

> 要移植 `internal/modules/server.go`,得先处理 `internal/readiness/checks.go`。

实现:`history.findBlockedPorts` —— 解析门禁报错的 `undefined: pkg.Symbol`,
在 hold 文件的源侧声明里反查符号归属。

### 5.4 M2 验收结果

| 验收项 | 结果 |
|---|---|
| **A2** 改写彻底 + 零误伤 | ✅ 物化树中源 module path 出现 **0** 次;兄弟路径(`github.com/yi-nology/go-git-platform` 等)逐字未变 |
| **A3** 格式门禁 | ✅ 物化结果 **gofmt 违规 0** |
| **A4** 编译门禁 | ⚠️ **不通过,且这是正确的** —— port 集合存在隐式依赖,已定位到具体 hold 文件 |

---

## 6. M2.5 实测:hold 的自动解法

M2 拦下移植后,M2.5 把「hold 全部交人工」改成「**先机械解,剩下的人工裁决**」。

### 6.1 前提:两树没有 git 共同祖先,但有内容级共同版本

`git merge-base` 为空 —— 公开树不是内部树推上去的同一批 commit,
而是**逐提交改写后的重做**:

| 证据 | 实测 |
|---|---|
| 公开树根提交 vs 内部树根提交 | subject 相同,文件数同为 **153** |
| 对应提交的文件内容 | 公开树 `74997f3:internal/readiness/checks.go` 与内部树 `77c5d48` 同文件**改写后逐字节相同**(md5 一致) |

### 6.2 分层判定

| 层 | 判据 | 解法 | 数量 |
|---|---|---|---|
| 一 | 源侧当前内容出现在**目标侧的历史版本**里 | 保留目标侧 | **26** |
| 二 | 目标侧当前内容出现在**源侧的历史版本**里 | 取源侧 | **3** |
| 三 | 两边都前进过 | 找共同基点 → 三向合并 | 自动合并 **2** / 真冲突 **5** |
| — | 两边都改过且无共同版本 | 无基点,需人工 | **4** |

⇒ **31/40(77.5%)机械解决**,需人工收敛到 **9 个**,且全是配置/文档/部署/测试文件。

> ⚠️ **本节数字已被 2026-10-23 的合并算法修复推翻:9 → 11。**
>
> 原来的 9 个是**漏判**的结果。三向合并的区域划分把
> 「插入点落在对方改动边界上」当成了不重叠,于是
> `etc/config-dev.yaml` 与 `etc/config-prod.yaml` 被判成「合并干净」——
> 实际合并出来的是**源侧的真实域名**(`code-review.me`)覆盖了目标侧
> 已脱敏的 `review.example.com`,而且那一行注释**重复了两次**。
>
> 修复后(区域按闭区间相交分组 + 与 `git merge-file --diff3` 逐字对拍校准):
> 待人工 **11** 个 = 真冲突 **7** + 无基点 **4**;
> `etc/config-dev.yaml`、`etc/config-prod.yaml` 归入真冲突。
> 已用 `git merge-file --diff3` 单独验证:git 也判这两个文件是冲突,
> 修复后的实现输出与 git **逐字一致**。
> **回归门禁分两层**:
> - `mirror/history/merge3_fuzz_test.go` —— 随机对拍(默认 500 组,`go test ./...` 就会跑):
>   断言「合并结果不丢任何一侧新增的行」。**这一层是 CI 门禁**。
> - `mirror/history/live_test.go` —— 实仓门禁,需要 `HISTORY_LIVE_SRC` / `HISTORY_LIVE_TGT`
>   两个环境变量才会跑(否则 skip)。它含「公开树不得含真实域名」的断言,
>   但**不是 CI 门禁**,只能手工跑或由部署方在流水线里注入环境变量。

仍需人工的 **9 个**(★ = 本次修复新增的真冲突):

| 文件 | 解法 |
|---|---|
| `Dockerfile` | 真冲突(4 处) |
| `README.md` | 真冲突(3 处) |
| `deploy/Dockerfile.frontend` | 真冲突(2 处) |
| ★ `etc/config-dev.yaml` | 真冲突(1 处:脱敏域名 vs 真实域名) |
| ★ `etc/config-prod.yaml` | 真冲突(1 处:脱敏域名 vs 真实域名) |
| `Dockerfile.local` | 无共同版本 |
| `README.zh-CN.md` | 无共同版本 |
| `docs/archive/README.md` | 无共同版本 |
| `internal/service/statuscheck/poster_test.go` | 无共同版本 |

> 曾经的 11 个里还有 `AGENTS.md` 与 `internal/service/provider/service.go`:
> 前者因**内网克隆 URL 改写修复**而自动合并(见 §2 的说明),
> 后者因两侧 oekit 依赖升级一致而不再冲突。
> **`etc/config-dev.yaml` / `etc/config-prod.yaml` 始终在列** ——
> 这两个才是门禁要盯的:它们一旦被「合并干净」地写出去,源侧真实域名就进公开仓。

### 6.3 端到端:应用解法后门禁通过

```
物化: 文件 691,port 应用 3,解法写入 4,保留目标侧 25,仍待人工 9,受保护 39
gofmt 违规: 0
类型门禁: 通过
```

> ⚠️ 上面这行是**修复前**的产物(待人工 9,但构成不同:
> 那两个配置文件被「合并干净」地写了出去)。
> 修复后同一命令的输出是:
> `物化: 文件 691,port 应用 3,解法写入 3,保留目标侧 26,仍待人工 11,受保护 39`
> ——「解法写入」从 5 降到 3(那两个配置文件不再被「合并干净」地写出去),
> 「仍待人工」从 9 升到 11。
> **当前基线是 9**,构成见 §6.2(两个文件因改写修复与依赖升级不再需要人工)。

M2 拦下的那个依赖(`internal/modules/server.go` 依赖 hold 的 `internal/readiness/checks.go`)
被「取源侧」解法一并解决 —— **3 个 port 文件至此全部可移植**。

### 6.4 实现上踩到的三个坑

1. **历史遍历必须走全部父提交**:只走第一父会漏掉合并分支引入的中间版本,
   实测多判 18 个冲突。
2. **版本集合从 diff 顺带取 blob hash**,不读内容;源侧只把改写后的内容算 hash。
   否则逐文件 `commit.File(path)` 会让整轮从 15 秒变成 5 分钟以上。
3. **行模式 diff 必须转回行文本**:`DiffLinesToRunes` 把每行编码成一个 rune,
   直接用编码串会拿到「行号字符」而不是内容(输出里全是 `\x06`)。

---

## 7. M3 实测:内容移植 + 双远端推送

M3 把「计划」落成「真实的提交」并推出去。**用本地裸镜像当远端**验证
(推送是对外可见动作,真实推 GitHub 前需显式确认)。

### 7.1 移植结果

```
移植: 基线 HEAD e1995663 → 新 HEAD 88b3198f,新建 4 个提交,应用 8 个文件,跳过 35 个
  533b5ad8 → cfc4e420  fix(statuscheck): target_url 相对路径导致 GitLab 拒写门禁 status  (2 个文件)
  4893cfb7 → 8e5cd448  fix: swagger 补 9 个 Argus 接口 + LLM 探活 DNS 抖动豁免          (2 个文件)
  5fc1b02d → 82a55568  build(make): swagger 重生成目标固化——thriftgo 插件命令入 Makefile (1 个文件)
  d9bb3a37 → 88b3198f  feat(readiness): llm-usable 可用性探测——「可达≠可用」盲区修复      (3 个文件)
```

**提交按源提交分组**:同一源提交带来的文件落在同一个目标提交里,
「这批改动来自哪个源提交」在目标侧一目了然,回滚也能按提交回滚。
作者/提交者身份与时间戳**继承源提交** ⇒ 同一输入两次移植产生**同一批 hash**(幂等)。

### 7.2 核心约束验证

| 约束 | 验证方式 | 结果 |
|---|---|---|
| **目标树既有提交 hash 不变** | 移植前的 HEAD 必须是新 HEAD 的祖先 | ✅ |
| 幂等 | 两次移植的 HEAD 与提交列表一致 | ✅ |
| 先留底后发布 | 推送顺序 = backup → publish | ✅ |
| 任一失败不推其余 | 顺序推送,首个失败即中止 | ✅(代码路径) |
| 远端 HEAD 前进 | 两个裸镜像的 HEAD 都等于移植结果 | ✅ |
| **推出去的结果能编译** | 拉回推送结果 → gofmt + 全量类型门禁 | ✅ **gofmt 干净 + 门禁通过** |

最后一条是真正的验收:门禁跑在**推出去的产物**上,而不是源树或预演目录上。

### 7.3 提交消息形态

```
fix(statuscheck): target_url 相对路径导致 GitLab 拒写门禁 status——绝对地址或省略

由 git-ferry 历史同步模式移植:身份改写 git.enjoye.top/enjoydream/code-review-service → github.com/yi-nology/code-review-service

文件(2):
- etc/config-dev.yaml
- etc/config-prod.yaml
```

保留原 subject + 追加改写说明 + 文件清单。

### 7.4 痕迹清理的两档语义

| 类别 | 行为 | 实测 |
|---|---|---|
| 内网域名(`*.enjoye.top`) | **可自动清理**:按规则替换或删除 | 10 处命中,清理后不留痕 |
| 密钥形态(`Bearer` / `sk-` / `ghp_` / `api_key=`) | **强制 hold**:命中即不移植,不做自动替换 | 单元测试覆盖 |

自动替换密钥可能留下半吊子状态,比留痕更危险 —— 密钥类一律交人工。

### 7.5 接入面(M3.5)

| 面 | 形态 | 实测 |
|---|---|---|
| core 服务层 | `HistoryPlan` / `HistoryResolve` / `HistoryApply` / `HistoryPush` | ✅ |
| HTTP | `/api/v1/histsync/{plan,resolve,apply,push}` | ✅ 冒烟:`code=200`,summary 与基线一致 |
| CLI | `gitferry histsync +plan/+resolve/+apply/+push` | ✅ 冒烟:返回 Envelope |
| MCP | `plan_history_sync` / `resolve_history_sync` / `apply_history_sync` | ✅ 注册(工具总数 34) |

`resolve` 端点返回的 9 份冲突卷宗含「源侧独有 / 目标侧独有」逐行对照 ——
这就是 AI 建议器的输入形态。

### 7.6 通道 DB 化(M3.6)

「有哪些双仓在同步、各自什么规则」变成**可查询的状态**,不再每次调用重传一整套规则。

| 项 | 形态 |
|---|---|
| 模型 | `model.HistoryChannel`(module/host 映射、路径规则、痕迹规则、推送目标、分支) |
| 规则存储 | 路径/痕迹/推送目标以 JSON 落库 |
| DAO | `dao.HistoryChannelDAO`(CRUD + 按名查) |
| 迁移 | `model/init.go` 的 AutoMigrate |
| 仓库解析 | `sourceRepoKey`/`targetRepoKey` → 克隆到 `Git.TempDir/history/channel-N/{source,target}`(增量 fetch,失败重建);或 `sourceDir`/`targetDir` 直连 |
| HTTP | `/api/v1/histsync/channels/{,/:id,/:id/update,/:id/delete,/:id/plan,/:id/resolve,/:id/apply,/:id/push}` |
| CLI | `+channel-list / +channel-create / +channel-delete / +channel-plan / +channel-resolve / +channel-apply / +channel-push` |
| MCP | `list_histsync_channels` / `plan_histsync_channel` / `resolve_histsync_channel`(只读面,共 37 个工具) |

**实测**:新建通道 → 规则经 DB JSON 往返 → 按通道出的计划与解法分布,
与直接传参**完全一致**:

```
按通道 plan:    Total 691 / Same 609 / Port 3 / Hold 40 / Preserve 39
按通道 resolve: take_ours 26 / conflict 5 / no_base 4 / take_theirs 3 / merge_clean 2
                待裁决卷宗 9
```

**踩到的坑**:DTO 的 JSON tag 一开始用 camelCase,而既有 `HistoryOptionsDTO` 与 CLI 都是
snake_case ⇒ 建通道直接 500。已统一为 snake_case(全 histsync API 一致)。

### 7.7 接口重做:通道优先(v2.1)

M3.5 的接口有**两套并行的同义接口**(传参版 + 通道版)—— 传参版绕过通道配置,
还要求调用方知道服务端的磁盘路径。重做后只剩一套。

| | 重做前 | 重做后 |
|---|---|---|
| 配置 | 每次调用传一整套规则 | **只在通道里**(DB 持久化) |
| 执行 | `POST /histsync/plan` + body | `POST /histsync/channels/:id/plan`,**无请求体** |
| 推送产物路径 | 客户端回传服务端的临时路径 | **执行记录里服务端自己记着** |
| 推送目标 | 通道里一个 JSON 数组 | **独立表**,凭据加密落库,顺序 = 推送优先级 |
| 审计 | 无 | **执行记录**:计划统计 / commit 映射 / 检测报告 / ref 对照 |
| 回滚 | 无 | **按执行记录回滚**到指定 ref(留底提供保障) |

**端到端实测**(本地裸镜像当远端):

```
建通道           → code=200, id=1
建两个推送目标   → backup / publish(顺序 = 优先级)
按通道 plan      → 691/609/3/40/39(与基线一致)
按通道 apply     → run_id=1, status=success, e1995663 → 88b3198f
按通道 push      → pushed=[backup, publish],两端 HEAD 都是 88b3198f
执行记录列表     → run 1 status=success e1995663→88b3198f
回滚到 base_head → publish 回到 e1995663,backup 仍保留 88b3198f
```

回滚那一步最有说服力:**发布目标退回同步前,留底仍留着同步后的内容** ——
这正是留底的意义(发布能回滚,新内容不丢)。

**重做中踩到的两个真坑**:

1. **GORM `default:true` 吞掉「显式禁用」** —— 零值 `false` 被 GORM 省略,
   数据库默认值 `true` 生效,「禁用通道」被悄悄改成「启用」。
   修法:model 不用 `default` 标签,DTO 用 `*bool` 区分「未指定」与「显式 false」。
   (单测 `TestHistoryChannelDisabled` 抓到的。)
2. **DTO 的 JSON tag 风格必须与同域既有 DTO 一致** —— camelCase vs snake_case 不一致时
   字段**静默忽略**:接口返回 200,但规则从未生效。
   修法:新增 DTO 前先 grep 同域既有 DTO;配置落库后加「经 DB 往返 == 直接传参」的等价性断言。

**MCP 工具面只留只读**:通道列表 / 计划 / 解法 / 执行记录(共 35 个工具)。
apply 产生本地提交(不推送)也留在 CLI;推送必须走 CLI + 人工确认。

### 7.8 执行记录接审计哈希链(M3.8)

**两条链,职责分明**:

| 链 | 载体 | 范围 | 回答的问题 |
|---|---|---|---|
| 既有审计链 | `OperationLog`(全局) | 全系统写操作 | **谁**在什么时候对**什么**做了什么 |
| 通道执行链 | `HistoryRun.PrevHash/EntryHash` | 单个通道内 | 这批同步的**审计材料**有没有被改 |

两者同构(`EntryHash = SHA256(PrevHash || payload)`),但**不混在一起** ——
不同通道的同步记录混在一条链里没有意义,而且链会随通道数无限增长。

**通道执行链的哈希覆盖全部审计材料**:基线 HEAD、新 HEAD、计划统计、commit 映射、
应用/未应用文件、痕迹检测报告、门禁结果、推送前后 ref 对照。

**只在终态计算哈希**:执行中的记录字段还在变,提前算出的哈希必然失配。
续链取该通道最近一条**已入链**的记录 —— 执行中的记录没有哈希,必须跳过。

**实测(直接改库篡改)**:

```
篡改前   GET /histsync/channels/1/audit
         → {"ok":true,"checked":1,"message":"audit chain intact"}

改库      UPDATE history_runs SET head='被人改过的 head' WHERE id=1

篡改后   GET /histsync/channels/1/audit
         → {"ok":false,"checked":0,"broken_at":1,"message":"entry_hash mismatch (tampered?)"}

同时     GET /ops/audit-chain/verify
         → {"ok":true,"checked":4,"message":"audit chain intact"}   ← 两条链互不影响
```

**关键动作也进既有审计链**:建通道/改通道/删通道、建目标/删目标、apply、push、rollback
各写一条 `OperationLog`,操作者由壳层鉴权中间件提供(`api-key:<指纹>`),
`details` 里带 `base_head`/`head`/`entry_hash`/`prev_hash`:

```
history.apply          历史同步:内容移植
history.target.create  创建推送目标 publish
history.target.create  创建推送目标 backup
history.channel.create 创建历史同步通道 cr
```

**接入面**:
- HTTP `GET /histsync/channels/:id/audit` —— 通道链校验
- CLI `gitferry histsync +channel-audit` —— 同上(共 16 个子命令)
- MCP `verify_histsync_channel_audit` —— 同上(共 36 个工具)

**单测**:`TestHistoryRunChain`(含篡改检出)、`TestHistoryRunChainSkipsInflight`
(执行中的记录跳过而不打断链)、`TestHistoryRunChainIsolatedPerChannel`(通道间互不干扰)。

---

### 7.9 裁决闭环 + Agent Skill 手册(M3.9)

「AI 只做语义判断」要落地,必须有**能把语义判断送回管道**的通路 —— 否则 AI 只能给建议。

```
resolve → 冲突卷宗(dossiers)
   ↓  AI/人工判定:keep_target / take_source / merged + 理由
override-set → HistoryOverride(DB)
   ↓
resolve(重算) → 该文件的解法被裁决覆盖,summary.resolved 计数
   ↓
apply → 用裁决内容
```

**实测**:

```
写回裁决      internal/service/provider/service.go → keep_target
              理由:目标侧有 Gerrit 平台集成(源侧没有)
重新 resolve  summary.resolved 0 → 1
              该文件解法 conflict → take_ours
              理由带上了裁决理由与裁决者(agent:test)
              待裁决数 9 → 8
apply 后       新 HEAD 保留目标侧的 Gerrit 集成(未被源侧覆盖) ✅
统一审计链     出现 history.override 动作 ✅
```

**裁决只对 `hold` 生效**;其余文件仍走确定性判定。
**裁决改变实际发布内容** ⇒ MCP 工具侧标为危险操作(`set_histsync_override`),需用户确认。

### 7.10 Agent Skill 手册

`skills/gitferry-histsync/`:

| 文件 | 内容 |
|---|---|
| `SKILL.md` | 与镜像中心的**模式区分**(名字撞车警示)、核心概念、6 步标准流程、**AI 职责边界表**、安全规则、常见陷阱、首次配置通道 |
| `references/conflict-adjudication.md` | 判定词表(`keep_target`/`take_source`/`merged`)、**信号表**(看什么 → 结论)、3 个真实实例、常见错误表 |

手册里最要紧的两块:

1. **AI 职责边界表** —— 明确「机械变换(路径改写/gofmt/差异判定/类型门禁/痕迹检测)
   不碰,只做语义判断(能否公开 / 要不要脱敏 / 冲突怎么解)」。
2. **冲突裁决手册** —— 教「看独有行的**性质**而不是数量」:
   - 目标侧已把内网域名脱敏成示例域名 → `keep_target`
   - 目标侧有源侧没有的 Gerrit 集成 → `keep_target`
   - 两侧只是措辞不同 → `merged`

### 7.11 预检:摊开影响面但不推送

预检回答 plan/resolve 回答不了的三个问题:**推出去能不能编译**、**将产生哪些提交**、
**推送会不会覆盖别人的提交**。

**实测(真仓 + 本地裸镜像)**:

```
预检  base_head → new_head: e1995663 → 88b3198f
      门禁:门禁通过 —— 物化结果 gofmt 干净且类型检查通过
      needs_force: False
      backup  (空仓库) :: 远端是空仓库(首次发布),推送会创建该分支
      publish (齐平)   :: 远端与本地基线一致,推送是快进
      ⚠️ 远端 backup 状态未知(不可达或空仓库),推送前请确认
      ⚠️ 有 9 个冲突待人工裁决,未裁决的文件不会带过去

随后真的 apply + push:
      两个裸镜像的 HEAD 都是 88b3198f   ← 与预检的 new_head 完全一致 ✅
```

**预检与执行走同一套代码路径** —— 这是预检可信的前提。
单测 `TestHistoryPreview` 断言:预检的 `new_head` == 真正 apply 后的 HEAD。

**实现中发现的一个真 bug**:`history.Apply` 原先只更新了 ref,**没把新 tree 检出到工作区**,
于是门禁验的是目标树的**旧**内容 —— 结论看着对,验的对象却是错的。
修法:Apply 结束时把新 HEAD 检出到工作区,工作区目录即「按计划应用后的目标树」。

### 7.12 端到端演练(DR drill)

用**本地裸镜像当「GitHub」**跑完整条链(不碰真实远端):
`建通道 → 目标 → 裁决 → 预检 → apply → push → 审计 → 回滚 → 再推`。

**演练结果**:

```
① 裁决后 resolve   resolved=9  待裁决=0
② 预检             门禁通过 | 预演HEAD=88b3198f | 需强推=False
③ apply            run#1  e1995663→88b3198f
④ push             ['backup','github'] → 两端都是 88b3198f
⑤ 审计             通道链:intact(checked=4) | 全局链:intact(checked=16)
⑥ 回滚             ref=e1995663 → github 回到 e1995663
⑦ 回滚后 github.git 实际 HEAD: e1995663  ✅ 与回滚目标一致
⑧ 回滚后再推       两端又都是 88b3198f(留底里一直有这份内容)
⑨ 执行记录链
     run1 kind=apply     e1995663→88b3198f  pushedRun=None
     run2 kind=push      e1995663→88b3198f  pushedRun=1
     run3 kind=rollback  88b3198f→e1995663  pushedRun=1
     run4 kind=push      e1995663→88b3198f  pushedRun=1
```

### 7.13 演练抓出来的两个 bug

**① push 改了已入链的 run → 链误报「被篡改」。**

第一次演练时通道链报 `entry_hash mismatch (tampered?)`,但**没人篡改任何东西**。
根因:`push` 往 apply 记录里写 `RefsAfter`,而哈希是 apply 结束时算的 —— **已入链的记录被改动**。

修法:**一次动作一条记录**。`apply` / `push` / `rollback` 各写各的,
`push` 记录引用被推的 apply 记录(`pushedRunId`),**不修改**它。
链只增不改,永不误报。

**② push 找「最近执行记录」,找到的是上一条 push 记录。**

`apply` 记录才有产物路径(`WorkRepoPath`);push 记录自己不带产物。
所以 push 必须找最近一次 **kind=apply** 的记录,而不是最近一条记录。

> 这两个 bug **单测都测不出来** —— 它们只在「apply 之后又 push、再回滚、再 push」
> 这种**多动作串联**下才暴露。只有把整条链真跑一遍才能发现。

### 7.14 9 个冲突的裁决结论:高度一致

逐个读完 9 个卷宗后,**判定结果全是 `keep_target`**,共同规律是:

> **源侧的改动是「内网形态」,目标侧是「公开形态」。**

| 文件 | 源侧独有的东西 |
|---|---|
| AGENTS.md | 内网工作副本说明(`harbor.enjoye.top`、215 环境) |
| Dockerfile / deploy/Dockerfile.frontend | `harbor.enjoye.top/library/*` 内网镜像源 |
| Dockerfile.local | `GOPRIVATE/GOINSECURE/GIT_SSL_NO_VERIFY`(构建期跳过证书校验) |
| README.md / .zh-CN.md | 旧版(目标侧有 badge、Forgejo/Gitee/Gerrit、Release 产物) |
| docs/archive/README.md | 「internal-only artifacts」措辞 |
| provider/service.go | 旧版(目标侧有完整 Gerrit 集成 + ClientHolder) |
| poster_test.go | 真实对外域名 `code-review.me`(目标侧已脱敏) |

**含义**:这类冲突**大部分可以内建规则** —— 「源侧独有行含内网痕迹 → keep_target」,
不必每次都靠 AI 逐个判。AI 的价值在**规则覆盖不到**的那些。

### 7.15 冲突自动判据:9 个冲突解掉 8 个

把演练里 9 个人工裁决的共同规律**内建为确定性判据**(通道级可配、可关):

| 判据 | 命中条件 | 实测命中 |
|---|---|---|
| `internal_trace` | 源侧独有行含内网形态(内网 registry / 内网域名 / 跳过证书校验 / 内网措辞) | AGENTS.md、Dockerfile、Dockerfile.local、deploy/Dockerfile.frontend、docs/archive/README.md |
| `public_asset` | 目标侧含公开资产(badge / license / 公共镜像源 / 多平台)而源侧没有 | README.md、README.zh-CN.md |
| `placeholder_domain` | 源侧用真实对外域名,目标侧已脱敏为示例域名 | poster_test.go |

```
不开判据:待裁决 9 个
开判据:  待裁决 1 个  ← internal/service/provider/service.go
```

**剩下那 1 个是应该留下的**:两版功能都全、只是新旧不同 —— 真语义判断。

**一条重要的顺序约束**:判据**只在合并解不了时兜底**。
若把判据放在合并之前,「能干净合并」的文件会被降级成「保留目标侧」,
**丢掉源侧的非冲突改动**。这个顺序错误在实测中被发现并纠正
(`etc/config-*.yaml` 本可干净合并,却一度被降级)。

### 7.16 演练二:判据 + 凭据轮换 + 一条命令

```
① 开判据 resolve   待裁决 9 → 1
② 人工裁决最后 1 个  待裁决 1 → 0
③ 凭据轮换          has_credential=true, cred_type=token(只换密钥,URL 不动)
④ 预检              门禁通过;凭据状态正确反映(backup 无凭据、github 有 token)
⑤ +sync             blocked=None,pushed=[backup, github] —— 一条命令跑完预检→移植→推送
   最终 github.git HEAD: 88b3198f   backup.git HEAD: 88b3198f
⑥ 审计              通道链:intact(checked=2) | 全局链:intact(checked=5)
```

**`+sync` 的阻塞语义**:门禁不过 / 还有待裁决冲突 / 需要强推 —— 三者任一,
**停下并报告,不擅自往下走**。这三件事都不是程序能替用户决定的。

**凭据轮换与「更新目标」的区别**:轮换**只换密钥**,不碰 URL 与目标名 ——
换密钥是常规运维动作,不该有机会顺手改掉推送到哪里。

---

## 8. 本基线未覆盖的部分

M1 计划生成、M2 发布前校验、M2.5 hold 自动解法、M3 移植推送、M3.5 接入面已做到。以下待后续:

- **真实推 GitHub**:已用本地裸镜像完整验证推送语义,真实推送待显式确认
- **真实推 GitHub**:已用本地裸镜像验证推送语义,真实推送待显式确认后执行

---

## 9. 附录:完整计划报告

以下由 `HISTORY_LIVE_REPORT` 生成,是本基线的原始产物。

## 附录:计划报告原文

### 统计

| 处置 | 含义 | 数量 |
|---|---|---|
| `port` | 源侧超集,需覆盖目标 | 3 |
| `port_with_scrub` | 需清理痕迹后移植 | 0 |
| `hold` | 两侧各有独有内容,待人工确认 | 40 |
| `exclude` | 命中排除规则,不参与同步 | 0 |
| `preserve` | 仅目标侧存在,受保护 | 39 |
| `same` | 两侧一致,无需动作 | 609 |

合计 691 个路径;痕迹命中 10 处;相对目标侧 +1052 / -974 行。

## 需要移植（3）

| 路径 | 原因 | 增/删 | 来源提交 | 痕迹 |
|---|---|---|---|---|
| `internal/modules/server.go` | 源侧为超集(仅新增 4 行,无删改),以源侧内容覆盖 | +4 / -0 | d9bb3a37 feat(readiness): llm-usable 可用性探测——「可达≠可用」盲区修复（429 即报配额尽） | — |
| `internal/readiness/checks_test.go` | 源侧新增,目标侧无此路径 | +0 / -0 | d9bb3a37 feat(readiness): llm-usable 可用性探测——「可达≠可用」盲区修复（429 即报配额尽） | — |
| `swagger/openapi.yaml` | 源侧为超集(仅新增 493 行,无删改),以源侧内容覆盖 | +493 / -0 | 4893cfb7 fix: swagger 补 9 个 Argus 接口 + LLM 探活 DNS 抖动豁免 | — |

## 待人工确认（40）

| 路径 | 原因 | 增/删 | 来源提交 | 痕迹 |
|---|---|---|---|---|
| `AGENTS.md` | 两侧各有独有内容(源侧 +29 / 目标侧 +18),需人工确认 | +29 / -18 | 68fc11d2 docs: 架构图补当前形态——执行流程与端口归属 | internal-host×1 internal-registry×1 |
| `Dockerfile` | 两侧各有独有内容(源侧 +7 / 目标侧 +5),需人工确认 | +7 / -5 | 3926619d chore(deps): OCR 二进制版本 v1.12.1 → v1.13.2 | internal-host×2 internal-registry×2 |
| `Dockerfile.deploy` | 两侧各有独有内容(源侧 +1 / 目标侧 +1),需人工确认 | +1 / -1 | 3926619d chore(deps): OCR 二进制版本 v1.12.1 → v1.13.2 | — |
| `Dockerfile.local` | 两侧各有独有内容(源侧 +3 / 目标侧 +2),需人工确认 | +3 / -2 | 3926619d chore(deps): OCR 二进制版本 v1.12.1 → v1.13.2 | — |
| `Makefile` | 两侧各有独有内容(源侧 +13 / 目标侧 +1),需人工确认 | +13 / -1 | 5fc1b02d build(make): swagger 重生成目标固化——thriftgo 插件命令入 Makefile，改 idl 后防再漏生成 | — |
| `README.md` | 两侧各有独有内容(源侧 +19 / 目标侧 +37),需人工确认 | +19 / -37 | b5172f3c docs: 架构图对齐当前分层——补引擎适配层与领域层 | — |
| `README.zh-CN.md` | 两侧各有独有内容(源侧 +19 / 目标侧 +30),需人工确认 | +19 / -30 | b5172f3c docs: 架构图对齐当前分层——补引擎适配层与领域层 | — |
| `architecture-policy.yaml` | 两侧各有独有内容(源侧 +1 / 目标侧 +3),需人工确认 | +1 / -3 | 23d33905 refactor(domain): 评审领域类型迁到 internal/domain/review，依赖方向不再倒置 | — |
| `cmd/mcp-server/main.go` | 两侧各有独有内容(源侧 +1 / 目标侧 +5),需人工确认 | +1 / -5 | b8033cbc chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、core v0.6.0 等），import 随官方拍平 | — |
| `deploy/Dockerfile.frontend` | 两侧各有独有内容(源侧 +2 / 目标侧 +2),需人工确认 | +2 / -2 | 489b6c8f harden startup and compose defaults | internal-host×2 internal-registry×2 |
| `docs/archive/README.md` | 两侧各有独有内容(源侧 +5 / 目标侧 +8),需人工确认 | +5 / -8 | 825d0f5f Merge pull request 'feat/productization-iteration-0-3' (#25) from feat/productization-iteration-0-3 into main | — |
| `etc/config-dev.yaml` | 两侧各有独有内容(源侧 +1 / 目标侧 +1),需人工确认 | +1 / -1 | 533b5ad8 fix(statuscheck): target_url 相对路径导致 GitLab 拒写门禁 status——绝对地址或省略 | — |
| `etc/config-prod.yaml` | 两侧各有独有内容(源侧 +1 / 目标侧 +1),需人工确认 | +1 / -1 | 533b5ad8 fix(statuscheck): target_url 相对路径导致 GitLab 拒写门禁 status——绝对地址或省略 | — |
| `go.mod` | 两侧各有独有内容(源侧 +5 / 目标侧 +7),需人工确认 | +5 / -7 | 320bce07 chore(deps): oekit / oagentkit 依赖地板抬到最新（11 个模块） | — |
| `go.sum` | 两侧各有独有内容(源侧 +40 / 目标侧 +14),需人工确认 | +40 / -14 | 320bce07 chore(deps): oekit / oagentkit 依赖地板抬到最新（11 个模块） | — |
| `internal/config/loader.go` | 两侧各有独有内容(源侧 +13 / 目标侧 +47),需人工确认 | +13 / -47 | 44030993 refactor(deps)!: ekit 单体迁移 oekit 模块矩阵 | — |
| `internal/config/yaml.go` | 两侧各有独有内容(源侧 +2 / 目标侧 +27),需人工确认 | +2 / -27 | c1570348 refactor(cost): service 层脱离 ORM，定价走仓储接口 + oekit/cache 缓存 | — |
| `internal/mcp/types.go` | 两侧各有独有内容(源侧 +1 / 目标侧 +1),需人工确认 | +1 / -1 | de879015 refactor(mcp,repo,common): 审查遗留清理——参数解析去重/统计过滤统一/共享层测试 | — |
| `internal/modules/gerrit.go` | 两侧各有独有内容(源侧 +42 / 目标侧 +19),需人工确认 | +42 / -19 | 44030993 refactor(deps)!: ekit 单体迁移 oekit 模块矩阵 | — |
| `internal/modules/probe.go` | 两侧各有独有内容(源侧 +13 / 目标侧 +1),需人工确认 | +13 / -1 | 4893cfb7 fix: swagger 补 9 个 Argus 接口 + LLM 探活 DNS 抖动豁免 | — |
| `internal/modules/provider.go` | 两侧各有独有内容(源侧 +2 / 目标侧 +3),需人工确认 | +2 / -3 | 6ad99ea4 refactor(provider): 仓库列表缓存换 oekit/pkg/cache，补并发回源合并 | — |
| `internal/modules/writeback.go` | 两侧各有独有内容(源侧 +5 / 目标侧 +8),需人工确认 | +5 / -8 | 44030993 refactor(deps)!: ekit 单体迁移 oekit 模块矩阵 | — |
| `internal/ocr/llm_coverage_test.go` | 两侧各有独有内容(源侧 +9 / 目标侧 +65),需人工确认 | +9 / -65 | 77c5d480 refactor: 用 agent-harness / agent-memory 重构编排与记忆层 | — |
| `internal/ocr/llm_types.go` | 两侧各有独有内容(源侧 +26 / 目标侧 +13),需人工确认 | +26 / -13 | 3b2314bb docs: 架构图补当前形态 + 清理分期实施语言 | — |
| `internal/ocr/runner.go` | 两侧各有独有内容(源侧 +31 / 目标侧 +61),需人工确认 | +31 / -61 | 3b2314bb docs: 架构图补当前形态 + 清理分期实施语言 | — |
| `internal/ocr/runner_parse.go` | 两侧各有独有内容(源侧 +4 / 目标侧 +5),需人工确认 | +4 / -5 | 23d33905 refactor(domain): 评审领域类型迁到 internal/domain/review，依赖方向不再倒置 | — |
| `internal/readiness/checks.go` | 两侧各有独有内容(源侧 +129 / 目标侧 +6),需人工确认 | +129 / -6 | d9bb3a37 feat(readiness): llm-usable 可用性探测——「可达≠可用」盲区修复（429 即报配额尽） | — |
| `internal/repository/postgres/ocr_settings.go` | 两侧各有独有内容(源侧 +60 / 目标侧 +78),需人工确认 | +60 / -78 | 44030993 refactor(deps)!: ekit 单体迁移 oekit 模块矩阵 | — |
| `internal/repository/postgres/ocr_settings_cache_test.go` | 两侧各有独有内容(源侧 +7 / 目标侧 +67),需人工确认 | +7 / -67 | 825d0f5f Merge pull request 'feat/productization-iteration-0-3' (#25) from feat/productization-iteration-0-3 into main | — |
| `internal/repository/repoerr/errors.go` | 两侧各有独有内容(源侧 +1 / 目标侧 +1),需人工确认 | +1 / -1 | 103d43be Merge pull request 'refactor/pr-b1-ocr-registry' (#19) from refactor/pr-b1-ocr-registry into main | — |
| `internal/service/judge/prompts.go` | 两侧各有独有内容(源侧 +3 / 目标侧 +7),需人工确认 | +3 / -7 | 23d33905 refactor(domain): 评审领域类型迁到 internal/domain/review，依赖方向不再倒置 | — |
| `internal/service/provider/service.go` | 两侧各有独有内容(源侧 +27 / 目标侧 +107),需人工确认 | +27 / -107 | 6ad99ea4 refactor(provider): 仓库列表缓存换 oekit/pkg/cache，补并发回源合并 | — |
| `internal/service/statuscheck/poster_test.go` | 两侧各有独有内容(源侧 +3 / 目标侧 +3),需人工确认 | +3 / -3 | b8033cbc chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、core v0.6.0 等），import 随官方拍平 | — |
| `internal/service/writeback/gerrit_adapter.go` | 两侧各有独有内容(源侧 +7 / 目标侧 +27),需人工确认 | +7 / -27 | b8033cbc chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、core v0.6.0 等），import 随官方拍平 | — |
| `internal/strutil/strutil.go` | 两侧各有独有内容(源侧 +6 / 目标侧 +12),需人工确认 | +6 / -12 | 45312b0f refactor(core): 高内聚低耦合重构——统一落库/失败上报、domain 纯净化、接口按角色拆窄 | — |
| `internal/vcs/cloneurl.go` | 两侧各有独有内容(源侧 +5 / 目标侧 +24),需人工确认 | +5 / -24 | 77c5d480 refactor: 用 agent-harness / agent-memory 重构编排与记忆层 | — |
| `internal/vcs/cloneurl_test.go` | 两侧各有独有内容(源侧 +1 / 目标侧 +18),需人工确认 | +1 / -18 | 77c5d480 refactor: 用 agent-harness / agent-memory 重构编排与记忆层 | — |
| `main.go` | 两侧各有独有内容(源侧 +1 / 目标侧 +7),需人工确认 | +1 / -7 | d747e512 chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、data v0.8.2、kitex v0.5.1、core v0.6.0、nacos·obs v0.2.0、nns-endpoint v0.1.4、network v0.5.0） | — |
| `test_gitcode_webhook.sh` | 两侧各有独有内容(源侧 +4 / 目标侧 +4),需人工确认 | +4 / -4 | f903c067 Merge pull request 'feature/review-map-enhancements' (#11) from feature/review-map-enhancements into main | — |
| `test_github_webhook.sh` | 两侧各有独有内容(源侧 +6 / 目标侧 +6),需人工确认 | +6 / -6 | f903c067 Merge pull request 'feature/review-map-enhancements' (#11) from feature/review-map-enhancements into main | — |

## 受保护(不得覆盖)（39）

| 路径 | 原因 | 增/删 | 来源提交 | 痕迹 |
|---|---|---|---|---|
| `.dockerignore` | 目标侧为超集(仅新增 1 行),保留目标侧内容 | +0 / -1 | 489b6c8f harden startup and compose defaults | — |
| `.github/workflows/build.yml` | 目标侧为超集(仅新增 13 行),保留目标侧内容 | +0 / -13 | 9e86c9b2 ci: 构建产物保持工作树干净 + 修正可复现性的表述 | — |
| `.gitignore` | 目标侧为超集(仅新增 3 行),保留目标侧内容 | +0 / -3 | 391d7df0 chore(hygiene): 目录卫生——修 .gitignore 误伤、取消跟踪本地工具计划、脱敏网关名 | — |
| `CHANGELOG.md` | 目标侧为超集(仅新增 46 行),保留目标侧内容 | +0 / -46 | a530a1a3 docs: 补 v0.2.0 变更记录 | — |
| `CODE_OF_CONDUCT.md` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `CONTRIBUTING.md` | 目标侧为超集(仅新增 8 行),保留目标侧内容 | +0 / -8 | d05b1f6d docs: README 等三份文档拆为中英双文件 | — |
| `CONTRIBUTING.zh-CN.md` | 目标侧为超集(仅新增 4 行),保留目标侧内容 | +0 / -4 | d05b1f6d docs: README 等三份文档拆为中英双文件 | — |
| `SECURITY.md` | 目标侧为超集(仅新增 3 行),保留目标侧内容 | +0 / -3 | d05b1f6d docs: README 等三份文档拆为中英双文件 | — |
| `docs/FUNCTIONAL_OVERVIEW.md` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/logo.svg` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/00-login.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/01-dashboard.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/02-reviews-list.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/03-review-detail.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/04-review-gate.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/05-settings-engine.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/06-settings-platforms.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/07-settings-agents.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/08-settings-rules.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/09-settings-projects.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/10-project-reports.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/11-diagnostics.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/12-sessions.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/13-contributors.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/14-weekly-report.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `docs/assets/screenshots/15-scan.png` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `frontend/src/utils/url-parser.ts` | 目标侧为超集(仅新增 3 行),保留目标侧内容 | +0 / -3 | ba9304ae feat: argus 双引擎对接——trigger 分流 + push 承接 + 误报抑制 + webhook action 修复 | — |
| `frontend/src/views/GitPlatformSettings.vue` | 目标侧为超集(仅新增 6 行),保留目标侧内容 | +0 / -6 | 2a0b601c Merge pull request 'feat/productization-iteration-0-3' (#28) from feat/productization-iteration-0-3 into main | — |
| `internal/config/etc_strict_test.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/domain/platform/platform.go` | 目标侧为超集(仅新增 8 行),保留目标侧内容 | +0 / -8 | 1bb39745 refactor(repository): 迁移 8 个仓储接口为 domain 类型(#6 Phase 2-4) | — |
| `internal/ocr/runner_test.go` | 目标侧为超集(仅新增 88 行),保留目标侧内容 | +0 / -88 | 23d33905 refactor(domain): 评审领域类型迁到 internal/domain/review，依赖方向不再倒置 | — |
| `internal/service/gerrit/client.go` | 目标侧为超集(仅新增 39 行),保留目标侧内容 | +0 / -39 | 3651e6b3 fix(ci)+style: 同步 CI 改造到内网仓，修正 import 分组排序 | — |
| `internal/service/gerrit/client_test.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/service/gerrit/holder.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/service/provider/commitstatus.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/service/provider/commitstatus_gitlab_test.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/service/provider/commitstatus_test.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `internal/service/provider/gerrit_registration_test.go` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |
| `scripts/seed-demo.sql` | 仅目标侧存在,原样保留,不得覆盖 | +0 / -0 | — | — |

## 来源提交分组

| 提交 | 标题 | 文件数 | 聚合处置 |
|---|---|---|---|
| `489b6c8f` | harden startup and compose defaults | 2 | `hold` |
| `9e86c9b2` | ci: 构建产物保持工作树干净 + 修正可复现性的表述 | 1 | `preserve` |
| `391d7df0` | chore(hygiene): 目录卫生——修 .gitignore 误伤、取消跟踪本地工具计划、脱敏网关名 | 1 | `preserve` |
| `68fc11d2` | docs: 架构图补当前形态——执行流程与端口归属 | 1 | `hold` |
| `a530a1a3` | docs: 补 v0.2.0 变更记录 | 1 | `preserve` |
| `d05b1f6d` | docs: README 等三份文档拆为中英双文件 | 3 | `preserve` |
| `3926619d` | chore(deps): OCR 二进制版本 v1.12.1 → v1.13.2 | 3 | `hold` |
| `5fc1b02d` | build(make): swagger 重生成目标固化——thriftgo 插件命令入 Makefile，改 idl 后防再漏生成 | 1 | `hold` |
| `b5172f3c` | docs: 架构图对齐当前分层——补引擎适配层与领域层 | 2 | `hold` |
| `23d33905` | refactor(domain): 评审领域类型迁到 internal/domain/review，依赖方向不再倒置 | 4 | `hold` |
| `b8033cbc` | chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、core v0.6.0 等），import 随官方拍平 | 3 | `hold` |
| `825d0f5f` | Merge pull request 'feat/productization-iteration-0-3' (#25) from feat/productization-iteration-0-3 into main | 2 | `hold` |
| `533b5ad8` | fix(statuscheck): target_url 相对路径导致 GitLab 拒写门禁 status——绝对地址或省略 | 2 | `hold` |
| `ba9304ae` | feat: argus 双引擎对接——trigger 分流 + push 承接 + 误报抑制 + webhook action 修复 | 1 | `preserve` |
| `2a0b601c` | Merge pull request 'feat/productization-iteration-0-3' (#28) from feat/productization-iteration-0-3 into main | 1 | `preserve` |
| `320bce07` | chore(deps): oekit / oagentkit 依赖地板抬到最新（11 个模块） | 2 | `hold` |
| `44030993` | refactor(deps)!: ekit 单体迁移 oekit 模块矩阵 | 4 | `hold` |
| `c1570348` | refactor(cost): service 层脱离 ORM，定价走仓储接口 + oekit/cache 缓存 | 1 | `hold` |
| `1bb39745` | refactor(repository): 迁移 8 个仓储接口为 domain 类型(#6 Phase 2-4) | 1 | `preserve` |
| `de879015` | refactor(mcp,repo,common): 审查遗留清理——参数解析去重/统计过滤统一/共享层测试 | 1 | `hold` |
| `4893cfb7` | fix: swagger 补 9 个 Argus 接口 + LLM 探活 DNS 抖动豁免 | 2 | `hold` |
| `6ad99ea4` | refactor(provider): 仓库列表缓存换 oekit/pkg/cache，补并发回源合并 | 2 | `hold` |
| `d9bb3a37` | feat(readiness): llm-usable 可用性探测——「可达≠可用」盲区修复（429 即报配额尽） | 3 | `hold` |
| `77c5d480` | refactor: 用 agent-harness / agent-memory 重构编排与记忆层 | 3 | `hold` |
| `3b2314bb` | docs: 架构图补当前形态 + 清理分期实施语言 | 2 | `hold` |
| `103d43be` | Merge pull request 'refactor/pr-b1-ocr-registry' (#19) from refactor/pr-b1-ocr-registry into main | 1 | `hold` |
| `3651e6b3` | fix(ci)+style: 同步 CI 改造到内网仓，修正 import 分组排序 | 1 | `preserve` |
| `45312b0f` | refactor(core): 高内聚低耦合重构——统一落库/失败上报、domain 纯净化、接口按角色拆窄 | 1 | `hold` |
| `d747e512` | chore(deps): oekit 矩阵升级最新（bootstrap/middleware v0.5.0、logx v0.3.0、hertz v0.5.2、gormx v0.5.1、data v0.8.2、kitex v0.5.1、core v0.6.0、nacos·obs v0.2.0、nns-endpoint v0.1.4、network v0.5.0） | 1 | `hold` |
| `f903c067` | Merge pull request 'feature/review-map-enhancements' (#11) from feature/review-map-enhancements into main | 2 | `hold` |
