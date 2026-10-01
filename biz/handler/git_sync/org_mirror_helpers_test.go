package git_sync

import (
	"testing"
)

func TestOrgMirrorResolveTarget(t *testing.T) {
	cases := []struct {
		strategy, owner, repo, org, user string
		wantOwner, wantRepo              string
	}{
		{"preserve", "acme", "api", "", "", "acme", "api"},
		{"single", "acme", "api", "backup", "", "backup", "api"},
		{"flat", "acme", "api", "", "mirror", "mirror", "api"},
		{"mixed", "alice", "dot", "backup", "alice", "alice", "dot"},
		{"mixed", "acme", "api", "backup", "alice", "backup", "api"},
	}
	for i, c := range cases {
		o, r := resolveOrgTarget(c.strategy, c.owner, c.repo, c.org, c.user)
		if o != c.wantOwner || r != c.wantRepo {
			t.Fatalf("case %d: got %s/%s want %s/%s", i, o, r, c.wantOwner, c.wantRepo)
		}
	}
}

func TestRewriteRepoURL(t *testing.T) {
	got := rewriteRepoURL("https://github.com/old/oldrepo.git", "new", "newrepo")
	want := "https://github.com/new/newrepo.git"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	got = rewriteRepoURL("git@github.com:old/oldrepo.git", "new", "newrepo")
	want = "git@github.com:new/newrepo.git"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
