# Git 备份/镜像工具调研证据 (GitFerry 功能探索)

## 1. cooperspencer/gickup
来源: https://github.com/cooperspencer/gickup , conf.example.yml (已读 README + conf.example.yml)

### 核心能力
- clone/mirror 仓库从一主机到另一主机；源: GitHub, GitLab, Codeberg(Forgejo), Gitea, Gogs, Bitbucket, OneDev, Sourcehut, Opengist, Any(任意 URL/本地 bare)
- 目标: GitHub, GitLab, Codeberg/Gitea, Gogs, OneDev, Sourcehut, Radicle, Local, S3, Azure Blob, WebDAV
- 目标端还支持 zip 归档、bare/mirror clone、LFS、keep N 份轮换

### 配置模型 (YAML, gickup_spec.json schema)
- `source[].<platform>[]` 多账户数组; token / token_file / user / username+password / ssh+sshkey / url (GHES 等)
- include / exclude / includeorgs / excludeorgs; wiki, issues(仅本地), starred, contributed, gists
- `filter`: stars 门槛, lastactivity(如 1y), excludearchived, languages, excludeforks
- `destination`: createorg, visibility(repositories/organizations), force push, mirror.enabled(自己 clone+push 而非交给目标端 pull-mirror), mirrorinterval(2h)
- local 目标: structured(host/user/repo), zip, keep:5, bare, mirror, lfs
- s3: endpoint/bucket/静态或 IAM 凭证/region/storageclass/src_repo_url_tag_key/datecreatedir/lfs/zip
- multi-config: 一个文件多个 source→destination 对; cron 在第一个配置生效

### 调度 / 通知 / 可观测
- `cron: 0 22 * * *` (无 cron 则跑一次退出); 时区靠容器
- `log`: timeformat, file-logging(dir/file/maxage)
- `metrics.prometheus`: endpoint /metrics, listen_addr :6178
- `metrics.heartbeat.urls` + `failure_urls` (healthchecks.io / deadmanssnitch 成功/失败分路 ping)
- `metrics.push`: ntfy, gotify, apprise

### 增量与失败重试
- git 本身增量 fetch/push; radicle 目标有 prune / force(覆盖分叉 refs)
- GitLab mirroring 作者注明难以测试 (需 EE)；无内建重试框架

### 安全设计
- token 可来自文件 token_file; 环境变量注入 S3 key
- 近期改为 credential helper 传 git 凭证 (替代 http.extraHeader, 避免凭证进进程参数)
- GitHub App 认证 (app_id / installation_id / private_key_file), 目标端用 App installation token push
- 目标仓库默认 private

### 独特亮点
1. 多目标扇出(一个源到本地+S3+WebDAV+其它 forge)
2. Prometheus + heartbeat 成功/失败双通道
3. 过滤器(stars/lastactivity/languages/archived/fork)
4. wiki/issues/gists/starred 附带备份
5. zip+keep 轮换备份
6. YAML schema (gickup_spec.json) 供 IDE 校验
7. multi-config 文件内多对映射
8. Radicle / OpenGist 等冷门源目标

---
(待补: Hesokuri / git-repo-mirror / GitLab mirroring / Forgejo-Gitea mirror)

## 2. Talgat/hesokuri (Clojure, 2013)
来源: https://github.com/Talgat/hesokuri
- 分布式 P2P git 备份/复制 daemon: commit 后自动 push 到所有配置的 peer
- 分支命名 X_hesokr_Y (来源peer标识), 支持经第三方 peer 转发中转同步
- live edit 分支 hesokuri: 工作区干净时自动 reset 到对端提交
- 配置: ~/.hesocfg 或 HESOCFG, Clojure 向量/映射, 每 repo 映射 peer→本地路径
- 自动建目录/init 空仓库; 不可达 peer 每 3 分钟重试 ping+push
- Web UI :8080 查看各 source/branch/peer 最后推送 hash, 可手动强制推送
- 安全: peer 间 SSH 公钥互信; 适合敏感数据不出自有机器场景
- 亮点: 事件驱动(commit hook)+周期补偿重试+多副本 N-way

## 3. gabrie30/ghorg (Go)
来源: https://github.com/gabrie30/ghorg
- 按 org/user 全量 clone/pull 仓库到单一目录, 供备份/审计/代码搜索
- Provider: GitHub(含 GHE/App 认证), GitLab(含自建), Bitbucket Cloud/Server, Gitea, Codeberg/Forgejo, Sourcehut
- 备份模式: --backup (git clone --mirror), --clone-wiki, --include-submodules, --git-filter=blob:none(排除二进制)
- 过滤链: flag(match-regex/prefix/topics/skip-archived/skip-forks) → target-repos-path(精确清单) → ghorgonly(子串白名单文件) → ghorgignore(黑名单) → --repo-filter-hook(JSON stdin→stdout 自定义脚本, 可改 clone_branch, 失败即中止)
- reclone.yaml: 保存多条 clone 命令; post_exec_script(status,name) 实现自定义通知/healthchecks; token_cmd 从 1Password/pass 等取 token
- ghorg reclone-server: HTTP 触发 reclone; ghorg reclone-cron: 定时
- ghorg stats: CSV 记录每次 clone (新克隆数/拉取数/目录大小/新 commit 数/错误数/耗时/版本), 便于审计趋势
- 并发 clone (--concurrency 默认 25); pprof/trace 性能剖析
- 配置优先级: CLI flags > conf.yaml; 支持多配置文件 --config


## 4. GitLab Repository Mirroring (docs.gitlab.com/user/project/repository/mirror/)
- Push / Pull / Bidirectional; Pull 需 Premium/Ultimate
- 同步内容: branches, tags, commits 自动同步
- 安全: SSH host key detect/手动录入, 连接前校验至少一个指纹 (防 MITM); 认证为 SSH 公钥(镜像级生成)或用户名+密码/token
- 分支范围: Only mirror protected branches; Mirror specific branches (RE2 正则, Premium+)
- Keep divergent refs: 防止 force-push 覆盖已分叉 refs
- 调度: 自动更新 + Update now 手动; GitLab.com 5 分钟限流; Self-Managed 管理员设 pull interval
- 可观测: 新 branches/tags/commits 进 activity feed; Silent Mode 全面禁用 push/pull
- 不支持: SCP 风格 URL, dumb HTTP, 不同 object format (SHA-1↔SHA-256)

## 5. Gitea / Forgejo Repo Mirroring
来源: docs.gitea.com/usage/repo-mirror + forgejo.org/docs/latest/user/repo-mirror
- Pull mirror: 仅在 New Migration 建仓时勾选 "This repository will be a mirror"; 创建后不可转换
- Push mirror: Settings→Repository→Mirror Settings; 默认 force push 覆盖远端
- Gitea 1.18+ / Forgejo: "Sync when new commits are pushed" 事件驱动, 可关闭周期同步
- 周期: Synchronize Now 手动强制; cron.update_mirrors SCHEDULE @every 10m, PULL_LIMIT/PUSH_LIMIT=50, RUN_AT_START
- mirror.ENABLED / DISABLE_NEW_PULL / DISABLE_NEW_PUSH; 超时 MIRROR: 300s
- Forgejo 独有: Push mirror 分支过滤(逗号分隔+glob, 如 main, feature/*; 无过滤则 git push --mirror)
- Forgejo 独有: SSH 认证 push mirror — 生成 Ed25519 密钥对, 公钥作 deploy key; LFS over SSH 不支持→LFS 不镜像
- 错误时展示可读错误信息; 不支持 SSH push mirror 的 Gitea 用 post-receive hook 变通

## 6. GitHub / GHE
来源: docs.github.com/en/repositories/archiving-a-github-repository/backing-up-a-repository
- 无原生 pull/push mirror (与 GitLab/Gitea 不同); 官方指引 git clone --mirror + git lfs fetch --all + 归档
- wiki 是独立 git 仓库可一并备份
- Migration archive (REST org migrations): 含部分元数据, 不含 LFS/discussions/packages, 无官方恢复路径, 仅归档
- Marketplace 第三方备份工具; 备份缺口正是 GitFerry 机会

## 7. Enteee/git-sync-mirror (容器)
- SRC/DST 环境变量驱动, HTTPS token 认证
- PRUNE: 源删分支/标签则目标同步删除; TWO_WAY 双向; GIT_FORCE_PUSH
- IGNORE_REFS_PATTERN (默认 refs/pull) 跳过 PR refs; SLEEP_TIME 周期
- 每侧独立 HTTP proxy; TLS-TOFU; HTTP_ALLOW_TOKENS_INSECURE 显式危险开关

## 8. haltman-io/agmh (Python)
- 本地 bare mirror 备份 + 跨 forge 推送 (GitHub/GitLab/Forgejo/Bitbucket/SourceHut)
- 可恢复状态 .agmh/state.json + .agmh/logs/; polling/watch 模式
- Token 仅环境变量, 明确拒绝写入配置/日志; TUI 可选
- 明确不迁移 issues/PR/权限等元数据 (边界清晰)

---
## 交叉结论 (面向 GitFerry)
GitFerry 现有: 多平台同步、cron、webhook、镜像中心发布、AI 助手。
缺口机会: 备份形态(mirror/zip/keep)、过滤链、事件+周期双驱动、失败重试与补偿、
成功/失败分路通知、Prometheus 指标、SSH host key/密钥治理、分支过滤与 prune、
keep divergent refs、LFS、wiki/issues 附带备份、stats 审计、secrets manager 集成。
