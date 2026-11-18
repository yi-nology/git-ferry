package acp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// terminal/* 必须受 cfg.Permission 约束：此前 deny / approve_reads 档仍可
// 通过 terminal/create 执行任意命令。
func TestTerminal_AuthorizationByPermissionMode(t *testing.T) {
	cases := []struct {
		name       string
		permission string
		command    string
		args       []string
		wantErr    bool
	}{
		{"deny blocks readonly", PermissionDeny, "ls", nil, true},
		{"deny blocks shell", PermissionDeny, "sh", []string{"-c", "id"}, true},
		{"approve_reads allows ls", PermissionApproveReads, "ls", []string{"-la"}, false},
		{"approve_reads allows cat inside root", PermissionApproveReads, "cat", []string{"README.md"}, false},
		{"approve_reads blocks write", PermissionApproveReads, "touch", []string{"x"}, true},
		{"approve_reads blocks shell", PermissionApproveReads, "sh", []string{"-c", "id"}, true},
		{"approve_reads blocks absolute path", PermissionApproveReads, "cat", []string{"/etc/passwd"}, true},
		{"approve_reads blocks parent traversal", PermissionApproveReads, "cat", []string{"../secret"}, true},
		{"approve_reads allows git status", PermissionApproveReads, "git", []string{"status"}, false},
		{"approve_reads blocks git checkout", PermissionApproveReads, "git", []string{"checkout", "main"}, true},
		{"approve_reads allows git branch -a", PermissionApproveReads, "git", []string{"branch", "-a"}, false},
		{"approve_reads blocks git branch -D", PermissionApproveReads, "git", []string{"branch", "-D", "main"}, true},
		{"ask keeps legacy behavior", PermissionAsk, "sh", []string{"-c", "id"}, false},
		{"approve_all keeps legacy behavior", PermissionApproveAll, "sh", []string{"-c", "id"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &terminalHandler{permission: tc.permission}
			err := h.authorizeCommand(tc.command, tc.args)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
