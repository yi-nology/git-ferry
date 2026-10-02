package tools

import (
	"strings"
	"testing"

	"github.com/yi-nology/git-ferry-core/health"
)

func TestWeakDimensionNames(t *testing.T) {
	dims := []health.Dimension{
		{Name: "reliability", Score: 40},
		{Name: "freshness", Score: 90},
		{Name: "safety", Score: 55},
	}
	got := weakDimensionNames(dims)
	if len(got) != 2 || got[0] != "reliability" || got[1] != "safety" {
		t.Fatalf("got %v", got)
	}
}

func TestRankWeakDims(t *testing.T) {
	got := rankWeakDims(map[string]int{"safety": 2, "reliability": 3, "freshness": 1})
	if len(got) != 3 {
		t.Fatalf("got %v", got)
	}
	// 按 count 降序
	if got[0]["dimension"] != "reliability" || got[0]["tasks"] != 3 {
		t.Fatalf("first=%v", got[0])
	}
}

func TestSummarizeAnalyze_WithWeakDims(t *testing.T) {
	s := summarizeAnalyze("为何失败", []analyzeHit{{ErrType: "auth", RunID: 1}}, map[string]int{"safety": 2})
	if !strings.Contains(s, "auth") || !strings.Contains(s, "safety") {
		t.Fatalf("summary=%q", s)
	}
}

func TestNextActions_DimensionDriven(t *testing.T) {
	acts := nextActions(nil, map[string]int{"reliability": 1, "freshness": 1})
	if len(acts) == 0 {
		t.Fatal("expected actions")
	}
	joined := strings.Join(acts, " ")
	if !strings.Contains(joined, "diagnose") || !strings.Contains(joined, "cron") {
		t.Fatalf("acts=%v", acts)
	}
}

func TestNextActions_None(t *testing.T) {
	acts := nextActions(nil, nil)
	if len(acts) != 1 || acts[0] != "无需处理" {
		t.Fatalf("acts=%v", acts)
	}
}
