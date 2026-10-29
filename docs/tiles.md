# Tiles

`atmux tiles` (also `atmux mosaic`) shows your most recently active Claude
Code panes side by side in one window. The tiles are the real panes, not
copies: type into them, scroll them, and zoom one with `prefix z`.

It fits as many tiles of at least 80×20 as the screen allows, working and
waiting agents first, then the rest by how recently they finished.
`--count N` asks for a specific number.

## Your windows stay as they were

tmux can show a pane in only one place. To tile a pane, atmux swaps it with
a stand-in that holds its exact place in its own window: same position,
same size, same layout. The stand-in says where the pane went and offers
to bring it back.

Leave the tiles view however you like (`prefix s`, `prefix a`, switching
session, detaching) and every pane is swapped back. Run `atmux tiles` again
to pick the panes afresh, or `atmux tiles --restore` to put everything back
by hand.

While a pane is tiled it takes the tile's size, so Claude Code redraws for
it, and again when it goes home. Anyone looking at the pane's own window in
the meantime, say from another device, sees the stand-in.

Two alternatives fall short. Viewing the windows through a
second client resizes the whole original window, for everyone, to the tile's
size. Copying the screen (`capture-pane`) leaves everything alone but gives a
picture you cannot type into.

## How it is put back

The borrowing is written on the panes themselves as tmux pane options, so
any atmux process can undo it, even after a crash:

| Option             | On             | Value                                |
|--------------------|----------------|--------------------------------------|
| `@atmux_tile_home` | a tiled pane   | the session it belongs to            |
| `@atmux_tile_slot` | a tiled pane   | the stand-in holding its place       |
| `@atmux_tile_for`  | a stand-in     | the pane whose place it holds        |

Two global hooks, at index 4747 so your own hooks on the same events are
untouched, run `atmux tiles --restore-if-left` whenever a client changes
session or detaches. Once no client is looking at the tiles, the panes go
back, the `_atmux_tiles` session closes, and the hooks are removed.

If a tiled Claude Code exits, its stand-in is removed too, as if it had
exited in place. If a stand-in has been closed, its pane goes back as a new
window in its own session.

The `_atmux_tiles` session is left out of session lists: killing it would
kill the panes it holds.

## Limits

- Only this machine's panes. A pane cannot move between tmux servers, so
  remote hosts' agents stay out.
- Not with `window-size manual`: tmux 3.6b crashes creating any session in
  that mode, and tiles needs one.
- `atmux keybind --command tiles` binds it to `prefix T`.
