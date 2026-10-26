package release

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changelogPath 仓库根的 CHANGELOG(相对本包)。
const changelogPath = "../../CHANGELOG.md"

// breakingHeading 破坏性变更小节的标题(作者显式标记用)。
//
// **必须按"标题行"匹配**,不能用 strings.Contains ——
// 正文里提到"`### 破坏性变更`"这种字面量(比如门禁自己的说明)会被
// Contains 误判成"已标记"⇒ 门禁静默失效。
var breakingHeadingRe = regexp.MustCompile(`(?m)^###[ \t]+破坏性变更[ \t]*$`)

// migrationHeading 迁移指引小节的标题(major 版本必须给)。
var migrationHeadingRe = regexp.MustCompile(`(?m)^###[ \t]+迁移指引[ \t]*$`)

// section 是一个 CHANGELOG 版本段。
type section struct {
	version string
	major   int
	minor   int
	patch   int
	body    string
}

// isMajor 是否是 major 版本(x.0.0)。
func (s section) isMajor() bool { return s.minor == 0 && s.patch == 0 }

// isPre1 是否是 1.0 之前的版本(0.x.y:按 semver 任何东西都可能变,不受门禁约束)。
func (s section) isPre1() bool { return s.major == 0 }

// headerRe 匹配 `## [vX.Y.Z] - 日期` 形式的版本头。
var headerRe = regexp.MustCompile(`^## \[v?(\d+)\.(\d+)\.(\d+)\]`)

// parseSections 解析 CHANGELOG 的版本段(按文件里的顺序,最新的在前)。
func parseSections(t *testing.T) []section {
	t.Helper()
	raw, err := os.ReadFile(changelogPath)
	require.NoError(t, err)
	lines := strings.Split(string(raw), "\n")

	var out []section
	var cur *section
	for _, ln := range lines {
		if m := headerRe.FindStringSubmatch(ln); m != nil {
			maj, _ := strconv.Atoi(m[1])
			min, _ := strconv.Atoi(m[2])
			pat, _ := strconv.Atoi(m[3])
			out = append(out, section{
				version: fmt.Sprintf("v%s.%s.%s", m[1], m[2], m[3]),
				major:   maj, minor: min, patch: pat,
			})
			cur = &out[len(out)-1]
			continue
		}
		if cur != nil {
			cur.body += ln + "\n"
		}
	}
	require.NotEmpty(t, out, "CHANGELOG 里没解析出版本段(格式变了?)")
	return out
}

// breakingWords 高精度破坏性措辞表:命中即要求作者显式标记。
//
// 刻意**窄**:宁可漏判也不误判 —— 误判会把正常的 minor 修复卡住,
// 那比漏判更招人烦。
var breakingWords = []string{
	"破坏性", "breaking", "向后不兼容", "不兼容",
	"不再", "移除", "硬失败", "退出码", "语义变更", "字段名",
}

// TestChangelog_破坏性变更必须走major 最新版本段若显式标记了破坏性变更,
// 该版本**必须**是 major;若出现了破坏性措辞却没标记,必须标记或改写。
//
// 这条用例是**反向验证**:把 v2.0.0 改成 v1.39.0(非 major),本用例会变红。
func TestChangelog_破坏性变更必须走major(t *testing.T) {
	latest := parseSections(t)[0]
	marked := breakingHeadingRe.MatchString(latest.body)

	if marked {
		assert.True(t, latest.isMajor(),
			"%s 段里有 %q 标记 ⇒ 它必须发成 major 版本(当前 v%d.%d.%d)",
			latest.version, "### 破坏性变更", latest.major, latest.minor, latest.patch)
		return
	}
	if latest.isPre1() {
		return // 0.x 阶段按 semver 随时可破坏,不受门禁约束
	}
	var hits []string
	for _, w := range breakingWords {
		if strings.Contains(latest.body, w) {
			hits = append(hits, w)
		}
	}
	assert.Empty(t, hits,
		"%s 段出现破坏性措辞 %v 但没有 %q 标记:要么把它归到破坏性变更(并升 major),要么改写措辞",
		latest.version, hits, "### 破坏性变更")
}

// TestChangelog_major版本必须必须有迁移指引 major 版本必须给出迁移指引。
//
// 破坏性变更如果没有迁移说明,升级的人只能自己 diff —— 那不叫 major 版本。
func TestChangelog_major版本必须有迁移指引(t *testing.T) {
	latest := parseSections(t)[0]
	if !latest.isMajor() || latest.isPre1() {
		return
	}
	assert.True(t, migrationHeadingRe.MatchString(latest.body),
		"%s 是 major 版本,必须在段内给出 %q 小节", latest.version, "### 迁移指引")
}

// openapiPath 仓库里的 OpenAPI spec(相对本包)。
const openapiPath = "../../docs/openapi.json"

// TestOpenAPI_删除公开契约必须走 major 公开契约里删掉了 path 或请求参数,
// 就是破坏性变更 ⇒ 必须 major。
//
// 机械检测,不依赖作者自觉:CHANGELOG 的措辞扫描只能覆盖"作者写出来了"的部分。
func TestOpenAPI_删除公开契约必须走major(t *testing.T) {
	latest := parseSections(t)[0]
	if latest.isPre1() {
		return
	}
	prevTag := previousReleaseTag(t)
	if prevTag == "" {
		t.Skipf("找不到上一个发布 tag(浅克隆?),本门禁未执行 —— " +
			"**这不是「没有破坏性变更」的证明**")
	}
	prevRaw, err := gitShow(t, prevTag+":"+filepath.ToSlash(filepath.Join("..", "..", "docs", "openapi.json")))
	if err != nil {
		t.Skipf("取不到 %s 的 openapi.json(%v),本门禁未执行 —— "+
			"**这不是「没有破坏性变更」的证明**", prevTag, err)
	}
	curRaw, err := os.ReadFile(openapiPath)
	require.NoError(t, err)

	removed := removedOpenAPIContract(prevRaw, curRaw)
	if len(removed) == 0 {
		return
	}
	assert.True(t, latest.isMajor(),
		"相对上一个发布版本(%s),公开契约被删除了 %v ⇒ 这是破坏性变更,当前版本 %s 必须是 major",
		prevTag, removed, latest.version)
}

// previousReleaseTag 找上一个发布版本的 tag(按 CHANGELOG 里的版本号排序)。
func previousReleaseTag(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "tag", "--list", "v*", "--sort=-creatordate").Output()
	if err != nil {
		return ""
	}
	latest := parseSections(t)[0].version
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		tag := strings.TrimSpace(ln)
		if tag == "" || tag == latest {
			continue
		}
		if _, err := exec.Command("git", "rev-parse", tag).CombinedOutput(); err != nil { //nolint:gosec // G204:测试内固定 git 子命令
			continue // 本地没有这个 tag
		}
		return tag
	}
	return ""
}

// gitShow 取某个 git 对象的内容。
func gitShow(t *testing.T, obj string) ([]byte, error) {
	t.Helper()
	out, err := exec.Command("git", "show", obj).Output() //nolint:gosec // G204:测试内固定 git 子命令
	if err != nil {
		return nil, err
	}
	return out, nil
}

// removedOpenAPIContract 比对两份 OpenAPI,列出被删除的 path 与请求参数。
func removedOpenAPIContract(prev, cur []byte) []string {
	var p, c map[string]any
	if json.Unmarshal(prev, &p) != nil || json.Unmarshal(cur, &c) != nil {
		return nil
	}
	pp, _ := p["paths"].(map[string]any)
	cp, _ := c["paths"].(map[string]any)
	if pp == nil || cp == nil {
		return nil
	}
	var removed []string
	for path, pv := range pp {
		cv, ok := cp[path]
		if !ok {
			removed = append(removed, "path "+path)
			continue
		}
		removed = append(removed, removedParams(path, pv, cv)...)
	}
	return removed
}

// removedParams 比对单个 path 下各 method 的请求参数,列出被删的。
func removedParams(path string, prev, cur any) []string {
	pm, ok1 := prev.(map[string]any)
	cm, ok2 := cur.(map[string]any)
	if !ok1 || !ok2 {
		return nil
	}
	var removed []string
	for method, pv := range pm {
		cv, ok := cm[method]
		if !ok {
			continue // method 整体没了,按 path 级别算
		}
		removed = append(removed, removedParamNames(path, method, pv, cv)...)
	}
	return removed
}

// removedParamNames 比对某个 method 的 query/path 参数名集合。
func removedParamNames(path, method string, prev, cur any) []string {
	pm, ok1 := prev.(map[string]any)
	cm, ok2 := cur.(map[string]any)
	if !ok1 || !ok2 {
		return nil
	}
	prevNames := paramNames(pm["parameters"])
	curNames := paramNames(cm["parameters"])
	var removed []string
	for n := range prevNames {
		if !curNames[n] {
			removed = append(removed, fmt.Sprintf("%s %s 的参数 %s", method, path, n))
		}
	}
	return removed
}

// paramNames 从 parameters 数组里取出 name 集合。
func paramNames(v any) map[string]bool {
	out := map[string]bool{}
	arr, ok := v.([]any)
	if !ok {
		return out
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		if n, ok := m["name"].(string); ok {
			out[n] = true
		}
	}
	return out
}
