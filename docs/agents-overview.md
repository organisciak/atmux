# Agents overview

`atmux agents` (also `atmux overview`, `atmux recaps`) shows every Claude Code
session as a card: what it is doing, how long it has been doing it, and
Claude's latest recap of the session. Click a card, or select it and press
Enter, to jump to that pane.

```
Claude Code sessions  1 needs input  2 ready  1 working  7 dormant

╭──────────────────────────────────────╮╭──────────────────────────────────────╮
│▲ needs input: permission prompt 2m   ││✓ ready 4m  agent-hex-wall            │
│Cube light access point connection    ││Hex wall lighting installation        │
│Pushed and live on the next deploy.   ││Building hex-wall firmware for your   │
│                                      ││growing hexagon light wall; latest…   │
╰──────────────────────────────────────╯╰──────────────────────────────────────╯
```

Inside tmux it opens as a popup (`--no-popup` to run it in place). Cards keep
their positions as states change, so what you are about to click does not
move under the pointer. It refreshes every two seconds.

## States

| Activity      | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `working`     | A turn is running: thinking, streaming, tools, or subagents.   |
| `needs-input` | Blocked on you: a permission prompt, question, or dialog.      |
| `ready`       | The turn finished less than 30 minutes ago.                    |
| `dormant`     | Finished more than 30 minutes ago.                             |
| `fresh`       | Started but never prompted.                                    |
| `unknown`     | A Claude pane whose state could not be read.                   |

The duration on each card is how long the session has been in that state, so
`ready 4m` finished four minutes ago and `working 12m` has been on the same
turn for twelve.

## Where the state comes from

**Claude Code's session registry.** Each running Claude Code process writes
`~/.claude/sessions/<pid>.json` with its `status` (`busy`, `idle`, or
`waiting`), what it is waiting for, when that status last changed, and the
tmux pane it runs in. This is exact and costs a few small file reads. It is
not a documented interface, so atmux treats every field as optional.

Panes are matched to registry entries by pane id. A Claude started where it
could not see `$TMUX_PANE` is matched through its parent process, the pane's
shell.

**The screen, as a fallback.** Panes the registry does not describe — a
Claude Code too old to write it, or a remote host without a current atmux —
are read off the screen: `esc to interrupt` in the footer means working, a
numbered `Do you want to…` prompt means needs input, anything else means
idle. Screen-read idle times come from the window's last output.

## Where the recap comes from

When you come back to a session after a while, Claude writes a short recap
("※ recap: …") into the session's transcript as an `away_summary` entry.
atmux reads the most recent one from the transcript's tail.

A recap describes the session as of when it was written. If you have sent a
prompt since, the card shows the opening paragraph of Claude's latest reply
instead, which is newer. Working sessions show the prompt they are working
on. Transcripts are cached until they change, so idle sessions cost nothing
to refresh.

## Remote hosts

Saved remote hosts are included, as in `atmux sessions`. A host's registry and
transcripts only exist on that host, so atmux asks the host's own atmux for
them (`atmux agents --json --local` over the shared SSH connection). A host
with an older atmux, or none, falls back to reading screens, which gives
states but no recaps. Each host loads on its own, so an unreachable one never
holds up the rest.

## JSON for scripts

`atmux agents --json` prints the same data for anything that wants to poll
it: a status light, a menu bar item, a MIDI pad.

```json
{
  "schema": 1,
  "agents": [
    {
      "host": "vps",
      "session": "agent-cube-light",
      "window": "agents",
      "pane": "%3",
      "target": "agent-cube-light:0.0",
      "title": "Cube light access point connection issue",
      "activity": "needs-input",
      "status": "waiting",
      "waiting_for": "permission prompt",
      "since": "2026-09-30T17:02:11Z",
      "since_seconds": 131,
      "session_id": "914d3b89-…",
      "recap": "Pushed and live on the next deploy.",
      "recap_at": "2026-09-30T16:40:00Z",
      "recap_current": true,
      "last_prompt": "…",
      "last_prompt_at": "…",
      "last_reply": "…"
    }
  ]
}
```

`activity` is the field to map to colours. Fields other than `session`,
`window`, `pane`, `target`, and `activity` are omitted when unknown. `schema`
is bumped only for incompatible changes; readers should reject a schema newer
than they know. `--local` leaves out saved remote hosts.

To jump to an agent from a script, switch to its pane:
`tmux switch-client -t <pane>`.
