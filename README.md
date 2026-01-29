# agent-tmux

A CLI tool for managing tmux sessions optimized for AI coding workflows.

## Features

- Creates tmux sessions with dedicated panes for AI coding agents (codex, claude)
- Project-specific configuration via `.agent-tmux.conf`
- Easy session management (list, attach, kill)
- Shell completions for bash, zsh, fish, and PowerShell

## Installation

### From source

```bash
git clone https://github.com/porganisciak/agent-tmux.git
cd agent-tmux
make install
```

### Homebrew (coming soon)

```bash
brew tap porganisciak/tap
brew install agent-tmux
```

## Usage

### Start a session

Run `agent-tmux` in any project directory to create or attach to a session:

```bash
cd ~/projects/my-app
agent-tmux
```

This creates a session named `agent-my-app` with:
- **agents** window: side-by-side panes running `codex --yolo` and `claude code --yolo`
- **diag** window: diagnostics

### Commands

```bash
agent-tmux              # Start or attach to session for current directory
agent-tmux list         # List all agent-tmux sessions
agent-tmux attach NAME  # Attach to a specific session
agent-tmux kill NAME    # Kill a specific session
agent-tmux kill --all   # Kill all agent-tmux sessions
agent-tmux init         # Create a .agent-tmux.conf template
agent-tmux version      # Show version info
```

## Configuration

Create a `.agent-tmux.conf` file in your project root to customize the session:

```bash
# Create a template
agent-tmux init
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
agent-tmux completion bash > /etc/bash_completion.d/agent-tmux

# Zsh
agent-tmux completion zsh > "${fpath[1]}/_agent-tmux"

# Fish
agent-tmux completion fish > ~/.config/fish/completions/agent-tmux.fish
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
