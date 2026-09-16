package converter

import (
	"testing"
	"time"

	"github.com/yi-nology/git-sync-core/model"
)

func TestToOperationLogInfo(t *testing.T) {
	now := time.Now()
	l := &model.OperationLog{
		ID:        1,
		Actor:     "testuser",
		Action:    "create",
		Resource:  "repo",
		Detail:    "Created repository test-repo",
		IP:        "192.168.1.1",
		CreatedAt: now,
	}

	result := ToOperationLogInfo(l)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ID != 1 {
		t.Errorf("Expected ID 1, got %d", result.ID)
	}

	if result.User != "testuser" {
		t.Errorf("Expected User 'testuser', got '%s'", result.User)
	}

	if result.Action != "create" {
		t.Errorf("Expected Action 'create', got '%s'", result.Action)
	}

	if result.Resource != "repo" {
		t.Errorf("Expected Resource 'repo', got '%s'", result.Resource)
	}

	if result.Details != "Created repository test-repo" {
		t.Errorf("Expected Details 'Created repository test-repo', got '%s'", result.Details)
	}

	if result.IP != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got '%s'", result.IP)
	}
}

func TestToOperationLogInfoNil(t *testing.T) {
	result := ToOperationLogInfo(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestToOperationLogList(t *testing.T) {
	logs := []*model.OperationLog{
		{ID: 1, Actor: "user1", Action: "create"},
		{ID: 2, Actor: "user2", Action: "update"},
	}

	result := ToOperationLogList(logs)

	if len(result) != 2 {
		t.Fatalf("Expected 2 logs, got %d", len(result))
	}

	if result[0].ID != 1 {
		t.Errorf("Expected first log ID 1, got %d", result[0].ID)
	}

	if result[1].ID != 2 {
		t.Errorf("Expected second log ID 2, got %d", result[1].ID)
	}
}

func TestToOperationLogListEmpty(t *testing.T) {
	result := ToOperationLogList([]*model.OperationLog{})
	if len(result) != 0 {
		t.Errorf("Expected empty list, got %d items", len(result))
	}
}
