# Repository Guidelines

## Improvements to AGENTS.md

Regularly check if AGENTS.md is up to date, and update if it's not. If you learn new useful things that help in understanding the development, add it here. never check in any updates to agents that you make to prevent accidental exfiltration. Always leave it for a human to review unless they explicitly tell you to check the specific file in. 

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

