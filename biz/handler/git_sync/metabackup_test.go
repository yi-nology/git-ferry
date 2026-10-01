package git_sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

// fakeBackupProv 覆盖备份编排用到的可选能力;嵌入 nil Provider 保证未用到的
// 核心方法被触达时立刻 panic(测试即失败)。
type fakeBackupProv struct {
	sdkprov.Provider
	caps sdkprov.CapabilitySet

	gistPages  map[int][]*sdkprov.Gist
	gistCalls  []int
	releases   []*sdkprov.ReleaseInfo
	relCalls   int
	assetData  map[int64][]byte
	assetErr   map[int64]error
	assetCalls []int64
}

func (f *fakeBackupProv) Capabilities() sdkprov.CapabilitySet { return f.caps }

func (f *fakeBackupProv) ListMyGists(_ context.Context, page, _ int) ([]*sdkprov.Gist, error) {
	f.gistCalls = append(f.gistCalls, page)
	return f.gistPages[page], nil
}

func (f *fakeBackupProv) ListReleases(_ context.Context, _, _ string) ([]*sdkprov.ReleaseInfo, error) {
	f.relCalls++
	return f.releases, nil
}

func (f *fakeBackupProv) DownloadReleaseAsset(_ context.Context, _, _ string, assetID int64, w io.Writer) error {
	f.assetCalls = append(f.assetCalls, assetID)
	if err := f.assetErr[assetID]; err != nil {
		return err
	}
	_, err := w.Write(f.assetData[assetID])
	return err
}

// TestBackupGists_PaginatesToMax 迁自原 githubapi 上限截断断言,并覆盖分页拉全:
// maxGists=150 时应翻到第 2 页(原实现只拉 1 页 ≤100 却标 maxGists=200),截断 150。
func TestBackupGists_PaginatesToMax(t *testing.T) {
	page1 := make([]*sdkprov.Gist, 0, 100)
	for i := 0; i < 100; i++ {
		page1 = append(page1, &sdkprov.Gist{ID: fmt.Sprintf("p1-gist-%03d", i)})
	}
	page2 := make([]*sdkprov.Gist, 0, 100)
	for i := 0; i < 100; i++ {
		page2 = append(page2, &sdkprov.Gist{ID: fmt.Sprintf("p2-gist-%03d", i)})
	}
	// 路径不安全 ID:落盘文件名必须经清洗(原 githubapi 的路径清洗断言迁移点)
	page2[0].ID = "evil/../id"

	prov := &fakeBackupProv{
		caps:      sdkprov.CapabilitySet{Gists: true},
		gistPages: map[int][]*sdkprov.Gist{1: page1, 2: page2},
	}
	dest := t.TempDir()
	count, warnings, err := backupGists(t.Context(), prov, dest, 150)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, 50+100, count, "应分页拉到第 2 页并截断到 maxGists=150")
	assert.Equal(t, []int{1, 2}, prov.gistCalls, "只应翻到第 2 页")

	entries, err := os.ReadDir(dest)
	require.NoError(t, err)
	assert.Len(t, entries, 150)
	// 清洗后的文件名: "/" → "_"、".." → "_"
	_, err = os.Stat(filepath.Join(dest, "evil___id.json"))
	assert.NoError(t, err, "路径不安全的 gist ID 必须落为清洗后的文件名")
}

func TestBackupGists_DefaultMaxAndNoCapability(t *testing.T) {
	// 无 Gists 能力:报错,不落盘
	prov := &fakeBackupProv{caps: sdkprov.CapabilitySet{}}
	_, _, err := backupGists(t.Context(), prov, t.TempDir(), 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support gists")

	// 默认上限 200:1 页 100 条 + 第 2 页不足一页 → 2 页拉全
	page1 := make([]*sdkprov.Gist, 100)
	for i := range page1 {
		page1[i] = &sdkprov.Gist{ID: fmt.Sprintf("g-%03d", i)}
	}
	prov2 := &fakeBackupProv{
		caps:      sdkprov.CapabilitySet{Gists: true},
		gistPages: map[int][]*sdkprov.Gist{1: page1, 2: {{ID: "last"}}},
	}
	count, _, err := backupGists(t.Context(), prov2, t.TempDir(), 0)
	require.NoError(t, err)
	assert.Equal(t, 101, count)
	assert.Equal(t, []int{1, 2}, prov2.gistCalls)
}

// TestDownloadReleaseAssets_ReusesReleases 迁自原附件下载断言:复用已取到的
// releases(不重复请求)、文件名 tag__name 清洗、流式落盘。
func TestDownloadReleaseAssets_ReusesReleases(t *testing.T) {
	prov := &fakeBackupProv{
		caps:      sdkprov.CapabilitySet{ReleaseAssets: true},
		assetData: map[int64][]byte{7: []byte("payload")},
	}
	releases := []*sdkprov.ReleaseInfo{{
		TagName: "rel/../v1",
		Assets:  []*sdkprov.ReleaseAsset{{ID: 7, Name: "a/b.bin"}},
	}}
	dest := t.TempDir()
	saved, warnings, err := downloadReleaseAssets(t.Context(), prov, "o", "r", releases, dest, 10)
	require.NoError(t, err)
	assert.Empty(t, warnings)
	assert.Equal(t, 0, prov.relCalls, "已持有带 Assets 的 releases 时不应重复请求")
	// SanitizePathToken("rel/../v1")="rel___v1", SanitizePathToken("a/b.bin")="a_b.bin"
	require.Equal(t, []string{"rel___v1__a_b.bin"}, saved)
	data, rerr := os.ReadFile(filepath.Join(dest, "rel___v1__a_b.bin"))
	require.NoError(t, rerr)
	assert.Equal(t, []byte("payload"), data)
}

func TestDownloadReleaseAssets_FetchAndCapAndWarn(t *testing.T) {
	prov := &fakeBackupProv{
		caps:      sdkprov.CapabilitySet{ReleaseAssets: true},
		assetData: map[int64][]byte{1: []byte("one"), 2: []byte("two"), 3: []byte("three")},
		assetErr:  map[int64]error{2: fmt.Errorf("boom")},
		releases: []*sdkprov.ReleaseInfo{
			{TagName: "v1", Assets: []*sdkprov.ReleaseAsset{{ID: 1, Name: "one.bin"}}},
			{TagName: "v2", Assets: []*sdkprov.ReleaseAsset{{ID: 2, Name: "two.bin"}, {ID: 3, Name: "three.bin"}}},
		},
	}
	dest := t.TempDir()
	saved, warnings, err := downloadReleaseAssets(t.Context(), prov, "o", "r", nil, dest, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, prov.relCalls, "未持有 releases 时应现拉 ListReleases")
	// 附件下载失败:warning 保留原格式 "tag/name: err",继续后续附件
	require.Equal(t, 1, len(warnings))
	assert.Contains(t, warnings[0], "v2/two.bin")
	assert.Contains(t, warnings[0], "boom")
	assert.Equal(t, []string{"v1__one.bin", "v2__three.bin"}, saved, "失败附件跳过,后续附件继续下载")

	// maxAssets 上限截断(原 githubapi 的上限截断断言迁移点):第 3 个附件不再下载
	prov2 := &fakeBackupProv{
		caps:      sdkprov.CapabilitySet{ReleaseAssets: true},
		assetData: map[int64][]byte{1: []byte("one"), 2: []byte("two"), 3: []byte("three")},
		releases: []*sdkprov.ReleaseInfo{{
			TagName: "v1",
			Assets:  []*sdkprov.ReleaseAsset{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}},
		}},
	}
	saved2, _, err := downloadReleaseAssets(t.Context(), prov2, "o", "r", nil, t.TempDir(), 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"v1__a", "v1__b"}, saved2)
	assert.Equal(t, []int64{1, 2}, prov2.assetCalls, "达到 maxAssets 后不再下载")

	// 无能力门控
	prov3 := &fakeBackupProv{}
	_, _, err = downloadReleaseAssets(t.Context(), prov3, "o", "r", nil, t.TempDir(), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support release assets")
}
