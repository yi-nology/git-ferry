package cmdutil

// 全局 flags（root PersistentFlags 绑定）。
var (
	BaseURL string
	Format  string
	Debug   bool
	Yes     bool
)

// SuggestYes 危险操作未加 --yes 时的提示语。
const SuggestYes = "危险操作需确认；脚本场景可加 --yes"
