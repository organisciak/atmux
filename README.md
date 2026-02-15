# atmux (agent-tmux)

atmux (short for agent-tmux) gives you per-project tmux sessions tuned for AI coding. Run it in a repo, get a session with agent panes, and hop back in later.

## Quick try (you don't have to read the rest)

Just give it a spin:

```bash
brew tap organisciak/tap
brew install atmux
# or build from source:
# brew install --build-from-source ./homebrew/atmux.rb

# go to a project you're working on
cd ~/projects/my-app

# Start a new session
# optionally run onboarding to choose your default agents
atmux
# Detach from tmux: Ctrl-b d

# optional: run `atmux init` to create an editable ./agent-tmux.conf

# see all of your projects, whether active, attached, or inactive but previously run
atmux sessions
```

To shut things down:

```bash
atmux kill --all
```

If it's not for you:

```bash
brew uninstall atmux agent-tmux
# Optional: brew untap organisciak/tap
```

## What you can do

- Save and revive projects. atmux keeps a recent history of projects you ran, so you can jump back in with `atmux sessions` or `atmux open`.
- Move between sessions fast. The sessions list is a quick, clickable way to attach without hunting for names.
- Control everything from one screen. `atmux browse` shows a tree of sessions, windows, and panes, lets you preview output, and send commands/escape to any pane without switching away.
- Customize per project. Add a `.agent-tmux.conf` and define exactly which windows and panes you want for that repo.
- Enjoy quality-of-life extras like shell completions and popup-friendly UIs.

## Installation

### Homebrew (recommended)

```bash
brew tap organisciak/tap
brew install atmux
```

Homebrew installs the `atmux` command.
Alias: `brew install agent-tmux` (installs `atmux` plus an `agent-tmux` wrapper).

### Build from source

If you prefer to build locally:

```bash
brew install --build-from-source ./homebrew/atmux.rb
```

### Manual install

```bash
git clone https://github.com/organisciak/atmux.git
cd atmux
make install
```

This installs the `atmux` command.

## Usage

### Start a session

Run `atmux` in any project directory to create or attach to a session:

```bash
cd ~/projects/my-app
atmux
```

This creates a session named `agent-my-app` with:
- **agents** window: panes running your configured agents (defaults are provided, and you can customize them)
- Additional windows/panes from your `.agent-tmux.conf` (if present)

### Commands

```bash
atmux                 # Start or attach to session for current directory
atmux sessions [NAME] # Interactive sessions list or attach directly by name
atmux list            # Alias for sessions
atmux attach NAME     # Alias for sessions NAME
atmux list-sessions   # Alias for sessions
atmux browse          # Interactive session browser with pane previews
atmux open            # Quick TUI to jump into active or recent sessions
atmux send TARGET TXT # Send text to a target pane (local or remote)
atmux kill NAME       # Kill a specific session
atmux kill --all      # Kill all atmux sessions
atmux init            # Create a .agent-tmux.conf template
atmux history list    # Show recent sessions history
atmux history remove  # Remove a specific history entry
atmux history clear   # Clear history
atmux version         # Show version info
```

#### Browse mode

```bash
atmux browse
```

- Tree view of sessions, windows, and panes
- Live preview of selected pane output
- Send commands (and Escape) to any pane from the same screen
- Mouse and keyboard navigation
- Optional popup mode: `atmux browse --popup`

#### Sessions TUI

```bash
atmux sessions
```

- Click or select a session to attach
- Renders inline by default

#### Remote tmux sessions

`atmux` includes remote tmux execution via SSH-backed executors. Today, this is exposed through `atmux send --remote`.

```bash
# Send to one remote host
atmux send --remote=devbox agent-my-app:agents.0 "bd ready"

# Send the same command to multiple remote hosts
atmux send --remote=user@host1,user@host2 agent-my-app:agents.0 "/compact"
```

Requirements:
- `ssh` installed locally
- `tmux` installed on each remote host
- target pane exists on each remote host
- optional host aliases via `remote_*` directives in `.agent-tmux.conf` or global config

Behavior:
- One executor per remote host
- SSH ControlMaster connection reuse (`ControlPersist=300`)
- 10 second timeout per remote tmux command
- host keys accepted on first connect (`StrictHostKeyChecking=accept-new`)

For full details, see `docs/remote-sessions.md`.

## Configuration

Create a `.agent-tmux.conf` file in your project root to customize the session:

```bash
# Create a template
atmux init
```

### Config format

```conf
# Comments start with #

# Example: one window with multiple panes
window:dev
pane:npm run dev
pane:pnpm run emulators
pane:npm run build:watch

# Example: window with vertical panes
window:logs
vpane:tail -f logs/app.log
vpane:tail -f logs/error.log

# Add panes to the existing agents window
agents:htop
vagents:watch -n 1 'git status'

# Optional: remote host aliases for --remote flags
remote_host:user@devbox.example.com
remote_alias:devbox
remote_port:22
remote_attach:ssh
```

### Directives

| Directive | Description |
|-----------|-------------|
| `window:name` | Create a new window with the given name |
| `pane:cmd` | Add horizontal pane to current window |
| `vpane:cmd` | Add vertical pane to current window |
| `agents:cmd` | Add horizontal pane to the agents window |
| `vagents:cmd` | Add vertical pane to the agents window |
| `remote_host:host` | Define a remote host for alias resolution |
| `remote_alias:name` | Set alias for the most recent `remote_host` |
| `remote_port:port` | Set SSH port for the most recent `remote_host` |
| `remote_attach:ssh\|mosh` | Set attach method for the most recent `remote_host` |

## Shell Completions

```bash
# Bash
atmux completion bash > /etc/bash_completion.d/atmux

# Zsh
atmux completion zsh > "${fpath[1]}/_atmux"

# Fish
atmux completion fish > ~/.config/fish/completions/atmux.fish
```

## Development

```bash
# Build
make build

# Install to /usr/local/bin
make install

# Install to ~/bin
make install-home

# Build releases for all platforms
make release

# Run tests
make test

# Show commits since the latest version tag (v*)
make version-status

# Create annotated release tag summary (and optionally push)
make tag-version VERSION=v0.2.0
make tag-version VERSION=v0.2.0 PUSH=1

# Update Homebrew formula URL + SHA for a version tag
make brew-bump VERSION=v0.2.0

# Install local git hooks (includes post-commit version reminder)
make install-hooks
```

## License

MIT
