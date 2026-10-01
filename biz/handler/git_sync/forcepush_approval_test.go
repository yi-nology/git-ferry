package git_sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForcePushApproval_RoundTrip(t *testing.T) {
	// 直接测 load/save 路径逻辑：写到 data/ 会污染，改用 chdir
	tmp := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	list := []forcePushApproval{{
		ID: "fp-1", TaskKey: "t1", Branch: "main", Reason: "divergent",
	}}
	if err := saveForcePushApprovals(list); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "data", "force-push-approvals.json")); err != nil {
		// BackupDir 为空时默认 data/ 相对 cwd
		t.Fatalf("approval file: %v", err)
	}
	got := loadForcePushApprovals()
	if len(got) != 1 || got[0].ID != "fp-1" {
		t.Fatalf("got %+v", got)
	}
	if IsForcePushApproved("t1", "main") {
		t.Fatal("not approved yet")
	}
	got[0].Approved = true
	if err := saveForcePushApprovals(got); err != nil {
		t.Fatal(err)
	}
	if !IsForcePushApproved("t1", "main") {
		t.Fatal("want approved")
	}
}
