package service

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	errors "github.com/cockroachdb/errors"
	"github.com/google/uuid"
	sdkprov "github.com/yi-nology/git-platform-sdk/provider"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
)

// RepoService handles repository-related operations.
type RepoService struct {
	repoDAO     *dao.RepoDAO
	platformDAO *dao.PlatformDAO
	providerMgr *sdkprov.Manager
}

// NewRepoService creates a new RepoService instance.
func NewRepoService(repoDAO *dao.RepoDAO, platformDAO *dao.PlatformDAO, providerMgr *sdkprov.Manager) *RepoService {
	return &RepoService{
		repoDAO:     repoDAO,
		platformDAO: platformDAO,
		providerMgr: providerMgr,
	}
}

// ListRepos returns a paginated list of repositories.
func (rs *RepoService) ListRepos(ctx context.Context, offset, limit int) ([]*model.Repo, int64, error) {
	page := dao.DefaultPagination(offset, limit)
	return rs.repoDAO.FindAll(page)
}

// CountRepos returns the total repo count via COUNT aggregate.
func (rs *RepoService) CountRepos() (int64, error) {
	return rs.repoDAO.Count()
}

// ListReposWithFilter returns a filtered, sorted, paginated list of repositories.
func (rs *RepoService) ListReposWithFilter(ctx context.Context, offset, limit int, filter *dao.RepoFilter) ([]*model.Repo, int64, error) {
	page := dao.DefaultPagination(offset, limit)
	return rs.repoDAO.ListWithFilter(page, filter)
}

// GetRepo returns a repository by key.
func (rs *RepoService) GetRepo(ctx context.Context, key string) (*model.Repo, error) {
	return rs.repoDAO.FindByKey(key)
}

// CreateRepo creates a new repository.
func (rs *RepoService) CreateRepo(ctx context.Context, req *model.CreateRepoRequest) (*model.Repo, error) {
	// Try to detect platform from URL
	result, err := sdkprov.DetectPlatform(req.RemoteURL)
	if err != nil && req.PlatformID == 0 {
		// If detection fails and no platform_id provided, return error
		if errors.Is(err, sdkprov.ErrPlatformNotSupported) {
			return nil, errors.Wrapf(err, "unsupported platform for URL %s", req.RemoteURL)
		}
		return nil, errors.Wrap(err, "invalid remote URL")
	}

	// If platform_id is provided, use the platform's type
	var platformType string
	var platformOwner, platformRepo string
	if result != nil {
		platformType = string(result.Platform)
		platformOwner = result.Owner
		platformRepo = result.Repo
	}

	if req.PlatformID > 0 && rs.platformDAO != nil {
		platform, err := rs.platformDAO.FindByID(req.PlatformID)
		if err == nil && platform != nil {
			platformType = platform.Type
		}
	}

	// Parse owner/repo from URL if not detected
	if platformOwner == "" || platformRepo == "" {
		// https://host/group/sub/repo.git → path "group/sub/repo"
		// 嵌套群组必须整段路径保留,否则 ListBranches/Webhook 拼 pidOf 会 404。
		raw := strings.TrimSuffix(strings.TrimSuffix(req.RemoteURL, ".git"), "/")
		if u, uerr := url.Parse(raw); uerr == nil && u.Path != "" {
			path := strings.Trim(u.Path, "/")
			if owner, name, ok := strings.Cut(path, "/"); ok && owner != "" && name != "" {
				platformOwner, platformRepo = owner, name
			}
		} else {
			parts := strings.Split(raw, "/")
			if len(parts) >= 2 {
				platformRepo = parts[len(parts)-1]
				platformOwner = parts[len(parts)-2]
			}
		}
	}

	repo := &model.Repo{
		Key:           uuid.New().String(),
		Name:          req.Name,
		PlatformID:    req.PlatformID,
		Platform:      platformType,
		PlatformOwner: platformOwner,
		PlatformRepo:  platformRepo,
		CloneURL:      req.RemoteURL,
		AccessToken:   req.AccessToken,
		Status:        model.RepoStatusActive,
	}

	if err := rs.repoDAO.Create(repo); err != nil {
		return nil, err
	}

	// Update platform repo count
	if repo.PlatformID > 0 && rs.platformDAO != nil {
		if err := rs.platformDAO.UpdateRepoCount(repo.PlatformID); err != nil {
			slog.Warn("create repo: failed to update platform repo count", "platform_id", repo.PlatformID, "error", err)
		}
	}

	return repo, nil
}

// UpdateRepo updates an existing repository.
func (rs *RepoService) UpdateRepo(ctx context.Context, req *model.UpdateRepoRequest) (*model.Repo, error) {
	repo, err := rs.repoDAO.FindByKey(req.Key)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, ErrRepoNotFound
	}

	if req.Name != "" {
		repo.Name = req.Name
	}
	if req.AccessToken != "" {
		repo.AccessToken = req.AccessToken
	}

	if err := rs.repoDAO.Update(repo); err != nil {
		return nil, err
	}

	return repo, nil
}

// DeleteRepo deletes a repository by key.
func (rs *RepoService) DeleteRepo(ctx context.Context, key string) error {
	// Get repo first to know the platform_id
	repo, err := rs.repoDAO.FindByKey(key)
	if err != nil {
		return err
	}
	if repo == nil {
		return ErrRepoNotFound
	}

	platformID := repo.PlatformID

	if err := rs.repoDAO.Delete(key); err != nil {
		return err
	}

	// Update platform repo count
	if platformID > 0 && rs.platformDAO != nil {
		if err := rs.platformDAO.UpdateRepoCount(platformID); err != nil {
			slog.Warn("delete repo: failed to update platform repo count", "platform_id", platformID, "error", err)
		}
	}

	return nil
}

// GetRepoWithProvider 一次调用完成 repo 查找 + provider 解析,
// 消除调用方重复的 "GetRepoByKey → nil check → resolveRepoProvider" 样板。
func (rs *RepoService) GetRepoWithProvider(repoKey string) (*model.Repo, sdkprov.Provider, error) {
	repo, err := rs.repoDAO.FindByKey(repoKey)
	if err != nil {
		return nil, nil, err
	}
	if repo == nil {
		return nil, nil, ErrRepoNotFound
	}
	prov, err := rs.resolveRepoProvider(repo)
	if err != nil {
		return nil, nil, err
	}
	return repo, prov, nil
}

// resolveRepoProvider 解析仓库对应的 provider:repo token 优先,回退关联平台 token;
// 有关联平台记录时按平台配置构造(经 Manager 缓存),否则按 CloneURL 探测平台。
func (rs *RepoService) resolveRepoProvider(repo *model.Repo) (sdkprov.Provider, error) {
	token := repo.AccessToken
	var platform *model.Platform
	if repo.PlatformID > 0 && rs.platformDAO != nil {
		var findErr error
		platform, findErr = rs.platformDAO.FindByID(repo.PlatformID)
		if findErr != nil {
			return nil, errors.Wrapf(findErr, "find platform %d for repo %s", repo.PlatformID, repo.Key)
		}
		if platform != nil && token == "" {
			token = platform.AccessToken
		}
	}
	if platform != nil {
		return rs.providerMgr.Get(providerConfig(platform, token))
	}
	return rs.providerMgr.GetByURL(repo.CloneURL, token)
}

// GetRepoByKey returns a repository by key.
func (rs *RepoService) GetRepoByKey(key string) (*model.Repo, error) {
	return rs.repoDAO.FindByKey(key)
}

// SetWebhookSecret 设置仓库的 Webhook 密钥(经 repo_dao 加密落库),供入站验签使用。
func (rs *RepoService) SetWebhookSecret(repoKey, secret string) error {
	repo, err := rs.repoDAO.FindByKey(repoKey)
	if err != nil {
		return err
	}
	if repo == nil {
		return ErrRepoNotFound
	}
	repo.WebhookSecret = secret
	return rs.repoDAO.Update(repo)
}

// TestConnection tests the connection to a repository.
func (rs *RepoService) TestConnection(ctx context.Context, repoKey string) (*model.TestConnectionResult, error) {
	_, prov, err := rs.GetRepoWithProvider(repoKey)
	if err != nil {
		return &model.TestConnectionResult{Success: false, Message: err.Error()}, nil
	}

	result, err := prov.TestConnection(ctx)
	if err != nil {
		return &model.TestConnectionResult{Success: false, Message: err.Error()}, nil
	}

	return &model.TestConnectionResult{Success: result.Connected, Message: result.Message}, nil
}

// ListBranches returns a list of branches for a repository.
func (rs *RepoService) ListBranches(ctx context.Context, repoKey string) ([]string, error) {
	repo, prov, err := rs.GetRepoWithProvider(repoKey)
	if err != nil {
		return nil, err
	}

	branches, err := prov.ListBranches(ctx, repo.PlatformOwner, repo.PlatformRepo)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, b := range branches {
		result = append(result, b.Name)
	}

	return result, nil
}
