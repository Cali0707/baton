package store

import (
	"context"
	"testing"
	"time"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("OpenDB() error: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testInboxItem(sourceID string) *InboxItem {
	number := 42
	now := time.Now().UTC().Truncate(time.Second)
	return &InboxItem{
		SourceType:      "github",
		SourceID:        sourceID,
		Kind:            "issue",
		Number:          &number,
		Title:           "Test issue",
		Body:            "Issue body",
		Author:          "alice",
		Labels:          `["bug"]`,
		Owner:           "org",
		Repo:            "repo",
		Metadata:        `{}`,
		Status:          ItemStatusNew,
		SourceUpdatedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func TestUpsertAndGetItem(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	item := testInboxItem("github:org/repo:issue:42")

	if err := db.UpsertItem(ctx, item); err != nil {
		t.Fatalf("UpsertItem() error: %v", err)
	}
	if item.ID == 0 {
		t.Fatal("UpsertItem() should set item.ID")
	}

	loaded, err := db.GetItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetItem() error: %v", err)
	}
	if loaded.Title != "Test issue" {
		t.Errorf("Title = %q, want %q", loaded.Title, "Test issue")
	}
	if loaded.Status != ItemStatusNew {
		t.Errorf("Status = %q, want %q", loaded.Status, ItemStatusNew)
	}
	if loaded.Number == nil || *loaded.Number != 42 {
		t.Errorf("Number = %v, want 42", loaded.Number)
	}
}

func TestUpsertItem_Dedup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item1 := testInboxItem("github:org/repo:issue:42")
	item1.Title = "Original title"
	if err := db.UpsertItem(ctx, item1); err != nil {
		t.Fatalf("first UpsertItem() error: %v", err)
	}
	firstID := item1.ID

	// Upsert with same source_id but different title.
	item2 := testInboxItem("github:org/repo:issue:42")
	item2.Title = "Updated title"
	if err := db.UpsertItem(ctx, item2); err != nil {
		t.Fatalf("second UpsertItem() error: %v", err)
	}

	// Should have same ID (upserted, not duplicated).
	if item2.ID != firstID {
		t.Errorf("second upsert ID = %d, want %d (same row)", item2.ID, firstID)
	}

	loaded, _ := db.GetItem(ctx, firstID)
	if loaded.Title != "Updated title" {
		t.Errorf("Title = %q, want 'Updated title'", loaded.Title)
	}
}

func TestUpsertItem_PreservesStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	// Archive the item.
	db.UpdateItemStatus(ctx, item.ID, ItemStatusArchived)

	// Re-upsert (simulating a sync).
	item2 := testInboxItem("github:org/repo:issue:42")
	item2.Title = "New title from sync"
	db.UpsertItem(ctx, item2)

	loaded, _ := db.GetItem(ctx, item.ID)
	if loaded.Status != ItemStatusArchived {
		t.Errorf("Status = %q after re-upsert, want %q (should be preserved)", loaded.Status, ItemStatusArchived)
	}
	if loaded.Title != "New title from sync" {
		t.Errorf("Title = %q, want 'New title from sync' (should be updated)", loaded.Title)
	}
}

func TestGetItemBySourceID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:99")
	db.UpsertItem(ctx, item)

	loaded, err := db.GetItemBySourceID(ctx, "github:org/repo:issue:99")
	if err != nil {
		t.Fatalf("GetItemBySourceID() error: %v", err)
	}
	if loaded.ID != item.ID {
		t.Errorf("ID = %d, want %d", loaded.ID, item.ID)
	}
}

func TestListItems_FiltersByStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item1 := testInboxItem("github:org/repo:issue:1")
	item1.Status = ItemStatusNew
	db.UpsertItem(ctx, item1)

	item2 := testInboxItem("github:org/repo:issue:2")
	item2.Status = ItemStatusNew
	db.UpsertItem(ctx, item2)
	db.UpdateItemStatus(ctx, item2.ID, ItemStatusArchived)

	// List active items only.
	active, err := db.ListItems(ctx, []ItemStatus{ItemStatusNew, ItemStatusInProgress})
	if err != nil {
		t.Fatalf("ListItems() error: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("ListItems(new,in_progress) = %d items, want 1", len(active))
	}

	// List archived.
	archived, err := db.ListItems(ctx, []ItemStatus{ItemStatusArchived})
	if err != nil {
		t.Fatalf("ListItems(archived) error: %v", err)
	}
	if len(archived) != 1 {
		t.Errorf("ListItems(archived) = %d items, want 1", len(archived))
	}
}

func TestDeleteItem_CascadesToRuns(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	run := &Run{
		InboxItemID:  item.ID,
		WorkflowType: "bug",
		AgentName:    "claude",
		Status:       StatusCompleted,
		StartedAt:    time.Now().UTC(),
	}
	db.CreateRun(ctx, run)

	if err := db.DeleteItem(ctx, item.ID); err != nil {
		t.Fatalf("DeleteItem() error: %v", err)
	}

	// Item should be gone.
	_, err := db.GetItem(ctx, item.ID)
	if err == nil {
		t.Error("GetItem() should fail after DeleteItem()")
	}

	// Runs should also be gone.
	runs, _ := db.ListRunsForItem(ctx, item.ID)
	if len(runs) != 0 {
		t.Errorf("ListRunsForItem() = %d runs after delete, want 0", len(runs))
	}
}

func TestCreateAndGetRun(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	run := &Run{
		InboxItemID:  item.ID,
		WorkflowType: "bug",
		AgentName:    "claude",
		Status:       StatusRunning,
		StartedAt:    time.Now().UTC(),
	}
	if err := db.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("CreateRun() should set run.ID")
	}

	loaded, err := db.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error: %v", err)
	}
	if loaded.WorkflowType != "bug" {
		t.Errorf("WorkflowType = %q, want 'bug'", loaded.WorkflowType)
	}
	if loaded.InboxItemID != item.ID {
		t.Errorf("InboxItemID = %d, want %d", loaded.InboxItemID, item.ID)
	}
}

func TestUpdateRun(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	run := &Run{
		InboxItemID:  item.ID,
		WorkflowType: "bug",
		AgentName:    "claude",
		Status:       StatusRunning,
		StartedAt:    time.Now().UTC(),
	}
	db.CreateRun(ctx, run)

	now := time.Now().UTC()
	run.Status = StatusCompleted
	run.CompletedAt = &now
	run.AgentSessionID = "agent-abc-123"

	if err := db.UpdateRun(ctx, run); err != nil {
		t.Fatalf("UpdateRun() error: %v", err)
	}

	loaded, _ := db.GetRun(ctx, run.ID)
	if loaded.Status != StatusCompleted {
		t.Errorf("Status = %q, want 'completed'", loaded.Status)
	}
	if loaded.AgentSessionID != "agent-abc-123" {
		t.Errorf("AgentSessionID = %q, want 'agent-abc-123'", loaded.AgentSessionID)
	}
	if loaded.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestListRunsForItem(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	for i := 0; i < 3; i++ {
		run := &Run{
			InboxItemID:  item.ID,
			WorkflowType: "bug",
			AgentName:    "claude",
			Status:       StatusCompleted,
			StartedAt:    time.Now().UTC(),
		}
		db.CreateRun(ctx, run)
	}

	runs, err := db.ListRunsForItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("ListRunsForItem() error: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("ListRunsForItem() = %d runs, want 3", len(runs))
	}
}

func TestListRuns_FiltersByStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	item := testInboxItem("github:org/repo:issue:42")
	db.UpsertItem(ctx, item)

	r1 := &Run{InboxItemID: item.ID, WorkflowType: "bug", Status: StatusCompleted, StartedAt: time.Now().UTC()}
	r2 := &Run{InboxItemID: item.ID, WorkflowType: "bug", Status: StatusFailed, StartedAt: time.Now().UTC()}
	r3 := &Run{InboxItemID: item.ID, WorkflowType: "bug", Status: StatusRunning, StartedAt: time.Now().UTC()}
	db.CreateRun(ctx, r1)
	db.CreateRun(ctx, r2)
	db.CreateRun(ctx, r3)

	completed, _ := db.ListRuns(ctx, []SessionStatus{StatusCompleted, StatusFailed})
	if len(completed) != 2 {
		t.Errorf("ListRuns(completed,failed) = %d, want 2", len(completed))
	}
}

func TestOutputEntries(t *testing.T) {
	db := testDB(t)

	// No file yet.
	entries, err := db.LoadEntries(999)
	if err != nil {
		t.Fatalf("LoadEntries() error for missing file: %v", err)
	}
	if entries != nil {
		t.Errorf("LoadEntries() = %v, want nil", entries)
	}

	// Write entries and read them back.
	db.AppendEntry(1, OutputEntry{Type: "thought", Text: "analyzing"})
	db.AppendEntry(1, OutputEntry{Type: "message", Text: "hello world"})

	entries, err = db.LoadEntries(1)
	if err != nil {
		t.Fatalf("LoadEntries() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("LoadEntries() returned %d entries, want 2", len(entries))
	}
	if entries[0].Type != "thought" || entries[0].Text != "analyzing" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].Type != "message" || entries[1].Text != "hello world" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

func TestOpenDB_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(dir)
	if err != nil {
		t.Fatalf("OpenDB() error: %v", err)
	}
	defer db.Close()

	// Should be able to query inbox_items.
	ctx := context.Background()
	items, err := db.ListItems(ctx, []ItemStatus{ItemStatusNew})
	if err != nil {
		t.Fatalf("ListItems() error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("ListItems() = %d, want 0", len(items))
	}
}
