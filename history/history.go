// Package history provides agent session history storage using SQLite.
package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	schemaVersion = 2
	maxHistory    = 100 // Maximum entries before LRU eviction
)

// Entry represents a single agent history entry.
type Entry struct {
	ID               int64
	Name             string
	WorkingDirectory string
	SessionName      string
	Host             string // Remote host label ("" = local)
	AttachMethod     string // "ssh" or "mosh" ("" = local/ssh default)
	CreatedAt        time.Time
	LastUsedAt       time.Time
}

// Store manages the history database.
type Store struct {
	db *sql.DB
}

// DataDir returns the user data directory for atmux.
func DataDir() (string, error) {
	var base string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, "Library", "Application Support")
	case "windows":
		base = os.Getenv("APPDATA")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, "AppData", "Roaming")
		}
	default: // Linux and others
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
	}
	return filepath.Join(base, "atmux"), nil
}

// DBPath returns the full path to the history database.
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.sqlite3"), nil
}

// Open opens the history store, creating the database if needed.
func Open() (*Store, error) {
	dbPath, err := DBPath()
	if err != nil {
		return nil, err
	}

	// Ensure directory exists
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

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate ensures the schema is up to date.
func (s *Store) migrate() error {
	var version int
	err := s.db.QueryRow("PRAGMA user_version").Scan(&version)
	if err != nil {
		return err
	}

	if version < 1 {
		// Initial schema
		_, err := s.db.Exec(`
			CREATE TABLE IF NOT EXISTS agent_history (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				working_directory TEXT NOT NULL,
				session_name TEXT NOT NULL,
				host TEXT NOT NULL DEFAULT '',
				attach_method TEXT NOT NULL DEFAULT 'ssh',
				created_at INTEGER NOT NULL,
				last_used_at INTEGER NOT NULL
			);

			CREATE UNIQUE INDEX IF NOT EXISTS agent_history_unique
				ON agent_history (session_name, working_directory, host);

			CREATE INDEX IF NOT EXISTS agent_history_last_used
				ON agent_history (last_used_at DESC);

			CREATE INDEX IF NOT EXISTS agent_history_name
				ON agent_history (name);

			PRAGMA user_version = 2;
		`)
		if err != nil {
			return err
		}
	}

	if version == 1 {
		// Migration from v1 to v2: add host and attach_method columns,
		// update unique index to include host.
		_, err := s.db.Exec(`
			ALTER TABLE agent_history ADD COLUMN host TEXT NOT NULL DEFAULT '';
			ALTER TABLE agent_history ADD COLUMN attach_method TEXT NOT NULL DEFAULT 'ssh';

			DROP INDEX IF EXISTS agent_history_unique;
			CREATE UNIQUE INDEX agent_history_unique
				ON agent_history (session_name, working_directory, host);

			PRAGMA user_version = 2;
		`)
		if err != nil {
			return err
		}
	}

	return nil
}

// SaveEntry inserts or updates an agent history entry.
// If an entry with the same session_name, working_directory, and host exists,
// it updates last_used_at. Otherwise, it inserts a new entry.
// An empty host means a local session.
func (s *Store) SaveEntry(name, workingDir, sessionName, host, attachMethod string) error {
	now := time.Now().Unix()
	if attachMethod == "" {
		attachMethod = "ssh"
	}

	// Try to update existing entry first
	result, err := s.db.Exec(`
		UPDATE agent_history
		SET name = ?, last_used_at = ?, attach_method = ?
		WHERE session_name = ? AND working_directory = ? AND host = ?
	`, name, now, attachMethod, sessionName, workingDir, host)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		// Insert new entry
		_, err = s.db.Exec(`
			INSERT INTO agent_history (name, working_directory, session_name, host, attach_method, created_at, last_used_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, name, workingDir, sessionName, host, attachMethod, now, now)
		if err != nil {
			return err
		}
	}

	// Enforce max history limit (LRU eviction)
	return s.enforceLimitLRU()
}

// enforceLimitLRU removes oldest entries if over the limit.
func (s *Store) enforceLimitLRU() error {
	_, err := s.db.Exec(`
		DELETE FROM agent_history
		WHERE id NOT IN (
			SELECT id FROM agent_history
			ORDER BY last_used_at DESC
			LIMIT ?
		)
	`, maxHistory)
	return err
}

// LoadHistory returns all entries, most recently used first.
func (s *Store) LoadHistory() ([]Entry, error) {
	rows, err := s.db.Query(`
		SELECT id, name, working_directory, session_name, host, attach_method, created_at, last_used_at
		FROM agent_history
		ORDER BY last_used_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		var createdAt, lastUsedAt int64
		if err := rows.Scan(&e.ID, &e.Name, &e.WorkingDirectory, &e.SessionName, &e.Host, &e.AttachMethod, &createdAt, &lastUsedAt); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		e.LastUsedAt = time.Unix(lastUsedAt, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteEntry removes an entry by ID.
func (s *Store) DeleteEntry(id int64) error {
	_, err := s.db.Exec("DELETE FROM agent_history WHERE id = ?", id)
	return err
}

// DeleteBySessionName removes an entry by session name.
func (s *Store) DeleteBySessionName(sessionName string) error {
	_, err := s.db.Exec("DELETE FROM agent_history WHERE session_name = ?", sessionName)
	return err
}

// ClearHistory removes all entries.
func (s *Store) ClearHistory() error {
	_, err := s.db.Exec("DELETE FROM agent_history")
	return err
}

// GetBySessionName finds an entry by session name.
func (s *Store) GetBySessionName(sessionName string) (*Entry, error) {
	row := s.db.QueryRow(`
		SELECT id, name, working_directory, session_name, host, attach_method, created_at, last_used_at
		FROM agent_history
		WHERE session_name = ?
	`, sessionName)

	var e Entry
	var createdAt, lastUsedAt int64
	err := row.Scan(&e.ID, &e.Name, &e.WorkingDirectory, &e.SessionName, &e.Host, &e.AttachMethod, &createdAt, &lastUsedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.CreatedAt = time.Unix(createdAt, 0)
	e.LastUsedAt = time.Unix(lastUsedAt, 0)
	return &e, nil
}

// Count returns the number of entries in history.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM agent_history").Scan(&count)
	return count, err
}
