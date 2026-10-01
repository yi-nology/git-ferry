package commands

import (
	"net/url"
	"testing"

	"github.com/spf13/cobra"

	"github.com/yi-nology/git-ferry/internal/cli/client"
	"github.com/yi-nology/git-ferry/internal/cli/config"
	"github.com/yi-nology/git-ferry/internal/cli/output"
)

func TestRequireFlag(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("key", "", "")
	if _, err := requireFlag(cmd, "key"); err == nil {
		t.Fatal("expected error for missing flag")
	}
	_ = cmd.Flags().Set("key", "k1")
	v, err := requireFlag(cmd, "key")
	if err != nil || v != "k1" {
		t.Fatalf("v=%q err=%v", v, err)
	}
}

func TestConfirmDangerBlocksWhenNotYes(t *testing.T) {
	env := confirmDanger("测试操作")
	if env == nil {
		t.Fatal("expected denial envelope")
	}
	if env.OK {
		t.Fatal("expected not ok")
	}
	if env.Error == nil || env.Error.Code != 409 {
		t.Fatalf("error=%+v", env.Error)
	}
	if env.Error.Suggestion == "" {
		t.Fatal("expected suggestion")
	}
}

func TestFlagOr(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("source-branch", "", "")
	if got := flagOr(cmd, "source-branch", "main"); got != "main" {
		t.Fatalf("got %q", got)
	}
	_ = cmd.Flags().Set("source-branch", "dev")
	if got := flagOr(cmd, "source-branch", "main"); got != "dev" {
		t.Fatalf("got %q", got)
	}
}

func TestSetIf(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("task", "", "")
	q := url.Values{}
	setIf(cmd, q, "task", "task_key")
	if q.Has("task_key") {
		t.Fatal("empty flag should not set query")
	}
	_ = cmd.Flags().Set("task", "t1")
	setIf(cmd, q, "task", "task_key")
	if q.Get("task_key") != "t1" {
		t.Fatalf("q=%v", q)
	}
}

func TestScWiresCommonFlags(t *testing.T) {
	var fn runFunc = func(_ *client.Client, _ *config.Config, _ *cobra.Command, _ []string) (*output.Envelope, error) {
		return output.Success(nil, nil), nil
	}
	c := sc("+list", "desc", fn)
	for _, name := range []string{"key", "task", "run-id", "source-repo", "target-repo", "cron"} {
		if c.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}
