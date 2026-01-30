# Repository Guidelines

## Project Structure & Module Organization
- `cmd/`: Cobra CLI commands (`root.go`, `list.go`, `attach.go`, etc.).
- `tmux/`: Session orchestration and tmux command execution (`session.go`).
- `config/`: Parser and templates for `.agent-tmux.conf` (`parser.go`).
- `main.go`: Entrypoint wiring the CLI.
- `homebrew/`: Homebrew formula (`agent-tmux.rb`).
- Top-level `Makefile`, `go.mod`, `go.sum` define build and dependencies.

## Build, Test, and Development Commands
- `make build`: Build the `agent-tmux` binary with version metadata.
- `make install`: Install to `/usr/local/bin/agent-tmux`.
- `make install-home`: Install to `~/bin/agent-tmux` (or `~/bin/agent-tmux-cli` symlink if a dir exists).
- `make release`: Cross-build binaries into `dist/`.
- `make test`: Run Go tests (`go test ./...`).

## Coding Style & Naming Conventions
- Go code is formatted with standard `gofmt` (tabs for indentation).
- Package naming is short and lowercase (`cmd`, `tmux`, `config`).
- File naming follows Go conventions (lowercase, underscores when needed).
- Prefer clear, imperative function names (e.g., `Create`, `Attach`, `ApplyConfig`).

## Testing Guidelines
- Tests run via `go test ./...` (see `make test`).
- There are currently no `_test.go` files; add tests alongside packages as needed.
- Name test files `*_test.go` and test functions `TestXxx`.

## Commit & Pull Request Guidelines
- Commit history uses short, descriptive sentences (e.g., “Add gitignore and fix Makefile install targets”).
- Prefer small, focused commits that describe the change outcome.
- PRs should include: a brief summary, key commands run (e.g., `make test`), and notes on behavior changes. Add screenshots only if CLI output formatting changes.

## Configuration & Runtime Notes
- Project-specific behavior is configured in `.agent-tmux.conf` in the project root.
- The default config template is generated via `agent-tmux init`.
- The diagnostics script is resolved from common install locations (e.g., `/usr/local/bin/agent-tmux-diag.sh`).

## CLI Feature Notes
- `agent-tmux sessions` is an interactive list with click-to-attach behavior.
- `agent-tmux browse` opens a tree-based TUI with pane previews and command sending.
- `agent-tmux browse --popup` uses a tmux popup overlay (tmux 3.2+).

<!-- bv-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses beads (`bd`) and the helper beads viewer (`bv`).

### Essential Commands

```bash
# View issues (launches TUI - avoid in automated sessions)
bv

# CLI commands for agents (use these instead)
bd ready              # Show issues ready to work (no blockers)
bd list --status=open # All open issues
bd show <id>          # Full issue details with dependencies
bd create --title="..." --type=task --priority=2
bd update <id> --status=in_progress
bd close <id> --reason="Completed"
bd close <id1> <id2>  # Close multiple issues at once
bd sync               # Commit and push changes
```

### Workflow Pattern

1. **Start**: Run `bd ready` to find actionable work
2. **Claim**: Use `bd update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `bd close <id>`
5. **Sync**: Always run `bd sync` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `bd ready` shows only unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers, not words)
- **Types**: task, bug, feature, epic, question, docs
- **Blocking**: `bd dep add <issue> <depends-on>` to add dependencies

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
bd sync                 # Commit beads changes
git commit -m "..."     # Commit code
bd sync                 # Commit any new beads changes
git push                # Push to remote
```

### Best Practices

- Check `bd ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `bd create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always `bd sync` before ending session

<!-- end-bv-agent-instructions -->
