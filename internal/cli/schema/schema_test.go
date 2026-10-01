package schema

import (
	"strings"
	"testing"
)

func TestLoadAndList(t *testing.T) {
	domains, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) == 0 {
		t.Fatal("expected domains")
	}
	foundSync := false
	for _, d := range domains {
		if d.Name == "sync" {
			foundSync = true
			if len(d.Endpoints) == 0 {
				t.Fatal("sync domain has no endpoints")
			}
		}
	}
	if !foundSync {
		t.Fatal("expected sync domain")
	}
}

func TestShowDomain(t *testing.T) {
	eps, err := Show("ops")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) < 5 {
		t.Fatalf("ops endpoints too few: %d", len(eps))
	}
}

func TestShowKeyword(t *testing.T) {
	eps, err := Show("diagnose")
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for _, e := range eps {
		if strings.Contains(e.Path, "diagnose") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("no diagnose endpoint in %+v", eps)
	}
}

func TestShowMissing(t *testing.T) {
	if _, err := Show("no-such-thing-xyz"); err == nil {
		t.Fatal("expected error")
	}
}
