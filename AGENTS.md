# Repository Guidelines

## Improvements to AGENTS.md

Regularly check if AGENTS.md is up to date, and update if it's not. If you learn new useful things that help in understanding the development, add it here, and commit those updates along with the related change. Keep secrets, credentials and personal data out of it.

## Agent Orchestration
- When spawning parallel task agents across worktrees, use lightweight/focused agents rather than heavyweight exploratory ones. Agents should start implementing immediately, not spend time exploring the codebase.
- Always provide agents with specific file paths and clear scope to avoid excessive exploration phases.
- Before starting parallel agent work, verify that target issues/tasks are still open and in the expected state. Do not assume labels or issue states from previous sessions are current.
- When working with multiple worktree branches, proactively analyze file overlap between tasks BEFORE spawning agents. Plan merge order to minimize conflicts. After merging, always run tests before pushing.

## BEADS Workflow
- BEADS tasks should be executed in parallel using Task agents (not Teammate agents) across separate git worktrees.
- Each agent gets one task, one worktree. After completion, merge sequentially, resolve conflicts, run tests, then push.

## Project Structure & Module Organization
- `cmd/`: Cobra CLI commands (`root.go`, `list.go`, `attach.go`, etc.).
- `tmux/`: Session orchestration and tmux command execution (`session.go`).
- `config/`: Parser and templates for `.agent-tmux.conf` (`parser.go`).
- `main.go`: Entrypoint wiring the CLI.
- `homebrew/`: Homebrew formula (`atmux.rb`) and alias (`agent-tmux.rb`).
- Top-level `Makefile`, `go.mod`, `go.sum` define build and dependencies.

## Languages
- Primary language is Go. Also uses TypeScript, JavaScript, Markdown, and YAML. Always run `go test ./...` after Go changes and ensure code compiles before committing.

## Build, Test, and Development Commands
- `make build`: Build the `atmux` binary with version metadata.
- `make install`: Install to `/usr/local/bin/atmux`.
- `make install-home`: Install to `~/bin/atmux` (or `~/bin/atmux-cli` symlink if a dir exists).
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

## Task Completion Checklist
After completing any code change, always run:
1. `make test` — ensure all tests pass.
2. `make install` — install the updated binary so the local CLI stays current.
3. Consider a version bump (`make tag-version VERSION=vX.Y.Z`) when the change is user-facing (new features, bug fixes, behavior changes). Skip for internal refactors, docs-only, or in-progress work.
4. `make version-status` — shows commits since the last tag; use this to judge whether a bump is warranted.

## Commit & Pull Request Guidelines
- Commit history uses short, descriptive sentences (e.g., "Add gitignore and fix Makefile install targets").
- Prefer small, focused commits that describe the change outcome.
- PRs should include: a brief summary, key commands run (e.g., `make test`), and notes on behavior changes. Add screenshots only if CLI output formatting changes.

## Configuration & Runtime Notes
- Project-specific behavior is configured in `.agent-tmux.conf` in the project root.
- The default config template is generated via `atmux init`.
- The diagnostics script is resolved from common install locations (e.g., `/usr/local/bin/atmux-diag.sh`, fallback `agent-tmux-diag.sh`).

## CLI Feature Notes
- `atmux sessions` is an interactive list with click-to-attach behavior.
- `atmux browse` opens a tree-based TUI with pane previews and command sending.
- `atmux browse --popup` uses a tmux popup overlay (tmux 3.2+).
- `atmux tiles` (alias `mosaic`) borrows the most recent local Claude panes into `_atmux_tiles` with `swap-pane`, leaving a stand-in in each pane's place. The borrowing is recorded in pane options (`@atmux_tile_home`, `@atmux_tile_slot`, `@atmux_tile_for`), and hooks at index 4747 on `client-session-changed`/`client-detached` put the panes back once no client views the tiles. `swap-pane` is its own inverse, so re-check a pane's session right before swapping it back. `_atmux_tiles` is hidden from session lists because killing it would kill the borrowed panes. Its right-click menu lives in the `atmux-tiles` key table, a full copy of the root table (a session's `key-table` never falls back to root, so a partial table loses the mouse). tmux 3.6b fires no mouse event for clicks on a pane heading (`pane-border-status`), ranges included.
- The sessions list (`atmux`, `atmux sessions`) has a tab strip of other views; ←/→ or Tab move along it, Enter opens. Add new views to `hubViews` in `cmd/hub.go`.
- `atmux agents` (aliases `overview`, `recaps`) is a clickable grid of Claude Code panes with state and recap; `--json` is the scripting interface (see `docs/agents-overview.md`). Remote hosts are queried with `atmux agents --json --local`, so keep that flag combination working.
- `claude/` reads Claude Code's undocumented on-disk state: `~/.claude/sessions/<pid>.json` (status busy/idle/waiting, `waitingFor`, `statusUpdatedAt`, tmux pane id) and transcript `away_summary` entries (recaps). Treat every field as optional; parse failures must degrade to "unknown", never break session listing.
- Test TUI mouse handling against a separate tmux server (`tmux -L <name>`) with a `script`-backed client, sending SGR mouse sequences via `send-keys -l`. Never exercise `switch-client` from a detached pane on the main server: it can move the user's own client. In that harness: delete a stale `/private/tmp/tmux-$UID/<name>` socket first (otherwise every command reports "server exited unexpectedly"), and never run `new-session` at all while `window-size` is `manual`: tmux 3.6b crashes the whole server, with or without `-x/-y`. Create every session first, then `set -g window-size manual` and `resize-window -x -y`. Send keys with `send-keys -l` (a bare `+` is not delivered as typed). `send-keys` only reaches the app in a pane; to test tmux's own mouse bindings, drive a real client in a pty (Python `pty.fork` running `tmux attach`, `TIOCSWINSZ` for its size) and write SGR sequences to it. Through `exec`, pass `\;` (backslash included) to chain commands inside `bind-key`; a bare `;` ends bind-key itself. In `display-menu` a separator is a single empty argument, not an empty name/key/command triple.

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
bd update <id> --notes="PLAN: ..."
bd label add <id> planned
bd label remove <id> needs-plan
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

<!-- end-bv-agent-instructions -->

### Planning Flow

1. **Capture**: Create issues with `needs-plan` label.
2. **Plan**: Add a structured plan in notes (PLAN / ACCEPTANCE / RISKS).
3. **Mark planned**: Swap labels (`planned` on, `needs-plan` off).
4. **Gate**: Only move to `in_progress` when `planned` is set.

```bash
bd create --title="..." --type=task --priority=2 --label needs-plan
bd update <id> --notes "PLAN:\n1) ...\n2) ...\n\nACCEPTANCE:\n- ...\n\nRISKS/QUESTIONS:\n- ..."
bd label add <id> planned
bd label remove <id> needs-plan
bd list --status=open --label planned
bd list --status=open --label needs-plan
```

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

