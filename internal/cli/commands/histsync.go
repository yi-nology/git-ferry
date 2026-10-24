package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

// ---------- histsync(历史同步) ----------
//
// 两个各自有完整历史的仓库之间按**内容差异**同步:
// 目标树的既有提交原样保留(hash 不变),新内容以新提交追加。
//
// 与镜像中心(/mirror)的身份镜像模式并存,语义不同。
// 命令组名用 `histsync` 而非 `history`:`history` 已被「同步执行历史」占用。
//
// **通道优先**:配置存在通道里(DB 持久化),执行按 channel id 进行。
// 标准流程:channel-create → target-create(留底在前)→ plan → resolve
//          → 人工/AI 裁决冲突 → apply → push。

// newHistsyncCmd 历史同步命令组。
func newHistsyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "histsync",
		Short: "历史同步(双仓内容差异发布)",
		Long: "历史同步:把内部仓与公开仓之间的内容差异整理成计划,叠加到公开仓上。\n" +
			"公开仓的既有提交原样保留(hash 不变),新内容以新提交追加。\n" +
			"配置存在「通道」里(DB 持久化),执行按 channel id 进行。\n" +
			"标准流程:channel-create → target-create(留底在前)→ plan → resolve\n" +
			"          → 人工/AI 裁决冲突 → apply → push。",
	}
	cmd.AddCommand(
		// 通道
		hs("+channel-list", "列出历史同步通道", histsyncChannelList, hsFlagChannelID),
		hs("+channel-create", "新建历史同步通道(写)", histsyncChannelCreate, hsFlagChannelBody),
		hs("+channel-update", "更新历史同步通道(写)", histsyncChannelUpdate, hsFlags(hsFlagChannelID, hsFlagChannelBody)),
		hs("+channel-delete", "删除历史同步通道(危险)", histsyncChannelDelete, hsFlagChannelID),
		// 推送目标
		hs("+target-list", "列出通道的推送目标(顺序=推送优先级)", histsyncTargetList, hsFlagChannelID),
		hsCred(hs("+target-create", "新建推送目标(凭据加密落库)", histsyncTargetCreate, hsFlags(hsFlagChannelID, hsFlagTargetBody, hsFlagCred))),
		hs("+target-delete", "删除推送目标(危险)", histsyncTargetDelete, hsFlagTargetID),
		hsCred(hs("+target-rotate", "轮换推送目标凭据(只换密钥,不动 URL)", histsyncTargetRotate, hsFlags(hsFlagTargetID, hsFlagCred))),
		// 执行
		hs("+plan", "按通道生成同步计划(纯读)", histsyncPlan, hsFlagChannelID),
		hs("+resolve", "按通道生成计划并计算 hold 的自动解法", histsyncResolve, hsFlagChannelID),
		hs("+preview", "预检:摊开影响面(不推送)", histsyncPreview, hsFlagChannelID),
		hs("+sync", "一次命令:预检→移植→推送(有阻塞项则停下)", histsyncSync, hsFlags(hsFlagChannelID, hsFlagForce)),
		hs("+apply", "按通道把计划落到目标树(本地,不推送)", histsyncApply, hsFlagChannelID),
		hs("+push", "按通道推送移植产物(危险)", histsyncPush, hsFlags(hsFlagChannelID, hsFlagForcePush)),
		// 执行记录与回滚
		hs("+runs", "列出通道的执行记录", histsyncRuns, hsFlagChannelID),
		hs("+run-detail", "执行记录详情(审计材料)", histsyncRunDetail, hsFlagRunID),
		hs("+channel-audit", "校验通道内执行记录的哈希链", histsyncChannelAudit, hsFlagChannelID),
		// 裁决覆盖:人工/AI 对 hold 文件拍板
		hs("+override-list", "列出通道的裁决", histsyncOverrideList, hsFlagChannelID),
		hs("+override-set", "写入裁决(人工/AI 拍板)", histsyncOverrideSet, hsFlags(hsFlagChannelID, hsFlagOverride)),
		hs("+override-delete", "撤销裁决(回到自动判定)", histsyncOverrideDelete, hsFlagOverrideID),
		hs("+rollback", "按执行记录把目标树回滚到指定 ref(危险)", histsyncRollback, hsFlags(hsFlagRunID, hsFlagRef)),
	)
	return cmd
}

// hs 声明一个历史同步子命令,并挂上常用 flags。
// 各命令**真正会读**的 flag 集。
//
// 允许集 = 这个命令的语义范围内、会被真正消费的参数。
// 不在集合里的 flag 一旦被显式设置,命令直接报错退出(见 rejectUnusedFlags)。
var (
	hsFlagChannelID   = []string{"channel-id"}
	hsFlagRunID       = []string{"run-id"}
	hsFlagTargetID    = []string{"target-id"}
	hsFlagOverrideID  = []string{"override-id"}
	hsFlagChannelBody = []string{
		"name", "source-dir", "target-dir", "source-repo-key", "target-repo-key",
		"module-from", "module-to", "host-from", "host-to",
		"path-rules", "scrub-rules", "target-ref",
		"author-name", "author-email", "disabled", "enable",
	}
	hsFlagTargetBody = []string{"name", "url"}
	hsFlagForce      = []string{"force", "force-push"}
	hsFlagForcePush  = []string{"force-push"}
	hsFlagCred       = []string{"cred-type", "credential", "credential-file", "username", "expires-at"}
	hsFlagOverride   = []string{"path", "verdict", "content-file", "reason", "decided-by"}
	hsFlagRef        = []string{"ref"}
)

// hsFlags 组合若干组,得到该命令允许的 flag 集。
func hsFlags(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// rejectUnusedFlags 拒绝「本命令不认、却被显式设置」的 flag。
//
// **为什么要有这道闸**:静默丢弃的参数比报错危险得多 ——
// 它让人以为操作生效了(而服务端根本没收到那个参数)。
// 上一轮只给 `+push --force` 单独补了报错,同一个病在另外 18 个命令上还在。
func rejectUnusedFlags(cmd *cobra.Command, allowed []string) error {
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[a] = true
	}
	var bad []string
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if !ok[f.Name] {
			bad = append(bad, "--"+f.Name)
		}
	})
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	// 「--force」在 push 上是 sync 的语义(忽略阻塞项)—— 指路比只说「不接受」有用。
	if len(bad) == 1 && bad[0] == "--force" && ok["force-push"] {
		return fmt.Errorf("%s 不接受 --force(它的语义是 sync 的「忽略阻塞项继续」);"+
			"覆盖远端已有提交请用 --force-push", cmd.Name())
	}
	return fmt.Errorf("%s 不接受这些参数:%s(传了也不会生效,已拒绝;本命令支持的参数见 --help)",
		cmd.Name(), strings.Join(bad, ", "))
}

// hsCred 给命令挂上**凭据相关**的 flag。
//
// 凭据 flag 不该挂在每个命令上:以前 `+plan --credential SECRET` 会被接受
// 然后丢弃 —— secret 照样进了 shell 历史,用户却以为设上了。
// 只有真正能写凭据的命令(target-create / target-rotate)才有这些 flag。
func hsCred(cmd *cobra.Command) *cobra.Command {
	f := cmd.Flags()
	f.String("cred-type", "", "凭据类型:none|token|ssh_key")
	f.String("credential", "", "凭据明文(优先用 --credential-file,避免出现在命令历史里)")
	f.String("credential-file", "", "从文件读凭据(推荐)")
	f.String("username", "", "HTTPS Basic 用户名(token 类凭据用)")
	f.String("expires-at", "", "凭据到期时间(RFC3339)")
	return cmd
}

func hs(name, short string, fn runFunc, allowed []string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Run:   runWith(fn),
		// **无效参数必须响亮失败,不能静默丢弃**。
		//
		// CLI 过去把 29 个 flag 撒在每个子命令上,于是
		// `+plan --force-push`、`+channel-list --verdict merged` 这类
		// 无效参数**被静默接受**(实测 21 个命令共 578 处静默丢弃)——
		// 用户以为设上了,服务端根本没收到。
		// 静默丢弃比报错危险:它让人以为操作生效了。
		PreRunE: func(c *cobra.Command, args []string) error {
			return rejectUnusedFlags(c, allowed)
		},
	}
	f := cmd.Flags()
	f.String("channel-id", "", "历史同步通道 ID")
	f.String("run-id", "", "历史同步执行记录 ID")
	f.String("target-id", "", "推送目标 ID")
	f.String("name", "", "通道名 / 推送目标名")
	f.String("source-dir", "", "内部源仓本地路径(与 --source-repo-key 二选一)")
	f.String("target-dir", "", "公开目标仓本地路径(与 --target-repo-key 二选一)")
	f.String("source-repo-key", "", "内部源仓 key(取自仓库表)")
	f.String("target-repo-key", "", "公开目标仓 key(取自仓库表)")
	f.String("module-from", "", "源 module path(身份改写)")
	f.String("module-to", "", "目标 module path(身份改写)")
	f.String("host-from", "", "源主机名(身份改写,如 git.enjoye.top)")
	f.String("host-to", "", "目标主机名(身份改写,如 github.com)")
	f.String("path-rules", "", "路径规则,逗号分隔 from=to;to 为空表示排除")
	f.String("scrub-rules", "", "痕迹规则,逗号分隔 name=regex[:auto|mandatory]")
	f.String("target-ref", "", "目标仓分支(默认 main)")
	f.String("url", "", "推送目标地址(https/ssh/本地路径)")
	f.String("ref", "", "回滚目标 ref(通常是某次同步前的 HEAD commit)")
	f.String("override-id", "", "裁决 ID")
	f.String("path", "", "文件路径(裁决定位键)")
	f.String("verdict", "", "裁决:keep_target|take_source|merged")
	f.String("content-file", "", "裁决后的内容文件(仅 merged 用)")
	f.String("reason", "", "裁决理由(审计用)")
	f.String("decided-by", "", "裁决者标识(人工或 AI)")
	f.Bool("force", false, "有阻塞项时也继续(仍会推送到远端)")
	f.Bool("force-push", false, "强制推送:远端与基线已分叉也照样覆盖(会丢掉远端已有的提交)")
	f.Bool("disabled", false, "创建/更新时禁用该通道(默认启用)")
	f.Bool("enable", false, "重新启用该通道(与 --disabled 互斥)")
	f.String("author-name", "", "目标侧新提交的作者名(默认 git-ferry;不继承内网作者)")
	f.String("author-email", "", "目标侧新提交的作者邮箱(默认 git-ferry@users.noreply.github.com)")
	return cmd
}

// histsyncChannelBody 组装通道的创建/更新请求体。
func histsyncChannelBody(cmd *cobra.Command) (map[string]any, error) {
	body := map[string]any{}
	for flag, key := range map[string]string{
		"name": "name", "source-dir": "source_dir", "target-dir": "target_dir",
		"source-repo-key": "source_repo_key", "target-repo-key": "target_repo_key",
		"module-from": "module_source", "module-to": "module_target",
		"host-from": "host_source", "host-to": "host_target",
		"target-ref": "target_branch",
	} {
		if v := flagStr(cmd, flag); v != "" {
			body[key] = v
		}
	}
	if flagBool(cmd, "disabled") && flagBool(cmd, "enable") {
		return nil, fmt.Errorf("--disabled 与 --enable 不能同时给")
	}
	if flagBool(cmd, "disabled") {
		body["enabled"] = false
	}
	if flagBool(cmd, "enable") {
		body["enabled"] = true
	}
	if v := flagStr(cmd, "author-name"); v != "" {
		body["author_name"] = v
	}
	if v := flagStr(cmd, "author-email"); v != "" {
		body["author_email"] = v
	}
	if v := flagStr(cmd, "path-rules"); v != "" {
		var pathRules []map[string]any
		for _, item := range strings.Split(v, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			from, to, _ := strings.Cut(item, "=")
			pathRules = append(pathRules, map[string]any{
				"from": strings.TrimSpace(from), "to": strings.TrimSpace(to),
			})
		}
		body["path_rules"] = pathRules
	}
	if v := flagStr(cmd, "scrub-rules"); v != "" {
		var scrubRules []map[string]any
		for _, item := range strings.Split(v, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.SplitN(item, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("痕迹规则格式应为 name=regex[:auto|mandatory]: %q", item)
			}
			pattern, severity, _ := strings.Cut(parts[1], ":")
			if severity == "" {
				severity = "auto"
			}
			scrubRules = append(scrubRules, map[string]any{
				"name": strings.TrimSpace(parts[0]), "pattern": pattern, "severity": severity,
			})
		}
		body["scrub_rules"] = scrubRules
	}
	return body, nil
}

// histsyncCredential 读凭据:优先文件,其次命令行。
func histsyncCredential(cmd *cobra.Command) (string, error) {
	if path := flagStr(cmd, "credential-file"); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("读取凭据文件失败: %w", err)
		}
		return strings.TrimSpace(string(raw)), nil
	}
	return flagStr(cmd, "credential"), nil
}

// ---------- 通道 ----------

func histsyncChannelList(c *client.Client, _ *config.Config, _ *cobra.Command, _ []string) (*output.Envelope, error) {
	return c.Get("/api/v1/histsync/channels", nil)
}

func histsyncChannelCreate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if denied := confirmDanger("新建历史同步通道(配好推送目标后即可把内容发布到远端)"); denied != nil {
		return denied, nil
	}
	body, err := histsyncChannelBody(cmd)
	if err != nil {
		return nil, err
	}
	if name, _ := body["name"].(string); name == "" {
		return nil, fmt.Errorf("缺少必填参数 --name")
	}
	return c.Post("/api/v1/histsync/channels", body)
}

func histsyncChannelUpdate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if denied := confirmDanger("更新历史同步通道(改推送目标 = 改内容发到哪里)"); denied != nil {
		return denied, nil
	}
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	body, err := histsyncChannelBody(cmd)
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/update", body)
}

func histsyncChannelDelete(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("删除历史同步通道 " + id); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/delete", nil)
}

// ---------- 推送目标 ----------

func histsyncTargetList(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/histsync/channels/"+id+"/targets", nil)
}

func histsyncTargetCreate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if denied := confirmDanger("新建推送目标(内容会被推到���个地址)"); denied != nil {
		return denied, nil
	}
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	credential, err := histsyncCredential(cmd)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"name":       flagStr(cmd, "name"),
		"repo_url":   flagStr(cmd, "url"),
		"cred_type":  flagOr(cmd, "cred-type", "none"),
		"credential": credential,
		"username":   flagStr(cmd, "username"),
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/targets", body)
}

func histsyncTargetDelete(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "target-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("删除推送目标 " + id); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/targets/"+id+"/delete", nil)
}

// ---------- 执行 ----------

func histsyncPlan(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/plan", nil)
}

func histsyncResolve(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/resolve", nil)
}

func histsyncPreview(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/preview", nil)
}

func histsyncApply(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	if denied := confirmDanger("按通道把计划落到目标树(产生新提交)"); denied != nil {
		return denied, nil
	}
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/apply", nil)
}

func histsyncPush(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("按通道 " + id + " 推送历史同步结果(先留底后发布)"); denied != nil {
		return denied, nil
	}
	body := map[string]any{}
	if flagBool(cmd, "force-push") {
		// **字段名与服务端一致**(`force_push`)。
		// 原来 CLI 把 `--force-push` 翻译成 body 的 `force` ——
		// 而 `/push` 的 `force` 与 `/sync` 的 `force` 语义相反,
		// 这层手工转译迟早对不上(传错键还会被服务端静默丢弃)。
		body["force_push"] = true
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/push", body)
}

// ---------- 执行记录与回滚 ----------

func histsyncRuns(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/histsync/channels/"+id+"/runs", nil)
}

func histsyncChannelAudit(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/histsync/channels/"+id+"/audit", nil)
}

func histsyncRunDetail(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "run-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/histsync/runs/"+id, nil)
}

func histsyncRollback(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "run-id")
	if err != nil {
		return nil, err
	}
	ref, err := requireFlag(cmd, "ref")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("按执行记录 " + id + " 把目标树回滚到 " + ref); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/runs/"+id+"/rollback", map[string]any{"ref": ref})
}

// ---------- 裁决覆盖 ----------

func histsyncOverrideList(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	return c.Get("/api/v1/histsync/channels/"+id+"/overrides", nil)
}

func histsyncOverrideSet(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	path, err := requireFlag(cmd, "path")
	if err != nil {
		return nil, err
	}
	verdict, err := requireFlag(cmd, "verdict")
	if err != nil {
		return nil, err
	}
	// **写裁决直接决定发布内容** —— 必须有人工确认闸
	// (MCP 侧有 dangerGuard,CLI 侧原先没有 → 两条路径口径不一致)。
	if denied := confirmDanger("写入裁决 " + path + "(" + verdict + "):这会改变实际发布的内容"); denied != nil {
		return denied, nil
	}
	content := ""
	if p := flagStr(cmd, "content-file"); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("读取裁决内容文件失败: %w", err)
		}
		content = string(raw)
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/overrides", map[string]any{
		"path":       path,
		"verdict":    verdict,
		"content":    content,
		"reason":     flagStr(cmd, "reason"),
		"decided_by": flagStr(cmd, "decided-by"),
	})
}

func histsyncOverrideDelete(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "override-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("撤销裁决 " + id + "(回到自动判定)"); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/overrides/"+id+"/delete", nil)
}

// histsyncSync 一次跑完预检→移植→推送。
//
// 有阻塞项(门禁不过 / 有待裁决冲突 / 需强推)时服务端会**停下**并报告,
// 不会擅自往下走。推送本身是对外可见动作,仍需显式确认。
func histsyncSync(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "channel-id")
	if err != nil {
		return nil, err
	}
	if denied := confirmDanger("按通道 " + id + " 同步:移植并推送到远端(先留底后发布)"); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/channels/"+id+"/sync", map[string]any{
		"force":      flagBool(cmd, "force"),
		"force_push": flagBool(cmd, "force-push"),
	})
}

// histsyncTargetRotate 轮换推送目标的凭据(只换密钥,不动 URL)。
func histsyncTargetRotate(c *client.Client, _ *config.Config, cmd *cobra.Command, _ []string) (*output.Envelope, error) {
	id, err := requireFlag(cmd, "target-id")
	if err != nil {
		return nil, err
	}
	credential, err := histsyncCredential(cmd)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"cred_type":  flagOr(cmd, "cred-type", "none"),
		"credential": credential,
		"username":   flagStr(cmd, "username"),
		"expires_at": flagStr(cmd, "expires-at"),
	}
	if denied := confirmDanger("轮换推送目标 " + id + " 的凭据"); denied != nil {
		return denied, nil
	}
	return c.Post("/api/v1/histsync/targets/"+id+"/rotate", body)
}
