# atmux (agent-tmux)

atmux (short for agent-tmux) gives you per-project tmux sessions tuned for AI coding. Run it in a repo, get a session with agent panes, and hop back in later.

## Quick try (you don't have to read the rest)

Just give it a spin:

```bash
brew install --build-from-source ./homebrew/atmux.rb
# or, if you use the tap:
# brew tap organisciak/tap
# brew install atmux

cd ~/projects/my-app
atmux
# Detach from tmux: Ctrl-b d

cd ~/projects/another-app
atmux
# Detach again: Ctrl-b d

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

### Homebrew

```bash
brew install --build-from-source ./homebrew/atmux.rb
```

Or, if you use the tap:

```bash
brew tap organisciak/tap
brew install atmux
```

Homebrew installs the `atmux` command.
Alias: `brew install agent-tmux` (installs `atmux` plus an `agent-tmux` wrapper).

### From source

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
atmux list            # List all atmux sessions
atmux sessions        # Interactive sessions list (click or select to attach)
atmux list-sessions   # Alias for sessions
atmux browse          # Interactive session browser with pane previews
atmux open            # Quick TUI to jump into active or recent sessions
atmux attach NAME     # Attach to a specific session
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
