package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 权限矩阵单源:新增角色/能力时这里必须仍保持单调（admin ⊇ operator ⊇ readonly）。
func TestRoleMatrix_MonotonicCapabilities(t *testing.T) {
	assert.True(t, Can(RoleAdmin, "read"))
	assert.True(t, Can(RoleAdmin, "write"))
	assert.True(t, Can(RoleAdmin, "manage"))

	assert.True(t, Can(RoleOperator, "read"))
	assert.True(t, CanWrite(RoleOperator))
	assert.False(t, IsAdmin(RoleOperator))

	assert.True(t, Can(RoleReadonly, "read"))
	assert.False(t, CanWrite(RoleReadonly))
	assert.False(t, IsAdmin(RoleReadonly))

	assert.False(t, Can(Role("unknown"), "read"), "未知角色回落无权限")
}

func TestRoleMatrix_SnapshotDeterministic(t *testing.T) {
	first := MatrixSnapshot()
	assert.NotEmpty(t, first)
	assert.Equal(t, first, MatrixSnapshot(), "同一矩阵的快照必须稳定（审计/诊断可比对）")
}
