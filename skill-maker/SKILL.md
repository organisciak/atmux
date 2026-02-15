---
name: skill-maker
description: Create and push version tags that summarize repository changes since the last version bump. Use when the user asks to cut a release tag, summarize changes since the previous version, or update Homebrew formula version metadata.
---

# Release Tagging Workflow

1. Verify repository status.
- Run `git status --short`.
- Note any dirty files before tagging.

2. Inspect current version context.
- Run `make version-status`.
- Run `git describe --tags --abbrev=0 --match 'v[0-9]*'` to identify the latest bump tag.

3. Create the next version tag summary.
- Run `make tag-version VERSION=vMAJOR.MINOR.PATCH`.
- Add `PUSH=1` to push immediately: `make tag-version VERSION=vMAJOR.MINOR.PATCH PUSH=1`.
- Use the generated annotated message based on commit subjects since the previous `v*` tag.

4. Update Homebrew formula metadata for the same version.
- Run `make brew-bump VERSION=vMAJOR.MINOR.PATCH`.
- This updates `homebrew/atmux.rb` and `homebrew/agent-tmux.rb` URL + sha256 from the GitHub tag tarball.

5. Validate before finalizing.
- Run `go test ./...`.
- Confirm touched files with `git status --short`.

# Command Reference

```bash
make version-status
make tag-version VERSION=v0.2.0
make tag-version VERSION=v0.2.0 PUSH=1
make brew-bump VERSION=v0.2.0
make install-hooks
```

# Guardrails

- Keep release tags in `vMAJOR.MINOR.PATCH` format.
- Treat the latest `v*` tag as the last version bump source of truth.
- Refuse to overwrite existing tags; pick a new version instead.
