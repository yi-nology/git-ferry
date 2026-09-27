package git_sync

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// ExportIssuesReq 导出仓库 issues(备份/迁移用)。
type ExportIssuesReq struct {
	RepoKey string `json:"repo_key" form:"repo_key" query:"repo_key"`
	// State open/closed/all
	State string `json:"state" form:"state" query:"state"`
	// Max 最多导出条数,默认 500,上限 2000
	Max int `json:"max" form:"max" query:"max"`
	// Format json | csv
	Format string `json:"format" form:"format" query:"format"`
	// WithComments 同时导出评论(仅 json)
	WithComments bool `json:"with_comments" form:"with_comments" query:"with_comments"`
}

// ExportIssues GET /api/v1/ops/issues-export
// 借鉴 gickup 的 issues 备份:导出仓库 issue 列表,可选带评论。
func ExportIssues(ctx context.Context, c *app.RequestContext) {
	var req ExportIssuesReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}
	max := req.Max
	if max <= 0 {
		max = 500
	}
	if max > 2000 {
		max = 2000
	}

	svc, ok := requireSyncService(c)
	if !ok {
		return
	}
	repo, err := svc.GetRepo(ctx, req.RepoKey)
	if err != nil || repo == nil {
		response.NotFound(c, "repo not found")
		return
	}
	plat, err := svc.GetPlatformByID(ctx, repo.PlatformID)
	if err != nil || plat == nil {
		response.NotFound(c, "platform not found")
		return
	}

	prov, err := newIssueProvider(plat, repo.AccessToken)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	issues, err := listIssues(ctx, prov, repo, req.State, max)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if req.WithComments && req.Format != "csv" {
		if im, ok := prov.(sdkprov.IssueManager); ok {
			for _, iss := range issues {
				comments, cerr := im.ListIssueComments(ctx, repo.PlatformOwner, repo.PlatformRepo, iss.Number)
				if cerr == nil {
					iss.CommentList = comments
				}
			}
		}
	}

	if req.Format == "csv" {
		csv := buildIssuesCSV(issues)
		c.Data(consts.StatusOK, "text/csv; charset=utf-8", []byte(csv))
		return
	}
	response.Success(c, map[string]any{
		"repo_key":    req.RepoKey,
		"count":       len(issues),
		"exported_at": time.Now().Format(time.RFC3339),
		"items":       issues,
	})
}

// issueRow 导出用 issue 视图。
type issueRow struct {
	Number      string                  `json:"number"`
	Title       string                  `json:"title"`
	State       string                  `json:"state"`
	Author      string                  `json:"author,omitempty"`
	Labels      []string                `json:"labels,omitempty"`
	Assignees   []string                `json:"assignees,omitempty"`
	Body        string                  `json:"body,omitempty"`
	WebURL      string                  `json:"web_url,omitempty"`
	CreatedAt   string                  `json:"created_at"`
	UpdatedAt   string                  `json:"updated_at"`
	CommentList []*sdkprov.IssueComment `json:"comments,omitempty"`
}

func newIssueProvider(plat *corebridge.Platform, repoToken string) (sdkprov.Provider, error) {
	token := repoToken
	if token == "" {
		token = plat.AccessToken
	}
	cfg := sdkprov.Config{
		Platform: sdkprov.Platform(plat.Type),
		BaseURL:  plat.APIURL,
		Token:    token,
		SkipTLS:  plat.SkipTLSVerify,
	}
	p, err := sdkprov.NewProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}
	return p, nil
}

func listIssues(ctx context.Context, p sdkprov.Provider, repo *corebridge.Repo, state string, max int) ([]*issueRow, error) {
	// 平台不一定实现 IssueManager
	im, ok := p.(sdkprov.IssueManager)
	if !ok {
		return nil, fmt.Errorf("platform %s does not support issue export", repo.Platform)
	}
	// 空 state 交平台默认(通常 open+closed 或 open)
	st := sdkprov.IssueState(state)
	out := []*issueRow{}
	const perPage = 50
	for page := 1; len(out) < max; page++ {
		batch, _, err := im.ListIssues(ctx, sdkprov.ListIssuesOptions{
			Owner:   repo.PlatformOwner,
			Repo:    repo.PlatformRepo,
			State:   st,
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			return out, err
		}
		for _, iss := range batch {
			row := toIssueRow(iss)
			out = append(out, row)
			if len(out) >= max {
				return out, nil
			}
		}
		if len(batch) < perPage {
			break
		}
	}
	return out, nil
}

func toIssueRow(iss *sdkprov.Issue) *issueRow {
	author := ""
	if iss.Author != nil {
		author = iss.Author.Username
	}
	return &issueRow{
		Number:    iss.Number,
		Title:     iss.Title,
		State:     string(iss.State),
		Author:    author,
		Labels:    iss.Labels,
		Assignees: iss.Assignees,
		Body:      iss.Body,
		WebURL:    iss.WebURL,
		CreatedAt: iss.CreatedAt.Format(time.RFC3339),
		UpdatedAt: iss.UpdatedAt.Format(time.RFC3339),
	}
}

func buildIssuesCSV(issues []*issueRow) string {
	var b []byte
	b = append(b, []byte("number,state,author,title,labels,created_at\n")...)
	for _, iss := range issues {
		labels := ""
		for i, l := range iss.Labels {
			if i > 0 {
				labels += "|"
			}
			labels += l
		}
		b = append(b, []byte(fmt.Sprintf("%s,%s,%s,%s,%s,%s\n",
			csvEscape(iss.Number), csvEscape(iss.State), csvEscape(iss.Author),
			csvEscape(iss.Title), csvEscape(labels), iss.CreatedAt))...)
	}
	return string(b)
}
