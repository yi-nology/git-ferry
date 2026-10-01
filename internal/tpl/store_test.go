package tpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestSpecMerge_ChildOverrides(t *testing.T) {
	base := Spec{Cron: "0 2 * * *", SourceBranch: "main", RetryMax: 3}
	child := Spec{Cron: "0 5 * * *", Enabled: boolPtr(true)}
	out := base.Merge(child)
	assert.Equal(t, "0 5 * * *", out.Cron)    // child 覆盖
	assert.Equal(t, "main", out.SourceBranch) // 保留
	assert.Equal(t, 3, out.RetryMax)
	require.NotNil(t, out.Enabled)
	assert.True(t, *out.Enabled)
}

func TestResolve_Chain(t *testing.T) {
	list := []Template{
		{ID: "base", Spec: Spec{Cron: "0 1 * * *", SourceBranch: "main", RetryMax: 2}},
		{ID: "mid", Extends: "base", Spec: Spec{Cron: "0 2 * * *"}},
		{ID: "leaf", Extends: "mid", Spec: Spec{TargetBranch: "mirror", Enabled: boolPtr(false)}},
	}
	spec, chain, err := ResolveSelf(list, "leaf")
	require.NoError(t, err)
	assert.Equal(t, []string{"leaf", "mid", "base"}, chain)
	assert.Equal(t, "0 2 * * *", spec.Cron) // mid 覆盖 base
	assert.Equal(t, "main", spec.SourceBranch)
	assert.Equal(t, "mirror", spec.TargetBranch)
	assert.Equal(t, 2, spec.RetryMax)
	require.NotNil(t, spec.Enabled)
	assert.False(t, *spec.Enabled)
}

func TestResolve_Cycle(t *testing.T) {
	list := []Template{
		{ID: "a", Extends: "b"},
		{ID: "b", Extends: "a"},
	}
	_, _, err := ResolveSelf(list, "a")
	assert.ErrorIs(t, err, ErrCycle)
}

func TestResolve_MissingRoot(t *testing.T) {
	_, _, err := ResolveSelf(nil, "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestResolve_MissingParentStops(t *testing.T) {
	list := []Template{
		{ID: "child", Extends: "ghost", Spec: Spec{Cron: "0 9 * * *"}},
	}
	spec, chain, err := ResolveSelf(list, "child")
	require.NoError(t, err)
	assert.Equal(t, []string{"child"}, chain)
	assert.Equal(t, "0 9 * * *", spec.Cron)
}

func TestStoreEffective(t *testing.T) {
	s := &Store{list: []Template{
		{ID: "base", Name: "Base", Spec: Spec{Cron: "0 1 * * *"}},
		{ID: "child", Name: "Child", Extends: "base", Spec: Spec{Cron: "0 3 * * *"}},
	}}
	eff, err := s.Effective("child")
	require.NoError(t, err)
	assert.Equal(t, "0 3 * * *", eff.Spec.Cron)
	assert.Contains(t, eff.Description, "extends")
}
