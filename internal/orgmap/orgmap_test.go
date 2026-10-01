package orgmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePolicy(t *testing.T) {
	p, err := ParsePolicy("")
	require.NoError(t, err)
	assert.Equal(t, PolicyPreserve, p)
	p, err = ParsePolicy("SINGLE")
	require.NoError(t, err)
	assert.Equal(t, PolicySingle, p)
	_, err = ParsePolicy("bogus")
	assert.Error(t, err)
}

func TestMap_Preserve(t *testing.T) {
	out, err := Map(PolicyPreserve, &Input{SourceKey: "github/acme/app"})
	require.NoError(t, err)
	assert.Equal(t, "acme", out.TargetOwner)
	assert.Equal(t, "app", out.TargetRepo)
}

func TestMap_Single(t *testing.T) {
	out, err := Map(PolicySingle, &Input{
		SourceKey: "github/acme/app", TargetNamespace: "backup", TargetPlatform: "gitlab",
	})
	require.NoError(t, err)
	assert.Equal(t, "backup", out.TargetOwner)
	assert.Equal(t, "gitlab/backup/app", out.TargetKey)
}

func TestMap_FlatRequiresUser(t *testing.T) {
	_, err := Map(PolicyFlat, &Input{SourceKey: "a/b"})
	assert.Error(t, err)
}

func TestMap_Mixed(t *testing.T) {
	// 个人仓 → flat
	out, err := Map(PolicyMixed, &Input{
		SourceOwner: "alice", SourceRepo: "dotfiles",
		IsPersonal: true, TargetNamespace: "backup",
	})
	require.NoError(t, err)
	assert.Equal(t, "backup", out.TargetOwner)

	// 组织仓 → preserve
	out, err = Map(PolicyMixed, &Input{
		SourceOwner: "acme", SourceRepo: "app",
		IsPersonal: false, TargetNamespace: "backup",
	})
	require.NoError(t, err)
	assert.Equal(t, "acme", out.TargetOwner)
}
