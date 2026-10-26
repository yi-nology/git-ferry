# 设计文档

> 存放**现行**设计文档：描述当前架构与需求的落地形态。
> 历史设计过程、实施记录、一次性计划文档不在此维护 —— 那些是一次性的过程材料，
> 不是现行架构（现行架构见 [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)）。

## 目录

| 文档 | 状态 | 说明 |
|------|------|------|
| [acp-external-agent-design.md](acp-external-agent-design.md) | 实施中 | 外部 AI coding 工具接入（ACP）：执行后端可插拔、内置 eino / acpx / 原生 stdio 三种后端、领域工具经本服务 `/mcp` 复用、权限与治理边界 |
| [history-sync-design.md](history-sync-design.md) | 现行 | 历史同步（双仓内容差异发布）：计划 → 冲突裁决 → 移植 → 推送 + 审计链 |
| [history-sync-baseline.md](history-sync-baseline.md) | 实测基线 | 历史同步的实测数据来源（可复现 + 回归门禁） |

## 约定

- **一份事实源**：架构与接口面的真源是 `ARCHITECTURE.md` 与代码，本目录只写「为什么这么做」与「怎么落地」。
- **文档要能被检索**：标题里带上领域词（历史同步 / ACP / 镜像…），别用「方案二」这种代号。
- **实测优先**：能测的数字不要靠估；基线单独成文并标明复现方式。
- **改一侧同步另一侧**：服务端行为变了（工具语义、接口、权限），对应的 `skills/` 与 README 同步更新。
