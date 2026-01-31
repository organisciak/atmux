# atmux (agent-tmux)

atmux (short for agent-tmux) is a CLI tool for managing tmux sessions optimized for AI coding workflows.

## Features

- Creates tmux sessions with dedicated panes for AI coding agents (codex, claude)
- Project-specific configuration via `.agent-tmux.conf`
- Interactive session browser with pane previews and command sending
- Interactive sessions list with click-to-attach
- Shell completions for bash, zsh, fish, and PowerShell

## Installation

### From source

```bash
git clone https://github.com/porganisciak/agent-tmux.git
cd agent-tmux
make install
```

This installs the `atmux` command.

### Homebrew (coming soon)

```bash
brew tap porganisciak/tap
brew install atmux
```

Homebrew installs the `atmux` command.
Alias: `brew install agent-tmux` (installs `atmux` plus an `agent-tmux` wrapper).

## Usage

### Start a session

Run `atmux` in any project directory to create or attach to a session:

```bash
cd ~/projects/my-app
atmux
```

This creates a session named `agent-my-app` with:
- **agents** window: side-by-side panes running `codex --yolo` and `claude code --yolo`
- **diag** window: diagnostics

### Commands

```bash
atmux                 # Start or attach to session for current directory
atmux list            # List all atmux sessions
atmux sessions        # Interactive sessions list (click or select to attach)
atmux list-sessions   # Alias for sessions
atmux browse          # Interactive session browser with pane previews
atmux attach NAME     # Attach to a specific session
atmux kill NAME       # Kill a specific session
atmux kill --all      # Kill all atmux sessions
atmux init            # Create a .agent-tmux.conf template
atmux version         # Show version info
```

#### Browse mode

```bash
atmux browse
```

- Tree view of sessions, windows, and panes
- Live preview of selected pane output
- Send commands (and Escape) to a pane
- Mouse and keyboard navigation
- Optional popup mode: `atmux browse --popup`

#### Sessions TUI

```bash
atmux sessions
```

- Click or select a session to attach
- Renders inline by default

## Configuration

Create a `.agent-tmux.conf` file in your project root to customize the session:

```bash
# Create a template
atmux init
```

### Config format

```conf
# Comments start with #

# Create a new window with horizontal panes
window:dev
pane:pnpm dev
pane:pnpm run emulators

# Create a window with vertical panes
window:logs
vpane:tail -f logs/app.log
vpane:tail -f logs/error.log

# Add panes to the existing agents window
agents:htop
vagents:watch -n 1 'git status'
```

### Directives

| Directive | Description |
|-----------|-------------|
| `window:name` | Create a new window with the given name |
| `pane:cmd` | Add horizontal pane to current window |
| `vpane:cmd` | Add vertical pane to current window |
| `agents:cmd` | Add horizontal pane to the agents window |
| `vagents:cmd` | Add vertical pane to the agents window |

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
```

## License

MIT
