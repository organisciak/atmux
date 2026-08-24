# Agent session titles

Claude Code names each conversation — automatically at first, or whatever you
rename it to — and keeps the terminal title set to that name. tmux exposes it as
`pane_title`, so atmux can show which conversation a session is holding without
any cooperation from the agent.

atmux surfaces it in two places.

## In the tmux status bar

```console
$ atmux title on          # this project
$ atmux title off
$ atmux title             # show current setting
$ atmux title --all       # apply to every running atmux session
```

Or in `.agent-tmux.conf`:

```
session_title:on
```

The status bar then reads something like:

```
 ◐ Design persistent remote atmux session management
```

This sets `status-right` on the **session only**, so your global tmux status bar
is untouched everywhere else. `atmux title off` unsets it and the session falls
back to whatever it would otherwise inherit.

It is opt-in: silently overwriting someone's `status-right` would be invasive,
so a session created without the directive is left alone.

Sessions pick the setting up when they are created. `--all` exists for sessions
that were already running.

## In the session list

`atmux sessions` appends each session's title to its row:

```
 2. agent-workspace: 1 windows (created …)  agents[0:496K]  ✳ Obsidian Dataview example
 4. agent-ocsai-py: 1 windows (created …)   agents[0:688K]  ◑ MOTES confidence rescoring files
```

The title is the least critical column on the row, so it is dropped rather than
allowed to wrap when the terminal is narrow.

Titles are read with a single `list-panes -a` per host, so the list costs one
extra round trip per host rather than one per session. Remote hosts running
atmux send their titles in the [JSON payload](./sessions-json-schema.md)
instead; hosts without atmux still get titles, since the pane-title read works
over plain tmux too.

## The leading glyph

The glyph Claude Code prefixes is a live activity indicator:

| Glyph | Meaning |
| ----- | ------- |
| `✳` | idle, waiting for you |
| `◐ ◑ ◒ ◓`, `⠂ ⠄ ⠇ …` | spinner frames — the agent is working |

It is kept undimmed in the session list while the name itself recedes, so a
glance across a dozen sessions shows which ones are actually busy.

`tmux.SplitAgentTitle` separates the glyph from the text for callers that want a
stable string — searching, sorting, or storage — since the glyph changes from
moment to moment.

## Related

[Remote Control at launch](./remote-control.md) enables Claude Code's remote
access when a session starts, naming the remote session the same way.

## Renaming a conversation

There is no `/rename` slash command. Inside Claude Code:

1. Run `/resume`
2. Highlight the session
3. Press **Ctrl+R**, type the new name, press Enter

Renamed conversations show up here the same way auto-named ones do — Claude Code
resolves a custom title ahead of the generated one when setting the terminal
title.

## What counts as an agent pane

A pane is treated as an agent when its `pane_current_command` is Claude Code's
semver (its signature behaviour — e.g. `2.1.234`), or when it is literally
`claude`. A shell whose title happens to look like a session name is ignored,
which is what keeps a stale title from an exited agent out of the list.

Sessions with no agent pane simply carry no title, which stays distinguishable
from an agent that has not named itself yet.
