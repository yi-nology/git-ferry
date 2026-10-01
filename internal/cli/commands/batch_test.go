package commands

import (
	"testing"

	"github.com/yi-nology/git-ferry/internal/cli/cmdutil"
)

// 表驱动：批量 dry-run 不触发危险门；无 --yes 的真实执行被拦。
func TestTaskBatchRunDryRun(t *testing.T) {
	cmd := sc("+batch-run", "", taskBatchRun)
	_ = cmd.Flags().Set("task-keys", "t1,t2")
	_ = cmd.Flags().Set("dry-run", "true")

	// 直接调用 runFunc
	fn := taskBatchRun
	// 用假 client：dry-run 分支不会发请求
	env, err := fn(nil, nil, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("dry-run should be ok: %+v", env)
	}
	m := env.Data.(map[string]any)
	if m["dry_run"] != true {
		t.Fatalf("dry_run=%v", m["dry_run"])
	}
	if m["total"] != 2 {
		t.Fatalf("total=%v", m["total"])
	}
}

func TestTaskBatchRunRequiresKeys(t *testing.T) {
	cmd := sc("+batch-run", "", taskBatchRun)
	_, err := taskBatchRun(nil, nil, cmd, nil)
	if err == nil {
		t.Fatal("expected error without keys")
	}
}

func TestTaskBatchRunBlockedWithoutYes(t *testing.T) {
	old := cmdutil.Yes
	cmdutil.Yes = false
	defer func() { cmdutil.Yes = old }()

	cmd := sc("+batch-run", "", taskBatchRun)
	_ = cmd.Flags().Set("task-keys", "t1")
	env, err := taskBatchRun(nil, nil, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env.OK {
		t.Fatal("should be blocked")
	}
	if env.Error.Code != 409 {
		t.Fatalf("code=%v", env.Error.Code)
	}
}

func TestOpsRetryBatchDryRun(t *testing.T) {
	old := cmdutil.Yes
	cmdutil.Yes = true // 即便 --yes，dry-run 也不应发请求
	defer func() { cmdutil.Yes = old }()

	cmd := sc("+retry-batch", "", opsRetryBatch)
	_ = cmd.Flags().Set("dry-run", "true")
	env, err := opsRetryBatch(nil, nil, cmd, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("dry-run should ok: %+v", env)
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a , b ,,c ")
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("got %v", got)
	}
	if splitCSV("") != nil {
		t.Fatal("empty should be nil")
	}
}

// 确保 sc() 挂上了 --all / --dry-run / --task-keys
func TestScBatchFlags(t *testing.T) {
	c := sc("+x", "", taskBatchRun)
	for _, name := range []string{"all", "dry-run", "task-keys"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("missing --%s", name)
		}
	}
}
