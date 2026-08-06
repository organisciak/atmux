# `atmux sessions --json` schema

`atmux sessions --json` prints the **local** session list as JSON and exits. It
never contacts remote hosts, so it is safe to invoke over SSH without recursing.

This output is a compatibility surface: one atmux instance runs it on a remote
host and parses the result locally, and the two ends may be different versions.
Treat it accordingly.

```console
$ atmux sessions --json
```

## Envelope

| Field            | Type      | Notes |
| ---------------- | --------- | ----- |
| `schema_version` | integer   | Currently `1`. See [Versioning](#versioning). |
| `atmux_version`  | string    | Build version of the atmux that produced the payload. |
| `generated_at`   | RFC 3339  | Always UTC. |
| `sessions`       | array     | Empty array when no tmux server is running — never `null`. |

## Session object

| Field         | Type    | Notes |
| ------------- | ------- | ----- |
| `name`        | string  | tmux session name, e.g. `agent-myproject`. |
| `display`     | string  | Preformatted list line, close to default `tmux ls` output. |
| `attached`    | bool    | Whether a client is currently attached. |
| `working_dir` | string  | Session path. Omitted when tmux reports none. |
| `activity`    | integer | Unix timestamp of last activity; used for sorting. |
| `color`       | string  | Project accent colour from the project's `.agent-tmux.conf`. Omitted when unset. |
| `beads_open`  | integer | Open beads issues. **Omitted** when the directory is not a beads project. |

### `beads_open` is deliberately nullable

An omitted `beads_open` means "not a beads project"; a present `0` means "a
beads project with nothing open". Collapsing these would make every directory
look like an empty backlog. Readers must distinguish them.

Counting beads issues shells out once per session, which is slow enough to
matter over SSH. Pass `--no-beads` to skip it:

```console
$ atmux sessions --json --no-beads
```

## No tmux server

A host with no running tmux server is not an error. The command exits `0` and
emits an empty `sessions` array, so a caller can tell "nothing running" apart
from "could not ask".

## Versioning

`schema_version` is bumped only for changes that break existing readers —
removing a field, renaming one, or changing its type. Adding a new optional
field does not qualify, so readers must ignore unknown fields.

A reader that sees a `schema_version` higher than it understands should fall
back to plain `tmux list-sessions` over SSH rather than guessing.

## Producing the payload

The payload is built from the same collection path the interactive session list
uses (`tmux.ListSessionsRawWithExecutor` and `tmux.BeadsOpenCount`), so JSON
output and the TUI cannot report different data for the same host.
