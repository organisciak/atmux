# Agent history storage design

## Goals

- Track recently used agents for revival and quick attach.
- Keep storage lightweight, durable, and safe for concurrent CLI runs.
- Support future schema changes without breaking old installs.

## Storage format

Use SQLite in a single file. Rationale:

- Handles concurrent reads and writes safely.
- Allows efficient sorting and filtering by recency.
- Supports schema versioning and migrations.
- Avoids full-file rewrites and ad-hoc file locking.

## Location

Store history in the per-user data directory and keep config separate:

- Linux: `$XDG_DATA_HOME/agent-tmux/history.sqlite3`
  - Default: `~/.local/share/agent-tmux/history.sqlite3`
- macOS: `~/Library/Application Support/agent-tmux/history.sqlite3`
- Windows: `%AppData%\\agent-tmux\\history.sqlite3`

Notes:

- Config remains in `~/.config/agent-tmux/` (or the OS equivalent).
- Ensure the directory exists before opening the database.

## Schema (version 1)

Table: `agent_history`

- `id` INTEGER PRIMARY KEY
- `name` TEXT NOT NULL
- `working_directory` TEXT NOT NULL
- `session_name` TEXT NOT NULL
- `created_at` INTEGER NOT NULL (Unix seconds, UTC)
- `last_used_at` INTEGER NOT NULL (Unix seconds, UTC)

Indexes:

- Unique: `(session_name, working_directory)`
- Recency: `(last_used_at DESC)`
- Name lookup: `(name)`

Suggested DDL:

```sql
CREATE TABLE IF NOT EXISTS agent_history (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  working_directory TEXT NOT NULL,
  session_name TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_used_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS agent_history_unique
  ON agent_history (session_name, working_directory);

CREATE INDEX IF NOT EXISTS agent_history_last_used
  ON agent_history (last_used_at DESC);

CREATE INDEX IF NOT EXISTS agent_history_name
  ON agent_history (name);
```

## Versioning and migrations

- Use `PRAGMA user_version` to track schema version.
- Initialize `user_version` to `1` on first creation.
- Future schema changes should:
  - Read `user_version`.
  - Apply migrations in order.
  - Update `user_version` after each migration.

## Behavior notes

- Insert a row on first creation of a session.
- Update `last_used_at` on each attach or reuse event.
- Keep `created_at` immutable after initial insert.
