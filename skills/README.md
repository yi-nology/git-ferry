# GitFerry Agent Skills

面向 Claude Code / MiMo / Cursor 等 AI Agent 的结构化运维知识库。安装后，Agent 可直接通过 `gitferry` CLI 操作 GitFerry（Git 多平台同步 / 镜像 / 备份服务）。

## 安装

```bash
# 方式 1：skills CLI（推荐）
npx skills add ./skills -y -g

# 方式 2：复制到 Agent 的 skills 目录
cp -r skills/gitferry-* ~/.claude/skills/   # 路径按你的 Agent 调整
```

前置：已安装 `gitferry` CLI，并配置好基址与 API Key（见 [`gitferry-shared/SKILL.md`](gitferry-shared/SKILL.md)）。

## Skills 一览

| Skill | 说明 |
|-------|------|
| `gitferry-shared` | 认证、全局参数、Envelope 输出、安全规则。**必须先读** |
| `gitferry-repo` | 仓库列表 / 详情 / 分支 |
| `gitferry-task` | 同步任务 CRUD、手动触发、预览 |
| `gitferry-history` | 执行历史、失败诊断、重试 |
| `gitferry-ops` | 运维中心：健康评分、资产盘点、RPO、漂移、审计 |
| `gitferry-workflow` | 复合工作流：失败排查、灾备演练、平台接入 |

## 快速验证

```bash
gitferry auth status
gitferry ops +overview --format json
```

## 设计约定

- 所有命令输出统一 Envelope：`{ok, data, error, meta}`；**Agent 场景始终加 `--format json`**
- 危险操作（触发同步、连通性测试、灾备演练、重建）默认需确认，脚本场景用显式 `--yes`
- Token / API Key 只走环境变量或配置文件，永不写入输出
- Skills 与服务端 AI 工具（`internal/agent/tools/`）语义对齐；改一侧时同步另一侧
