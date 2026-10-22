package git_sync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/oekit/hertz/render"
	"github.com/oekit/pkg/ptrx"
	"github.com/yi-nology/git-ferry/biz/model/ops"
	"github.com/yi-nology/git-ferry/internal/corebridge"
	"github.com/yi-nology/git-ferry/internal/pkg/response"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// ExportIssues GET /api/v1/ops/issues-export
// 借鉴 gickup 的 issues 备份:导出仓库 issue 列表,可选带评论。
func ExportIssues(ctx context.Context, c *app.RequestContext) {
	var req ops.ExportIssuesReq
	if err := c.BindAndValidate(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.RepoKey == "" {
		response.BadRequest(c, "repo_key is required")
		return
	}
	max := int(req.Max)
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
	issues, err := listIssues(ctx, prov, repo, ptrx.Deref(req.State, ""), max)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if ptrx.Deref(req.WithComments, false) && ptrx.Deref(req.Format, "") != "csv" {
		if im, ok := prov.(sdkprov.IssueManager); ok {
			for _, iss := range issues {
				comments, cerr := im.ListIssueComments(ctx, repo.PlatformOwner, repo.PlatformRepo, iss.Number)
				if cerr == nil {
					iss.CommentList = comments
				}
			}
		}
	}

	if ptrx.Deref(req.Format, "") == "csv" {
		c.Render(consts.StatusOK, render.CSV{Rows: buildIssuesCSVRows(issues)})
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

func listIssues(ctx context.Context, p sdkprov.Provider, repo *corebridge.Repo, state string, max int) ([]*issueRow, error) {
	// 平台不一定实现 IssueManager
	im, ok := p.(sdkprov.IssueManager)
	if !ok {
		return nil, fmt.Errorf("platform %s does not support issue export", repo.Platform)
	}
	// 空 state 交平台默认(通常 open+closed 或 open)
	st := sdkprov.IssueState(state)
	const perPage = 50
	// 页预算 = 拉满 max 所需页 +1 页空页观测(空页终止语义,v0.73 分页面);
	// fn 内到 max 即 ErrStopIteration 提前收束,预算实际用不到。
	maxPages := (max + perPage - 1) / perPage
	if maxPages < 1 {
		maxPages = 1
	}
	var batch []*sdkprov.Issue
	err := sdkprov.EachBounded(ctx,
		func(ctx context.Context, page int) ([]*sdkprov.Issue, error) {
			items, _, lerr := im.ListIssues(ctx, sdkprov.ListIssuesOptions{
				Owner:   repo.PlatformOwner,
				Repo:    repo.PlatformRepo,
				State:   st,
				Page:    page,
				PerPage: perPage,
			})
			return items, lerr
		}, maxPages+1,
		func(iss *sdkprov.Issue) error {
			if len(batch) >= max {
				return sdkprov.ErrStopIteration
			}
			batch = append(batch, iss)
			return nil
		})
	if err != nil {
		return nil, err
	}
	out := make([]*issueRow, 0, len(batch))
	for _, iss := range batch {
		out = append(out, toIssueRow(iss))
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

// buildIssuesCSVRows 构造 issue 导出 CSV 行(首行表头)。
// 交给 render.CSV 走标准库 encoding/csv 序列化(RFC 4180),单元格转义由库负责。
func buildIssuesCSVRows(issues []*issueRow) [][]string {
	rows := make([][]string, 0, len(issues)+1)
	rows = append(rows, []string{"number", "state", "author", "title", "labels", "created_at"})
	for _, iss := range issues {
		rows = append(rows, []string{
			iss.Number, iss.State, iss.Author, iss.Title,
			strings.Join(iss.Labels, "|"), iss.CreatedAt,
		})
	}
	return rows
}
