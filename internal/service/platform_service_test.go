package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkprov "github.com/yi-nology/git-platform-sdk/provider"
	"github.com/yi-nology/git-sync-service/internal/dao"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupPlatformServiceTestDB(t *testing.T) (*gorm.DB, *PlatformService) {
	t.Helper()
	// Set up encryption key for tests
	key := "0123456789abcdef0123456789abcdef" // 32 bytes for AES-256
	t.Setenv("ENCRYPTION_KEY", key)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test db")

	err = db.AutoMigrate(&model.Platform{}, &model.Repo{})
	require.NoError(t, err, "failed to migrate test db")

	platformDAO, err := dao.NewPlatformDAO(db)
	require.NoError(t, err, "failed to create PlatformDAO")

	repoDAO, err := dao.NewRepoDAO(db)
	require.NoError(t, err, "failed to create RepoDAO")

	providerMgr := sdkprov.NewManager(0)
	svc := NewPlatformService(platformDAO, repoDAO, providerMgr)

	return db, svc
}

func TestPlatformService_CreateAndGet(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create a platform
	platform := &model.Platform{
		Key:         "github",
		Name:        "GitHub",
		Type:        "github",
		InstanceURL: "https://github.com",
		APIURL:      "https://api.github.com",
		Status:      "active",
	}

	err := svc.CreatePlatform(ctx, platform)
	require.NoError(t, err, "create failed")

	assert.NotZero(t, platform.ID, "expected platform ID to be set after create")

	// Get by key
	got, err := svc.GetPlatform(ctx, "github")
	require.NoError(t, err, "get failed")

	assert.Equal(t, "github", got.Key, "expected key 'github'")
	assert.Equal(t, "GitHub", got.Name, "expected name 'GitHub'")
}

func TestPlatformService_GetByID(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create a platform
	platform := &model.Platform{
		Key:  "gitlab",
		Name: "GitLab",
		Type: "gitlab",
	}

	err := svc.CreatePlatform(ctx, platform)
	require.NoError(t, err, "create failed")

	// Get by ID
	got, err := svc.GetPlatformByID(ctx, platform.ID)
	require.NoError(t, err, "get by ID failed")

	assert.Equal(t, "gitlab", got.Key, "expected key 'gitlab'")
	assert.Equal(t, "GitLab", got.Name, "expected name 'GitLab'")
}

func TestPlatformService_List(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create multiple platforms
	platforms := []*model.Platform{
		{Key: "github", Name: "GitHub", Type: "github"},
		{Key: "gitlab", Name: "GitLab", Type: "gitlab"},
		{Key: "bitbucket", Name: "Bitbucket", Type: "bitbucket"},
	}

	for _, p := range platforms {
		err := svc.CreatePlatform(ctx, p)
		require.NoError(t, err, "create failed")
	}

	// List all
	got, err := svc.ListPlatforms(ctx)
	require.NoError(t, err, "list failed")

	require.Len(t, got, 3, "expected 3 platforms")
}

func TestPlatformService_Update(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create a platform
	platform := &model.Platform{
		Key:         "github",
		Name:        "GitHub",
		Type:        "github",
		InstanceURL: "https://github.com",
	}

	err := svc.CreatePlatform(ctx, platform)
	require.NoError(t, err, "create failed")

	// Update the platform
	platform.Name = "GitHub Enterprise"
	platform.InstanceURL = "https://github.example.com"

	err = svc.UpdatePlatform(ctx, platform)
	require.NoError(t, err, "update failed")

	// Get the updated platform
	got, err := svc.GetPlatform(ctx, "github")
	require.NoError(t, err, "get failed")

	assert.Equal(t, "GitHub Enterprise", got.Name, "expected name 'GitHub Enterprise'")
	assert.Equal(t, "https://github.example.com", got.InstanceURL, "expected instance URL")
}

func TestPlatformService_Delete(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create a platform
	platform := &model.Platform{
		Key:  "github",
		Name: "GitHub",
	}

	err := svc.CreatePlatform(ctx, platform)
	require.NoError(t, err, "create failed")

	// Delete the platform
	err = svc.DeletePlatform(ctx, "github")
	require.NoError(t, err, "delete failed")

	// Try to get the deleted platform - should return (nil, nil)
	p, err := svc.GetPlatform(ctx, "github")
	require.NoError(t, err, "unexpected error")
	assert.Nil(t, p, "expected nil platform for deleted key")
}

func TestPlatformService_SetDefault(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create two platforms
	platform1 := &model.Platform{
		Key:       "github",
		Name:      "GitHub",
		IsDefault: true,
	}

	platform2 := &model.Platform{
		Key:       "gitlab",
		Name:      "GitLab",
		IsDefault: false,
	}

	err := svc.CreatePlatform(ctx, platform1)
	require.NoError(t, err, "create platform1 failed")

	err = svc.CreatePlatform(ctx, platform2)
	require.NoError(t, err, "create platform2 failed")

	// Set platform2 as default
	err = svc.SetDefaultPlatform(ctx, "gitlab")
	require.NoError(t, err, "set default failed")

	// Verify platform2 is now default
	got2, err := svc.GetPlatform(ctx, "gitlab")
	require.NoError(t, err, "get failed")

	assert.True(t, got2.IsDefault, "expected platform2 to be default")

	// Verify platform1 is no longer default
	got1, err := svc.GetPlatform(ctx, "github")
	require.NoError(t, err, "get failed")

	assert.False(t, got1.IsDefault, "expected platform1 to not be default")
}

func TestPlatformService_UpdateStatus(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Create a platform
	platform := &model.Platform{
		Key:    "github",
		Name:   "GitHub",
		Status: "active",
	}

	err := svc.CreatePlatform(ctx, platform)
	require.NoError(t, err, "create failed")

	// Update status
	err = svc.UpdatePlatformStatus(ctx, "github", "inactive", "connection failed")
	require.NoError(t, err, "update status failed")

	// Verify the status was updated
	got, err := svc.GetPlatform(ctx, "github")
	require.NoError(t, err, "get failed")

	assert.Equal(t, "inactive", got.Status, "expected status 'inactive'")
	assert.Equal(t, "connection failed", got.LastTestResult, "expected last test result 'connection failed'")
}

func TestPlatformService_GetNotFound(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Try to get a non-existent platform - should return (nil, nil)
	p, err := svc.GetPlatform(ctx, "nonexistent")
	require.NoError(t, err, "unexpected error")
	assert.Nil(t, p, "expected nil platform for non-existent key")
}

func TestPlatformService_GetByIDNotFound(t *testing.T) {
	_, svc := setupPlatformServiceTestDB(t)
	ctx := context.Background()

	// Try to get a non-existent platform - should return (nil, nil)
	p, err := svc.GetPlatformByID(ctx, 999)
	require.NoError(t, err, "unexpected error")
	assert.Nil(t, p, "expected nil platform for non-existent ID")
}

func TestSplitRepoPath(t *testing.T) {
	cases := []struct {
		name      string
		fullName  string
		sdkOwner  string
		sdkName   string
		wantOwner string
		wantPath  string
	}{
		{"普通两段路径", "yi-nology/iam-sdk", "wrong-user", "iam-sdk", "yi-nology", "iam-sdk"},
		{"GitLab 嵌套群组", "obs/sdk/server", "obs", "server", "obs", "sdk/server"},
		{"GitLab 展示名场景", "obs/insights-mcp-gateway", "obs", "Insights Mcp", "obs", "insights-mcp-gateway"},
		{"FullName 缺失回退", "", "owner", "repo", "owner", "repo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			owner, path := splitRepoPath(c.fullName, c.sdkOwner, c.sdkName)
			assert.Equal(t, c.wantOwner, owner)
			assert.Equal(t, c.wantPath, path)
			if c.fullName != "" {
				assert.Equal(t, c.fullName, owner+"/"+path, "pidOf 应还原完整路径")
			}
		})
	}
}

func TestRewriteCloneHost(t *testing.T) {
	cases := []struct {
		name     string
		rawURL   string
		instance string
		want     string
	}{
		{"私有实例重写公网地址", "https://gitcode.com/yi-nology/iam-web.git", "gitcode.kylinos.cn", "https://gitcode.kylinos.cn/yi-nology/iam-web.git"},
		{"实例地址带 scheme", "https://gitcode.com/a/b.git", "https://gitcode.kylinos.cn", "https://gitcode.kylinos.cn/a/b.git"},
		{"host 相同不重写", "https://om-gitlab.kylinos.cn/obs/x.git", "om-gitlab.kylinos.cn", "https://om-gitlab.kylinos.cn/obs/x.git"},
		{"公网平台无实例不重写", "https://github.com/o/r.git", "", "https://github.com/o/r.git"},
		{"空地址不处理", "", "gitcode.kylinos.cn", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &model.Platform{InstanceURL: c.instance}
			got := rewriteCloneHost(c.rawURL, p)
			assert.Equal(t, c.want, got, "rewriteCloneHost(%q, %q)", c.rawURL, c.instance)
		})
	}
}
