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
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a", "", "")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	err = store.SaveEntry("project-b", "/home/user/project-b", "atmux-project-b", "", "")
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
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a", "", "")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Update the same entry to bump last_used_at
	// Then add a new entry first, then touch the first one
	err = store.SaveEntry("project-b", "/home/user/project-b", "atmux-project-b", "", "")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Touch project-a again to make it most recent
	err = store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a", "", "")
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
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a", "", "")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	// Save same entry again (should update, not duplicate)
	err = store.SaveEntry("project-a-renamed", "/home/user/project-a", "atmux-project-a", "", "")
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
	err := store.SaveEntry("project-a", "/home/user/project-a", "atmux-project-a", "", "")
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
	store.SaveEntry("a", "/a", "atmux-a", "", "")
	store.SaveEntry("b", "/b", "atmux-b", "", "")
	store.SaveEntry("c", "/c", "atmux-c", "", "")

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
	store.SaveEntry("project", "/home/user/project", "atmux-project", "", "")

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

func TestSaveEntryWithHost(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save a local entry
	err := store.SaveEntry("local-project", "/home/user/project", "atmux-project", "", "")
	if err != nil {
		t.Fatalf("SaveEntry (local) failed: %v", err)
	}

	// Save a remote entry with same session name but different host
	err = store.SaveEntry("remote-project", "/home/user/project", "atmux-project", "devbox", "ssh")
	if err != nil {
		t.Fatalf("SaveEntry (remote) failed: %v", err)
	}

	// Should have 2 entries (different hosts)
	count, err := store.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 entries (local + remote), got %d", count)
	}

	// Load and verify host info
	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	hosts := make(map[string]string) // host -> name
	for _, e := range entries {
		hosts[e.Host] = e.Name
	}
	if hosts[""] != "local-project" {
		t.Errorf("expected local entry name 'local-project', got %q", hosts[""])
	}
	if hosts["devbox"] != "remote-project" {
		t.Errorf("expected remote entry name 'remote-project', got %q", hosts["devbox"])
	}
}

func TestSaveEntryRemoteAttachMethod(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save with mosh attach method
	err := store.SaveEntry("project", "", "atmux-project", "devbox", "mosh")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Host != "devbox" {
		t.Errorf("expected host 'devbox', got %q", entries[0].Host)
	}
	if entries[0].AttachMethod != "mosh" {
		t.Errorf("expected attach_method 'mosh', got %q", entries[0].AttachMethod)
	}
}

func TestSaveEntryDefaultAttachMethod(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save with empty attach method — should default to "ssh"
	err := store.SaveEntry("project", "", "atmux-project", "devbox", "")
	if err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AttachMethod != "ssh" {
		t.Errorf("expected default attach_method 'ssh', got %q", entries[0].AttachMethod)
	}
}

func TestMigrationV1ToLatest(t *testing.T) {
	// Create a v1 database manually
	tmpDir, err := os.MkdirTemp("", "history-migration-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test-history.sqlite3")
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create v1 schema manually
	_, err = db.Exec(`
		CREATE TABLE agent_history (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			working_directory TEXT NOT NULL,
			session_name TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX agent_history_unique
			ON agent_history (session_name, working_directory);
		CREATE INDEX agent_history_last_used
			ON agent_history (last_used_at DESC);
		CREATE INDEX agent_history_name
			ON agent_history (name);
		PRAGMA user_version = 1;
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create v1 schema: %v", err)
	}

	// Insert a v1 entry
	_, err = db.Exec(`
		INSERT INTO agent_history (name, working_directory, session_name, created_at, last_used_at)
		VALUES ('old-project', '/home/user/old', 'atmux-old', 1000, 2000)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert v1 entry: %v", err)
	}
	db.Close()

	// Now open with the Store which should auto-migrate to latest schema
	store, err := openPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open store (migration): %v", err)
	}
	defer store.Close()

	// Check that migration happened: user_version should be latest.
	var version int
	err = store.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("failed to read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after migration, got %d", schemaVersion, version)
	}

	// Load the old entry — host should be empty, attach_method should be 'ssh'
	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed after migration: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after migration, got %d", len(entries))
	}
	if entries[0].Name != "old-project" {
		t.Errorf("expected name 'old-project', got %q", entries[0].Name)
	}
	if entries[0].Host != "" {
		t.Errorf("expected empty host for migrated entry, got %q", entries[0].Host)
	}
	if entries[0].AttachMethod != "ssh" {
		t.Errorf("expected default attach_method 'ssh' for migrated entry, got %q", entries[0].AttachMethod)
	}

	// Verify we can now save a remote entry
	err = store.SaveEntry("remote-project", "/remote/dir", "atmux-remote", "server1", "mosh")
	if err != nil {
		t.Fatalf("SaveEntry (remote) after migration failed: %v", err)
	}

	entries, err = store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestMigrationV2ToV3AddsAttachMethod(t *testing.T) {
	// Create a v2-like database manually (has host, lacks attach_method).
	tmpDir, err := os.MkdirTemp("", "history-migration-v2-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test-history.sqlite3")
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE agent_history (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			working_directory TEXT NOT NULL,
			session_name TEXT NOT NULL,
			host TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			last_used_at INTEGER NOT NULL
		);
		CREATE UNIQUE INDEX agent_history_unique
			ON agent_history (session_name, working_directory, host);
		CREATE INDEX agent_history_last_used
			ON agent_history (last_used_at DESC);
		CREATE INDEX agent_history_name
			ON agent_history (name);
		PRAGMA user_version = 2;
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create v2 schema: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO agent_history (name, working_directory, session_name, host, created_at, last_used_at)
		VALUES ('old-project', '/home/user/old', 'atmux-old', '', 1000, 2000)
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to insert v2 entry: %v", err)
	}
	db.Close()

	store, err := openPath(dbPath)
	if err != nil {
		t.Fatalf("failed to open store (migration): %v", err)
	}
	defer store.Close()

	var version int
	err = store.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		t.Fatalf("failed to read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Errorf("expected schema version %d after migration, got %d", schemaVersion, version)
	}

	entries, err := store.LoadHistory()
	if err != nil {
		t.Fatalf("LoadHistory failed after migration: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after migration, got %d", len(entries))
	}
	if entries[0].AttachMethod != "ssh" {
		t.Errorf("expected default attach_method 'ssh' for migrated entry, got %q", entries[0].AttachMethod)
	}
}

func TestUniqueIndexWithHost(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Save same session name + working dir but different hosts
	err := store.SaveEntry("proj", "/dir", "atmux-proj", "", "")
	if err != nil {
		t.Fatalf("SaveEntry (local) failed: %v", err)
	}
	err = store.SaveEntry("proj", "/dir", "atmux-proj", "host-a", "ssh")
	if err != nil {
		t.Fatalf("SaveEntry (host-a) failed: %v", err)
	}
	err = store.SaveEntry("proj", "/dir", "atmux-proj", "host-b", "mosh")
	if err != nil {
		t.Fatalf("SaveEntry (host-b) failed: %v", err)
	}

	count, _ := store.Count()
	if count != 3 {
		t.Errorf("expected 3 entries (local + 2 remote hosts), got %d", count)
	}

	// Now update host-a entry — should not create a new row
	err = store.SaveEntry("proj-updated", "/dir", "atmux-proj", "host-a", "ssh")
	if err != nil {
		t.Fatalf("SaveEntry update failed: %v", err)
	}
	count, _ = store.Count()
	if count != 3 {
		t.Errorf("expected 3 entries after update, got %d", count)
	}
}
