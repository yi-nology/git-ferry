package tools

import (
	"context"
	"fmt"
	"github.com/oekit/pkg/errors"
	"slices"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/yi-nology/git-ferry/internal/agent/engine/delegate"
	"github.com/yi-nology/git-ferry/internal/corebridge"
)

// Registry 工具注册表:构建 eino 工具集,并支持按名调用(确认执行路径复用)。
type Registry struct {
	svc    SyncService
	byName map[string]tool.InvokableTool
	// danger 危险工具集合(需用户确认 / 只读角色不可执行)。
	//
	// **唯一事实源 = 注册处(repo.go 里 addDanger 的标注)**。
	// 之前危险工具有两份清单(这里 + 各实现里的 dangerGuard 调用),
	// 靠人工保持一致:新增危险工具只写 dangerGuard 不改清单,
	// MCP 的只读过滤与 system prompt 的清单就会漏。
	// 现在只有一处标注,其它地方(MCP 过滤、提示词清单、权限策略)全部派生。
	danger map[string]bool
	mem    MemStore
	policy *Policy
	// delegator 外部 coding agent 委派执行器(可选;未装配时工具如实报「未配置」)。
	delegator delegate.Runner
	// tasks 委派任务登记表(异步派发 + 按 ID 取回)。
	tasks *delegateTasks
}

func NewRegistry(svc SyncService) *Registry {
	r := &Registry{
		svc:    svc,
		byName: map[string]tool.InvokableTool{},
		danger: map[string]bool{},
		tasks:  newDelegateTasks(),
	}
	r.registerAll()
	return r
}

// registerAll 注册全部工具。
//
// **危险标注(addDanger)只在这里出现** —— 全仓唯一事实源:
//   - MCP 的 tools/list 过滤与 tools/call 预检 → reg.IsDanger()
//   - system prompt 的「危险工具清单」 → tools.DangerToolNames()
//   - 权限策略的动作分级 → reg.guardByPolicy() 把 r.IsDanger() 传给 Policy.Classify()
//
// 各实现里的 r.dangerGuard(...) 调用是**执行路径**,与这里的标注必须一致 ——
// 由 danger_registry_test.go 静态核对(见该文件的说明)。
func (r *Registry) registerAll() {
	must := func(t tool.InvokableTool, err error) tool.InvokableTool {
		if err != nil {
			panic(err) // 仅构造期:输入 struct 定义错误属编程错误
		}
		return t
	}
	r.add(must(utils.InferTool("list_repos", "查询同步仓库列表,支持关键字过滤", r.listRepos)))
	r.add(must(utils.InferTool("get_repo", "查询单个仓库详情(含状态、平台、分支)", r.getRepo)))
	r.add(must(utils.InferTool("list_branches", "列出仓库在源平台的分支", r.listBranches)))
	r.add(must(utils.InferTool("list_tasks", "查询同步任务列表", r.listTasks)))
	r.add(must(utils.InferTool("get_task", "查询单个同步任务详情", r.getTask)))
	r.add(must(utils.InferTool("list_sync_history", "查询任务最近同步执行历史", r.listHistory)))
	r.add(must(utils.InferTool("get_run_detail", "查询单次同步执行的详情(含执行步骤与错误链)", r.getRunDetail)))
	r.add(must(utils.InferTool("list_platforms", "查询平台列表及状态", r.listPlatforms)))
	r.add(must(utils.InferTool("list_webhook_rules", "查询 Webhook 同步规则", r.listRules)))
	r.add(must(utils.InferTool("get_system_overview", "系统概览(仓库数/任务状态/健康检查)", r.getSystemOverview)))
	r.add(must(utils.InferTool("get_sync_health", "同步任务健康评分(等级/问题列表,便于诊断)", r.getSyncHealth)))
	r.add(must(utils.InferTool("get_repo_inventory", "仓库资产盘点:哪些仓库没有同步任务覆盖(孤儿仓库)", r.getRepoInventory)))
	r.add(must(utils.InferTool("get_ops_todo", "统一待办队列:健康偏低/孤儿仓库/RPO超标,按优先级", r.getOpsTodo)))
	r.add(must(utils.InferTool("get_rpo_report", "RPO/RTO 观测:各任务最近备份时长与超标标记", r.getRPOReport)))
	r.add(must(utils.InferTool("get_backup_integrity", "冷备完整性校验(Merkle Root 比对)", r.getBackupIntegrity)))
	r.add(must(utils.InferTool("get_drift_report", "漂移检测:本地与目标远端分支是否一致", r.getDriftReport)))
	r.add(must(utils.InferTool("get_audit_chain", "审计日志哈希链完整性校验", r.getAuditChain)))
	r.add(must(utils.InferTool("get_metadata_snapshots", "列出冷备元数据快照(只读);回灌用 CLI ops +metadata-restore", r.getMetadataSnapshots)))
	// 历史同步(histsync):双仓内容差异发布,**通道优先**。
	// 只读面:apply 会产生本地提交、push 是对外可见动作,都在 CLI 侧。
	r.add(must(utils.InferTool("list_histsync_channels",
		"列出历史同步通道(DB 持久化的双仓同步配置)", r.listHistoryChannels)))
	r.add(must(utils.InferTool("plan_histsync_channel",
		"按历史同步通道生成同步计划,只读不写", r.planHistoryChannel)))
	r.add(must(utils.InferTool("resolve_histsync_channel",
		"按历史同步通道计算自动解法,并返回待人工裁决的冲突卷宗", r.resolveHistoryChannel)))
	r.add(must(utils.InferTool("list_histsync_runs",
		"列出历史同步通道的执行记录(审计留痕与回滚锚点)", r.listHistoryRuns)))
	r.add(must(utils.InferTool("verify_histsync_channel_audit",
		"校验历史同步通道内执行记录的哈希链(篡改任一审计材料都能检出)", r.verifyHistoryChannelAudit)))
	r.add(must(utils.InferTool("preview_histsync_channel",
		"历史同步预检:摊开影响面(将产生的提交/门禁结果/推送是否需强推),不推送", r.previewHistoryChannel)))
	r.addDanger(must(utils.InferTool("sync_histsync_channel",
		"一次跑完历史同步:预检→移植→推送(危险操作,需用户确认;有阻塞项则停下)", r.syncHistoryChannel)))
	r.addDanger(must(utils.InferTool("rotate_history_target_credential",
		"轮换历史同步推送目标的凭据(危险操作,需用户确认;只换密钥不动 URL)", r.rotateHistoryTargetCredential)))
	r.add(must(utils.InferTool("list_histsync_overrides",
		"列出历史同步通道的裁决(人工/AI 对 hold 文件的拍板结果)", r.listHistoryOverrides)))
	// 写裁决会改变实际发布内容,需用户确认。
	r.addDanger(must(utils.InferTool("set_histsync_override",
		"写入历史同步裁决(危险操作,需用户确认):keep_target|take_source|merged", r.setHistoryOverride)))
	// 危险工具(需用户确认)
	r.addDanger(must(utils.InferTool("run_task", "立即执行一次同步任务(危险操作,需用户确认)", r.runTask)))
	r.addDanger(must(utils.InferTool("test_repo_connection", "测试仓库连通性(危险操作,需用户确认)", r.testRepoConnection)))
	r.addDanger(must(utils.InferTool("test_platform_connection", "测试平台连通性(危险操作,需用户确认)", r.testPlatformConnection)))
	r.addDanger(must(utils.InferTool("retry_sync_run", "重试指定同步任务(危险操作,需用户确认)", r.retrySyncRun)))
	r.addDanger(must(utils.InferTool("run_dr_drill", "灾备演练:从冷备恢复+fsck+refs 比对(危险操作,需用户确认)", r.runDRDrill)))
	r.add(must(utils.InferTool("diagnose_run", "诊断一次失败执行:错误分类→原因→建议动作", r.diagnoseRunIn)))
	// 记忆会进**每次会话的 system prompt** ⇒ 持久化提示注入的通道。
	// 写/删记忆按危险工具处理:需用户确认,且只读角色一律拒绝。
	r.addDanger(must(utils.InferTool("ai_code_delegate",
		"把一段编码任务派发给外部 coding agent 执行(危险操作,需用户确认:外部 agent 在服务端主机上运行;异步派发,返回任务 ID)", r.delegateIn)))
	// 取回委派结果:只读工具(只是查一张表)⇒ 不标危险。
	r.add(must(utils.InferTool("ai_delegate_result",
		"按任务 ID 取回 ai_code_delegate 派发的外部 agent 委派结果(仍在进行中则返回当前状态)", r.delegateResultIn)))
	r.addDanger(must(utils.InferTool("remember",
		"把用户偏好/故障模式/事实写入长期记忆(危险操作,需用户确认:记忆会进每次会话的提示)", r.rememberIn)))
	r.add(must(utils.InferTool("recall_memory", "按关键字检索已记住的偏好与经验", r.recallIn)))
	r.addDanger(must(utils.InferTool("forget_memory",
		"删除一条记忆(危险操作,需用户确认)", r.forgetIn)))
	r.add(must(utils.InferTool("plan_mode", "为复杂运维目标制定分步执行计划(先计划后执行)", r.planMode)))
	// 切换工具权限模式是**治理动作**:档位拨错会把写操作全拒掉。
	// 之前它既不在危险集合里、也没有角色闸门 —— 模型自己就能拨档。
	r.addDanger(must(utils.InferTool("set_permission_mode",
		"切换**本会话**的工具权限模式:default|readonly|audit(治理操作,需用户确认;只有 admin 能拨)", r.setModeIn)))
	r.add(must(utils.InferTool("deep_analyze", "深度分析失败根因(一次完成健康/失败/playbook 汇总)", r.deepAnalyze)))
	r.addDanger(must(utils.InferTool("rebuild_task", "清空任务工作目录并全量重建(危险,需确认)", r.rebuildIn)))
}

// add 注册一个普通(只读)工具。
func (r *Registry) add(t tool.InvokableTool) {
	r.register(t, false)
}

// addDanger 注册一个**危险工具**:需用户确认,且只读角色不可执行。
//
// 危险标注只写在注册处(与工具描述相邻),别处不再另抄清单。
func (r *Registry) addDanger(t tool.InvokableTool) {
	r.register(t, true)
}

func (r *Registry) register(t tool.InvokableTool, danger bool) {
	info, err := t.Info(context.Background())
	if err != nil {
		panic(err)
	}
	r.byName[info.Name] = t
	if danger {
		r.danger[info.Name] = true
	}
}

// IsDanger 该工具是否被标注为危险(需用户确认 / 只读角色不可执行)。
func (r *Registry) IsDanger(name string) bool { return r.danger[name] }

// DangerToolNames 全部危险工具名(排序)。
func (r *Registry) DangerToolNames() []string {
	out := make([]string, 0, len(r.danger))
	for n := range r.danger {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func (r *Registry) ByName(name string) tool.InvokableTool { return r.byName[name] }

// Names 返回全部工具名。
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	return out
}

// ===== 仓库 =====

type listReposInput struct {
	Keyword string `json:"keyword,omitempty" jsonschema:"按名称或克隆地址过滤的关键字,可选"`
	Page    int    `json:"page,omitempty" jsonschema:"页码,从 1 开始,默认 1"`
}

type repoSummary struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	Owner         string `json:"owner,omitempty"`
	Status        string `json:"status"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

func (r *Registry) listRepos(ctx context.Context, in listReposInput) (map[string]any, error) {
	// 始终传非 nil filter:core 的 ListWithFilter 不做空指针防护
	filter := &corebridge.RepoFilter{}
	if in.Keyword != "" {
		filter.Search = in.Keyword
	}
	list, total, err := r.svc.ListReposWithFilter(ctx, pageBounds(in.Page), pageLimit, filter)
	if err != nil {
		return nil, errors.Wrap(err, "查询仓库列表失败")
	}
	repos := []repoSummary{}
	for _, rp := range list {
		repos = append(repos, repoSummary{
			Key: rp.Key, Name: rp.Name, Platform: rp.Platform,
			Owner: rp.PlatformOwner, Status: rp.Status,
			CloneURL: rp.CloneURL, DefaultBranch: rp.DefaultBranch,
		})
	}
	return map[string]any{"total": total, "page": maxInt(in.Page, 1), "repos": repos}, nil
}

type keyInput struct {
	Key string `json:"key" jsonschema:"仓库 key"`
}

func (r *Registry) getRepo(ctx context.Context, in keyInput) (string, error) {
	rp, err := r.svc.GetRepo(ctx, in.Key)
	if err != nil || rp == nil {
		return msgJSON(map[string]any{"found": false, "message": fmt.Sprintf("未找到仓库 %q", in.Key)}), nil
	}
	return marshalJSON(repoSummary{
		Key: rp.Key, Name: rp.Name, Platform: rp.Platform,
		Owner: rp.PlatformOwner, Status: rp.Status,
		CloneURL: rp.CloneURL, DefaultBranch: rp.DefaultBranch,
	}), nil
}

type branchInput struct {
	Key string `json:"key" jsonschema:"仓库 key"`
}

func (r *Registry) listBranches(ctx context.Context, in branchInput) (string, error) {
	branches, err := r.svc.ListBranches(ctx, in.Key)
	if err != nil {
		return errJSON("查询分支失败", err), nil
	}
	if len(branches) > pageLimit {
		branches = branches[:pageLimit]
	}
	return marshalJSON(map[string]any{"branches": branches}), nil
}
