# Remote atmux sessions

This document explains how remote tmux execution works in `atmux`.

## What "remote sessions" means in atmux

`atmux` uses a `TmuxExecutor` abstraction:

- `LocalExecutor`: runs tmux commands on the local machine.
- `RemoteExecutor`: runs tmux commands on a remote machine over SSH.

The remote executor enables `atmux` commands to target tmux sessions that exist on another host, without requiring a separate local tmux server.

## Current user-facing support

The currently exposed CLI feature is:

- `atmux send --remote=... <target> <text>`

Examples:

```bash
# Single host
atmux send --remote=devbox agent-my-app:agents.0 "bd ready"

# Multiple hosts (broadcast)
atmux send --remote=user@host1,user@host2 agent-my-app:agents.0 "/compact"
```

Important:

- The `<target>` pane name is resolved on each remote host.
- If one host fails, the command exits with an error for that host.
- `--remote` in `send` currently creates direct SSH executors from the flag values.

## Connection lifecycle

`RemoteExecutor` uses SSH ControlMaster to reduce connection overhead.

On first command to a host:

1. `atmux` creates a temp socket directory (`atmux-ssh-*`).
2. Starts an SSH ControlMaster connection with:
   - `ControlMaster=auto`
   - `ControlPath=<temp>/ctrl-%C`
   - `ControlPersist=300`
   - `StrictHostKeyChecking=accept-new`
3. Reuses that connection for subsequent commands to the same host.

Per-command execution:

- `Run` / `Output` execute `ssh <opts> <host> tmux <args...>`
- command timeout is 10 seconds

Cleanup:

- `Close()` sends `ssh -O exit` to stop the master connection
- removes the temp control socket directory

## Interactive attach mode

`RemoteExecutor.Interactive(...)` supports:

- `ssh` mode (default): `ssh -t <host> tmux <args...>`
- `mosh` mode: `mosh <host> -- tmux <args...>`

If a non-default SSH port is used with `mosh`, `--ssh=ssh -p <port>` is added.

Note: the current CLI primarily uses remote execution for command sending. Interactive remote attach plumbing exists in the executor layer.

## Configuration status

There is shared command-side plumbing for building executors from config (`cmd/remote.go`) and support for remote host metadata in session models.

At the moment:

- `atmux send --remote` is the active remote workflow
- full remote session listing/attach via standard commands is not yet fully wired end-to-end
- remote host aliases are configurable via `.agent-tmux.conf` and global config directives:
  - `remote_host:...`
  - `remote_alias:...`
  - `remote_port:...`
  - `remote_attach:ssh|mosh`

## Prerequisites

- local machine: `ssh` (and optionally `mosh`)
- remote machine(s): `tmux` available in shell PATH
- network access and SSH auth configured

## Troubleshooting

If a remote send fails:

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
