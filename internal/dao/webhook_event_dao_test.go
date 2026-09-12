package dao

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yi-nology/git-sync-service/sync/model"
	"gorm.io/gorm"
)

func setupWebhookEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "failed to open test db")

	err = db.AutoMigrate(&model.WebhookEvent{})
	require.NoError(t, err, "failed to migrate test db")

	return db
}

func TestWebhookEventDAO_CreateAndFindByID(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Source:    "github",
		ActorName: "testuser",
		Branch:    "main",
		CommitSHA: "abc123",
		Status:    "pending",
	}

	err := d.Create(event)
	require.NoError(t, err, "create failed")

	assert.NotZero(t, event.ID, "expected event ID to be set after create")

	// Find by ID
	got, err := d.FindByID(event.ID)
	require.NoError(t, err, "find by ID failed")

	assert.Equal(t, "evt-123", got.EventID, "expected event ID 'evt-123'")
	assert.Equal(t, "test-repo", got.RepoKey, "expected repo key 'test-repo'")
	assert.Equal(t, "push", got.EventType, "expected event type 'push'")
	assert.Equal(t, "github", got.Source, "expected source 'github'")
	assert.Equal(t, "testuser", got.ActorName, "expected actor name 'testuser'")
	assert.Equal(t, "main", got.Branch, "expected branch 'main'")
	assert.Equal(t, "abc123", got.CommitSHA, "expected commit SHA 'abc123'")
	assert.Equal(t, "pending", got.Status, "expected status 'pending'")
}

func TestWebhookEventDAO_FindByRepoKey(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create events with different repo keys
	events := []*model.WebhookEvent{
		{EventID: "evt-1", RepoKey: "repo1", EventType: "push", Status: "pending"},
		{EventID: "evt-2", RepoKey: "repo1", EventType: "push", Status: "processed"},
		{EventID: "evt-3", RepoKey: "repo2", EventType: "tag", Status: "failed"},
	}

	for _, event := range events {
		err := d.Create(event)
		require.NoError(t, err, "create failed")
	}

	// Find by repo key
	got, total, err := d.FindByRepoKey("repo1", DefaultPagination(0, 50))
	require.NoError(t, err, "find by repo key failed")

	require.Equal(t, int64(2), total, "expected 2 events")
	require.Len(t, got, 2, "expected 2 events")
}

func TestWebhookEventDAO_FindByEventID(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Status:    "pending",
	}

	err := d.Create(event)
	require.NoError(t, err, "create failed")

	// Find by event ID
	got, err := d.FindByEventID("evt-123")
	require.NoError(t, err, "find by event ID failed")

	assert.Equal(t, "evt-123", got.EventID, "expected event ID 'evt-123'")
	assert.Equal(t, "test-repo", got.RepoKey, "expected repo key 'test-repo'")
}

func TestWebhookEventDAO_Update(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create an event
	event := &model.WebhookEvent{
		EventID:   "evt-123",
		RepoKey:   "test-repo",
		EventType: "push",
		Status:    "pending",
	}

	err := d.Create(event)
	require.NoError(t, err, "create failed")

	// Update the event
	now := time.Now()
	event.Status = "processed"
	event.ProcessedAt = &now

	err = d.Update(event)
	require.NoError(t, err, "update failed")

	// Find by ID to verify update
	got, err := d.FindByID(event.ID)
	require.NoError(t, err, "find by ID failed")

	assert.Equal(t, "processed", got.Status, "expected status 'processed'")
	assert.NotNil(t, got.ProcessedAt, "expected processed at to be set")
}

func TestWebhookEventDAO_FindByID_NotFound(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Try to get a non-existent event
	got, err := d.FindByID(999)
	require.NoError(t, err, "unexpected error")

	assert.Nil(t, got, "expected nil when getting non-existent event")
}

func TestWebhookEventDAO_FindRecent(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create events
	events := []*model.WebhookEvent{
		{EventID: "evt-1", RepoKey: "repo1", Status: "pending"},
		{EventID: "evt-2", RepoKey: "repo1", Status: "processed"},
		{EventID: "evt-3", RepoKey: "repo2", Status: "failed"},
	}

	for _, event := range events {
		err := d.Create(event)
		require.NoError(t, err, "create failed")
	}

	// Find recent for repo1
	got, err := d.FindRecent("repo1", DefaultPagination(0, 50))
	require.NoError(t, err, "find recent failed")

	require.Len(t, got, 2, "expected 2 events")
}

func TestWebhookEventDAO_CountByRepoKey(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create events with different repo keys
	events := []*model.WebhookEvent{
		{EventID: "evt-1", RepoKey: "repo1", Status: "pending"},
		{EventID: "evt-2", RepoKey: "repo1", Status: "processed"},
		{EventID: "evt-3", RepoKey: "repo2", Status: "failed"},
	}

	for _, event := range events {
		err := d.Create(event)
		require.NoError(t, err, "create failed")
	}

	// Count by repo key
	count, err := d.CountByRepoKey("repo1")
	require.NoError(t, err, "count by repo key failed")

	assert.Equal(t, int64(2), count, "expected count 2")
}

func TestWebhookEventDAO_CleanupOlderThan(t *testing.T) {
	db := setupWebhookEventTestDB(t)
	d := NewWebhookEventDAO(db)

	// Create events with different ages
	oldEvent := &model.WebhookEvent{
		EventID:   "evt-1",
		RepoKey:   "repo1",
		Status:    "processed",
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	newEvent := &model.WebhookEvent{
		EventID:   "evt-2",
		RepoKey:   "repo1",
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	err := db.Create(oldEvent).Error
	require.NoError(t, err, "create old event failed")

	err = db.Create(newEvent).Error
	require.NoError(t, err, "create new event failed")

	// Cleanup older than 24 hours
	count, err := d.CleanupOlderThan(context.Background(), 24 * time.Hour)
	require.NoError(t, err, "cleanup failed")

	assert.Equal(t, int64(1), count, "expected count 1")

	// Verify only new event remains
	got, err := d.FindRecent("repo1", DefaultPagination(0, 50))
	require.NoError(t, err, "find recent failed")

	require.Len(t, got, 1, "expected 1 event")

	assert.Equal(t, "evt-2", got[0].EventID, "expected event ID 'evt-2'")
}

func TestWebhookEventDAO_Fields(t *testing.T) {
	now := time.Now()
	event := model.WebhookEvent{
		ID:           1,
		EventID:      "evt-123",
		RepoKey:      "test-repo",
		EventType:    "push",
		Source:       "github",
		ActorName:    "testuser",
		Branch:       "main",
		CommitSHA:    "abc123",
		Status:       "processed",
		ErrorMessage: "",
		ProcessedAt:  &now,
		CreatedAt:    now,
	}

	assert.Equal(t, uint(1), event.ID, "expected ID 1")
	assert.Equal(t, "evt-123", event.EventID, "expected event ID 'evt-123'")
	assert.Equal(t, "test-repo", event.RepoKey, "expected repo key 'test-repo'")
	assert.Equal(t, "push", event.EventType, "expected event type 'push'")
	assert.Equal(t, "github", event.Source, "expected source 'github'")
	assert.Equal(t, "testuser", event.ActorName, "expected actor name 'testuser'")
	assert.Equal(t, "main", event.Branch, "expected branch 'main'")
	assert.Equal(t, "abc123", event.CommitSHA, "expected commit SHA 'abc123'")
	assert.Equal(t, "processed", event.Status, "expected status 'processed'")
	assert.Equal(t, "", event.ErrorMessage, "expected empty error message")
	assert.NotNil(t, event.ProcessedAt, "expected processed at to be set")
}
