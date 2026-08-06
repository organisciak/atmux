# Remote atmux sessions

This document explains how remote tmux execution works in `atmux`.

## What "remote sessions" means in atmux

`atmux` uses a `TmuxExecutor` abstraction:

- `LocalExecutor`: runs tmux commands on the local machine.
- `RemoteExecutor`: runs tmux commands on a remote machine over SSH.

The remote executor enables `atmux` commands to target tmux sessions that exist on another host, without requiring a separate local tmux server.

## Current user-facing support

Remote execution is currently exposed in multiple commands:

- `atmux sessions [--remote=...] [--strategy=auto|replace|new-window]`
- `atmux browse --remote=...`
- `atmux send --remote=... <target> <text>`
- `atmux recents` (remote history entries reconnect to their saved host)
- `atmux remote-project [name] --host <host-or-alias> --dir <remote-dir> [--session <name>]`

Examples:

```bash
# List/attach across local + remote hosts
atmux sessions --remote=devbox

# Browse remote panes (preview/send/kill)
atmux browse --remote=devbox

# Single host
atmux send --remote=devbox agent-my-app:agents.0 "bd ready"

# Multiple hosts (broadcast)
atmux send --remote=user@host1,user@host2 agent-my-app:agents.0 "/compact"
```

Important:

- `sessions` always includes local plus configured remote hosts; `--remote` adds explicit host(s)/alias(es).
- `browse` includes remote hosts when `--remote` is provided.
- In `send`, the `<target>` pane name is resolved on each remote host.
- If one host fails during `send`, the command exits with an error for that host.

## Connection lifecycle

`RemoteExecutor` uses SSH ControlMaster so that connections are shared across
atmux invocations, not just within one.

The control socket lives at a stable path — `$XDG_CACHE_HOME/atmux/ssh/<hash>.sock`,
falling back to `/tmp/atmux-<uid>/ssh/` when that would overflow the 104-byte
limit macOS places on Unix socket paths. The directory is kept at mode `0700`:
anyone who can write there can hijack an authenticated connection.

On a command to a host:

1. If a master is already listening (`ssh -O check`), it is reused — no handshake.
2. A socket file that fails the check is stale (the remote rebooted, or the
   master was killed). SSH does not clear it on its own, so atmux removes it.
3. Otherwise `ssh -f -N` opens a master, returning once authenticated.

Options applied to the master and to every command riding it:

- `ControlMaster=auto`
- `ControlPersist=4h` (override with `ATMUX_SSH_CONTROL_PERSIST`)
- `ConnectTimeout=5`
- `ServerAliveInterval=15`, `ServerAliveCountMax=3`
- `StrictHostKeyChecking=accept-new`

The keepalives matter: `ssh -O check` only asks the *local* master process
whether it is running, so a master whose connection has silently died would
otherwise report healthy while every command through it hangs.

Cleanup:

- `Close()` is a no-op. Ending the process deliberately leaves the master
  running so the next invocation is cheap.
- `atmux remote disconnect [host...]` tears masters down explicitly.
- `atmux remote list` shows which hosts currently have a live connection.

## Reachability and retries

Failures are classified rather than collapsed into "unreachable":

| State | Meaning |
| ----- | ------- |
| `unreachable` | SSH itself failed — network down, VPN off, auth refused. |
| `tmux not installed` | SSH worked, but the host has no tmux. |
| *(healthy, no sessions)* | tmux answered and reported no server running. |

A failed host backs off exponentially from 5s to 2 minutes, and calls made
during backoff fail immediately instead of dialing again — one host behind a
downed VPN must not cost every refresh a connect timeout. Pressing `R` (or
`ctrl+r`) in the session list clears the backoff and re-polls every host, so a
host that has come back up is picked up without restarting atmux.

An unreachable host never removes or delays local sessions; hosts are fetched
concurrently and local results render immediately.

## Rich remote metadata

Plain `tmux list-sessions` cannot report working directories that mean anything
locally, project colors, or beads counts. When a remote host has atmux
installed, atmux asks it to describe itself instead:

1. Run `atmux sessions --json` on the host (see
   [the schema](./sessions-json-schema.md)).
2. If `atmux` is not on the non-interactive PATH — SSH runs a non-login shell,
   which on many setups skips the profile that adds `~/bin` or Homebrew — retry
   through `bash -lc`.
3. If neither works, or the host speaks a schema this build does not
   understand, fall back to plain `tmux list-sessions` unchanged.

The fetch doubles as the capability probe, so a host with atmux costs one round
trip rather than a probe plus a fetch. The result is cached per host for the
life of the process.

Beads counts cost a shell-out per session on the remote, so `--no-beads` is
passed through to the remote host rather than being filtered locally.

## Interactive attach mode

`RemoteExecutor.Interactive(...)` supports:

- `ssh` mode (default): `ssh -t <host> tmux <args...>`
- `mosh` mode: `mosh <host> -- tmux <args...>`

If a non-default SSH port is used with `mosh`, `--ssh=ssh -p <port>` is added.

This interactive path is used for remote attach flows in `sessions` and remote history revival in `recents`.

## Configuration status

There is shared command-side plumbing for building executors from config (`cmd/remote.go`) and support for remote host metadata in session models.

Current config/state support:

- remote host aliases are configurable via `.agent-tmux.conf` and global config directives:
  - `remote_host:...`
  - `remote_alias:...`
  - `remote_port:...`
  - `remote_attach:ssh|mosh`
- remote projects are configurable via global config directives:
  - `remote_project:...`
  - `remote_project_host:...`
  - `remote_project_dir:...`
  - `remote_project_session:...`
- `atmux remote-project` writes reusable remote project entries to global config.
- Remote project entries are currently configuration metadata; there is not yet a dedicated "launch remote project by name" command.

## Prerequisites

- local machine: `ssh` (and optionally `mosh`)
- remote machine(s): `tmux` available in shell PATH
- remote machine(s), optional: `atmux` on PATH, for working directories,
  project colors, and beads counts on remote rows
- network access and SSH auth configured

## Troubleshooting

If a remote command fails:

1. Verify SSH connectivity:
   ```bash
   ssh <host> "tmux -V"
   ```
2. Verify target session/pane exists remotely:
   ```bash
   ssh <host> "tmux list-sessions"
   ssh <host> "tmux list-panes -t <session>:<window>"
   ```
3. Confirm target format is valid:
   - `session:window.pane`
   - example: `agent-my-app:agents.0`
