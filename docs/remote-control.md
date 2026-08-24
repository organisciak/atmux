# Remote Control at launch

Claude Code's Remote Control makes a session reachable from claude.ai/code and
the Claude mobile app. atmux can turn it on as soon as a session starts, so a
whole "connect, launch, switch to the phone" round trip is one command.

That is the point of the feature: over a thin or intermittent link — tethering,
Starlink, mosh from a phone — you want to hop on, start the session, and do the
actual work from the app rather than through the terminal.

## Enabling it

In `.agent-tmux.conf` (or the global config):

```
remote_control:on
```

Then launching the project is the whole workflow:

```console
$ mosh myserver
$ cd ~/projects/my-app && atmux
Creating new session: agent-my-app
Enabling Remote Control as "my app" (waiting for the agent to start)...
Remote Control enabled. Open the Claude app to pick it up.
```

It is **off by default**. Enabling remote access is not something that should
happen because a directive was omitted.

## Naming

The remote session is named after the tmux session, with the atmux prefix
stripped and separators turned into spaces:

| tmux session | Shown in the app |
| ------------ | ---------------- |
| `agent-my-app` | `my app` |
| `agent-3d-print` | `3d print` |
| `agent-creativity-task-archive` | `creativity task archive` |

Window names are deliberately *not* used: atmux windows are called `agents`,
`dev`, `zsh`, which would make every session look the same in the app.

`atmux rc` uses the same naming, so a session enabled at launch and one enabled
later are labelled identically. Override with a positional argument:

```console
$ atmux rc "Something Else"
```

## Timing

`/remote-control` has to be typed into a running agent, which does not exist the
instant the session is created. atmux waits for an agent pane to appear — the
same check used everywhere else, keyed on Claude Code reporting its own semver
as `pane_current_command` — then gives it a moment to finish drawing its input
box before sending.

The wait is bounded at 30 seconds. If the agent never appears, atmux warns and
carries on rather than failing the launch:

```
Warning: could not enable Remote Control: no agent pane appeared in agent-my-app within 30s
Run `atmux rc` once the agent is up.
```

The session itself is unaffected — only the remote-control step is skipped.

## Enabling it later

For sessions already running, or ones launched without the directive:

```console
$ atmux rc              # interactive selector
$ atmux rc --all        # every detected agent pane
$ atmux rc --dry-run    # show what would be enabled
```
