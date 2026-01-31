package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) (*Store, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Override the database path for testing
	dbPath := filepath.Join(tmpDir, "test-history.sqlite3")
	db, err := openPath(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open test db: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// openPath is like Open but with a specific path (for testing).
func openPath(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func TestSaveAndLoadHistory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save entries
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	err = store.SaveEntry("project-b", "/home/user/project-b", "atmux-project-b")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Load history
	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify both entries exist (order may vary when timestamps are equal)
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["project-a"] || !names["project-b"] {
		t.Errorf("expected both projects in history, got %v", names)
	}
}

func TestRecencyOrder(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save entry
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Update the same entry to bump last_used_at
	// Then add a new entry - since update happens on existing entry,
	// we need to add another entry first, then touch the first one
	err = store.SaveEntry("project-b", "/home/user/project-b", "atmux-project-b")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Touch project-a again to make it most recent
	err = store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Load history - project-a should be first since it was touched last
	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// When entry is updated (touched), its last_used_at is updated
	// So project-a should now be first
	if entries[0].Name != "project-a" {
		t.Errorf("expected project-a first after touch, got %s", entries[0].Name)
	}
}

func TestUpdateExistingEntry(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save entry
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Save same entry again (should update, not duplicate)
	err = store.SaveEntry("project-a-renamed", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	count, err := store.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 entry after update, got %d", count)
	}

	// Verify name was updated
	entry, err := store.GetBySessionName("atmux-project-a")
	if err != nil {
		t.Fatalf("GetBySessionName failed: %v", err)
	}

	if entry.Name != "project-a-renamed" {
		t.Errorf("expected updated name, got %s", entry.Name)
	}
}

func TestDeleteEntry(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save entry
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	entries, _ := store.LoadHistory()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Delete by ID
	err = store.DeleteEntry(entries[0].ID)
	if err != nil {
		t.Fatalf("DeleteEntry failed: %v", err)
	}

	count, _ := store.Count()
	if count != 0 {
		t.Errorf("expected 0 entries after delete, got %d", count)
	}
}

func TestClearHistory(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save multiple entries
	store.SaveEntry("a", "/a", "atmux-a")
	store.SaveEntry("b", "/b", "atmux-b")
	store.SaveEntry("c", "/c", "atmux-c")

	count, _ := store.Count()
	if count != 3 {
		t.Fatalf("expected 3 entries, got %d", count)
	}

	// Clear all
	err := store.ClearHistory()
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	count, _ = store.Count()
	if count != 0 {
		t.Errorf("expected 0 entries after clear, got %d", count)
	}
}

func TestGetBySessionName(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Non-existent entry
	entry, err := store.GetBySessionName("nonexistent")
	if err != nil {
		t.Fatalf("GetBySessionName failed: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil for nonexistent entry")
	}

	// Save and retrieve
	store.SaveEntry("project", "/home/user/project", "atmux-project")

	entry, err = store.GetBySessionName("atmux-project")
	if err != nil {
		t.Fatalf("GetBySessionName failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.Name != "project" {
		t.Errorf("expected name 'project', got '%s'", entry.Name)
	}
}
