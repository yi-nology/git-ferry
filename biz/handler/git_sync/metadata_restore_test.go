package git_sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yi-nology/git-ferry/biz/model/ops"
	sdkprov "github.com/yi-nology/go-git-platform/provider"
)

func TestNormalizeRestoreKinds_EmptyUsesAvailable(t *testing.T) {
	snap := &ops.MetadataSnapshot{Files: []string{"labels.json", "issues.json", "releases.json"}}
	got := normalizeRestoreKinds(nil, snap)
	if len(got) != 3 {
		t.Fatalf("got %v want 3 kinds", got)
	}
	want := map[string]bool{"labels": true, "issues": true, "releases": true}
	for _, k := range got {
		if !want[k] {
			t.Fatalf("unexpected kind %s", k)
		}
	}
}

func TestNormalizeRestoreKinds_Explicit(t *testing.T) {
	snap := &ops.MetadataSnapshot{Files: []string{"labels.json", "issues.json", "pull_requests.json"}}
	got := normalizeRestoreKinds([]string{"Labels", " prs ", ""}, snap)
	if len(got) != 2 || got[0] != "labels" || got[1] != "prs" {
		t.Fatalf("got %v", got)
	}
}

func TestLatestMetadataSnapshot_PicksNewest(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "metadata", "r1")
	for _, d := range []string{"20260101-000000", "20260102-120000", "20260102-080000"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	got, err := latestMetadataSnapshot(root, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "20260102-120000" {
		t.Fatalf("got %s", got)
	}
}

func TestLatestMetadataSnapshot_Missing(t *testing.T) {
	if _, err := latestMetadataSnapshot(t.TempDir(), "nope"); err == nil {
		t.Fatal("want error")
	}
}

func TestLoadMetadataSnapshot(t *testing.T) {
	dir := t.TempDir()
	body := `{"repo_key":"r1","platform":"github","owner":"o","repo":"r","counts":{"labels":1},"files":["labels.json"]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, err := loadMetadataSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap.RepoKey != "r1" || snap.Counts["labels"] != 1 {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestIssueBodyHasRestoreMarker(t *testing.T) {
	b := issueBody(&issueRow{Number: "12", Title: "t", Body: "hello", Author: "alice", WebURL: "https://x/1"})
	for _, want := range []string{"gitferry-restore:issue", "hello", "alice", "https://x/1"} {
		if !strings.Contains(b, want) {
			t.Fatalf("body missing %q: %s", want, b)
		}
	}
}

func TestPRBodyHasRestoreMarker(t *testing.T) {
	b := prBody(&sdkprov.ChangeRequest{
		Number: "3", SourceBranch: "feat", TargetBranch: "main", Description: "desc",
	})
	for _, want := range []string{"gitferry-restore:pr", "feat", "main", "desc"} {
		if !strings.Contains(b, want) {
			t.Fatalf("body missing %q: %s", want, b)
		}
	}
}

func TestFilterIssuesSince(t *testing.T) {
	issues := []*issueRow{
		{Title: "old", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Title: "new", UpdatedAt: "2026-06-01T00:00:00Z"},
	}
	got := filterIssuesSince(issues, "2026-03-01T00:00:00Z")
	if len(got) != 1 || got[0].Title != "new" {
		t.Fatalf("got %+v", got)
	}
	// invalid since → 原样返回
	if len(filterIssuesSince(issues, "bad")) != 2 {
		t.Fatal("invalid since should pass through")
	}
}
