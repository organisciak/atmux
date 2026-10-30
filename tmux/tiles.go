package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// TilesSessionName holds the tiles view: one window of Claude Code panes
// borrowed from their own windows.
//
// tmux can show a pane in only one place, so each pane is swapped with a
// stand-in that keeps its place, and swapped back when nobody is looking at
// the tiles any more. Viewing a window from a second client instead would
// resize the whole window for everyone, and so would grouped sessions.
const TilesSessionName = "_atmux_tiles"

// Pane options that record the borrowing on the panes themselves, where any
// atmux process can find and undo it, even after a crash. Pane options travel
// with the pane when it is swapped.
const (
	optTileHome = "@atmux_tile_home" // on a borrowed pane: the session it belongs to
	optTileSlot = "@atmux_tile_slot" // on a borrowed pane: its stand-in
	optTileFor  = "@atmux_tile_for"  // on a stand-in: the pane whose place it holds
)

// tilesHooks put the panes back when the last client leaves the tiles view.
// They take one fixed index in tmux's hook arrays so the user's own hooks on
// the same events are left alone.
var tilesHooks = []string{"client-session-changed[4747]", "client-detached[4747]"}

// The smallest tile worth giving a Claude Code pane, border included.
const (
	TileMinWidth  = 80
	TileMinHeight = 20
)

// TileCount is how many tiles of at least TileMinWidth x TileMinHeight fit.
func TileCount(width, height int) int {
	return max(1, width/TileMinWidth) * max(1, height/TileMinHeight)
}

// RecentAgents returns the panes the tiles view can borrow, most recently
// active first. Working and waiting agents count as active now; the rest
// rank by when they last changed state. Remote panes are left out, because
// a pane cannot be swapped between tmux servers, as are panes already lent
// to an atmux view.
func RecentAgents(agents []AgentPane, now time.Time) []AgentPane {
	var out []AgentPane
	for _, a := range agents {
		if a.Executor != nil && a.Executor.IsRemote() {
			continue
		}
		if strings.HasPrefix(a.SessionName, "_atmux_") || a.Pane.ID == "" {
			continue
		}
		out = append(out, a)
	}
	lastActive := func(a AgentPane) time.Time {
		switch a.Activity(now) {
		case ActivityWorking, ActivityNeedsInput:
			return now
		}
		return a.Since
	}
	sort.SliceStable(out, func(i, j int) bool {
		return lastActive(out[i]).After(lastActive(out[j]))
	})
	return out
}

// TilesOptions describes a tiles view to build.
type TilesOptions struct {
	Agents        []AgentPane // Panes to borrow, in tile order
	Width, Height int         // Size of the window to build
	// Command is the shell command for "atmux tiles". Stand-ins, the hooks
	// and the tile menu run it with their own flags.
	Command string
}

// OpenTiles builds the tiles view and borrows the panes into it, putting back
// anything a previous view still holds first. It returns how many panes were
// borrowed. The caller switches the client to TilesSessionName.
func OpenTiles(opts TilesOptions) (int, error) {
	if err := RestoreTiles(); err != nil {
		return 0, err
	}
	if len(opts.Agents) == 0 {
		return 0, fmt.Errorf("no Claude Code panes to show")
	}

	// tmux 3.6b crashes on new-session while window-size is manual, which
	// would take every session down with it.
	if globalOption("window-size") == "manual" {
		return 0, fmt.Errorf("tiles needs a new tmux session, and tmux can crash creating one while window-size is manual")
	}
	out, err := exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{window_id}\t#{pane_id}",
		"-s", TilesSessionName, "-n", "tiles",
		"-x", fmt.Sprint(max(opts.Width, 1)), "-y", fmt.Sprint(max(opts.Height, 1)),
		opts.Command+" --stand-in").Output()
	if err != nil {
		return 0, fmt.Errorf("could not create the tiles session: %w", err)
	}
	ids := strings.Split(strings.TrimSpace(string(out)), "\t")
	if len(ids) != 2 {
		return 0, fmt.Errorf("unexpected new-session output %q", out)
	}
	window, standIns := ids[0], []string{ids[1]}

	for len(standIns) < len(opts.Agents) {
		out, err := exec.Command("tmux", "split-window", "-d", "-P", "-F", "#{pane_id}", "-t", window, opts.Command+" --stand-in").Output()
		if err != nil {
			// Out of room: show the ones that fit.
			break
		}
		standIns = append(standIns, strings.TrimSpace(string(out)))
		// Re-tile as we go, or the next split may find no room.
		exec.Command("tmux", "select-layout", "-t", window, "tiled").Run()
	}

	borrowed, first := 0, ""
	for i, standIn := range standIns {
		if borrowPane(opts.Agents[i], standIn) {
			if borrowed == 0 {
				first = opts.Agents[i].Pane.ID
			}
			borrowed++
		} else {
			exec.Command("tmux", "kill-pane", "-t", standIn).Run()
		}
	}
	if borrowed == 0 {
		exec.Command("tmux", "kill-session", "-t", TilesSessionName).Run()
		return 0, fmt.Errorf("could not move any Claude Code pane into the tiles view")
	}
	exec.Command("tmux", "select-layout", "-t", window, "tiled").Run()

	// Label each tile with the session it came from. These are options of
	// this window alone.
	exec.Command("tmux", "set-option", "-w", "-t", window, "pane-border-status", "top").Run()
	exec.Command("tmux", "set-option", "-w", "-t", window, "pane-border-format",
		" #{"+optTileHome+"}  #{pane_title} ").Run()
	exec.Command("tmux", "select-pane", "-t", first).Run()

	installTilesMenu(opts.Command)

	hook := "run-shell -b " + tmuxDoubleQuote(opts.Command+" --restore-if-left")
	for _, name := range tilesHooks {
		exec.Command("tmux", "set-hook", "-g", name, hook).Run()
	}
	return borrowed, nil
}

// tilesKeyTable holds the tiles view's key bindings: a copy of the root table,
// so the mouse and any bind -n keys work as everywhere else, with a right-click
// opening the tile menu. A session's own key table does not fall back to root,
// which is why the copy has to be whole.
const tilesKeyTable = "atmux-tiles"

// installTilesMenu gives the tiles view its key table. A right-click selects
// the tile under the pointer and offers what to do with it.
func installTilesMenu(command string) {
	out, err := exec.Command("tmux", "list-keys", "-T", "root").Output()
	if err != nil {
		return
	}
	f, err := os.CreateTemp("", "atmux-tiles-keys-*.conf")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(copyKeyTable(string(out), "root", tilesKeyTable))
	f.Close()
	if err != nil || exec.Command("tmux", "source-file", f.Name()).Run() != nil {
		// Without a full copy the view would lose the mouse; keep the root
		// table and go without the menu.
		exec.Command("tmux", "unbind-key", "-a", "-T", tilesKeyTable).Run()
		return
	}

	run := func(flags string) string {
		return "run-shell " + tmuxDoubleQuote(command+" "+flags)
	}
	args := append([]string{"bind-key", "-T", tilesKeyTable, "MouseDown3Pane",
		// On a command line a bare ";" would end bind-key itself; the
		// escaped one becomes part of the binding.
		"select-pane", "-t", "=", `\;`,
		"display-menu", "-T", "#[align=centre]#{" + optTileHome + "}", "-t", "=", "-x", "M", "-y", "M"},
		tileMenuItems(run)...)
	if exec.Command("tmux", args...).Run() != nil {
		exec.Command("tmux", "unbind-key", "-a", "-T", tilesKeyTable).Run()
		return
	}
	exec.Command("tmux", "set-option", "-t", TilesSessionName, "key-table", tilesKeyTable).Run()
}

// tileMenuItems are the tile menu's entries as display-menu arguments: name,
// key, command. run builds a command that runs atmux tiles with flags. The
// tile under the pointer is the active pane by the time an entry runs.
func tileMenuItems(run func(flags string) string) []string {
	closePrompt := "Close session #{" + optTileHome + "}, ending its Claude Code? (y/n)"
	return []string{
		"Zoom", "z", "resize-pane -Z",
		"Put back in its window", "p", run("--put-back #{pane_id}"),
		"Go to its window", "g", run("--go #{pane_id} --client #{client_name}"),
		"", // a separator takes just the empty name
		"Close its session", "x", "confirm-before -p " + tmuxDoubleQuote(closePrompt) + " " +
			tmuxDoubleQuote(run("--close #{pane_id}")),
	}
}

// copyKeyTable rewrites `tmux list-keys -T from` output to bind the same keys
// in table to.
func copyKeyTable(listKeys, from, to string) string {
	var b strings.Builder
	for _, line := range strings.Split(listKeys, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString(strings.Replace(line, " -T "+from+" ", " -T "+to+" ", 1))
		b.WriteString("\n")
	}
	return b.String()
}

// PutBackTile returns one tiled pane to its own window and closes its tile.
func PutBackTile(paneID string) error {
	panes, err := listTilePanes()
	if err != nil {
		return err
	}
	p, slot, ok := findTile(panes, paneID)
	if !ok {
		return fmt.Errorf("%s is not in the tiles view", paneID)
	}
	if slot != nil && !slot.inAtmuxView() {
		if err := exec.Command("tmux", "swap-pane", "-d", "-s", p.ID, "-t", p.Slot).Run(); err != nil {
			return err
		}
		// The stand-in now has the tile; the tile goes with it.
		exec.Command("tmux", "kill-pane", "-t", p.Slot).Run()
	} else if !sendPaneHome(p) {
		return fmt.Errorf("could not put %s back", paneID)
	}
	unsetPaneOption(p.ID, optTileHome)
	unsetPaneOption(p.ID, optTileSlot)
	return retileOrClose()
}

// CloseTiledSession kills the session a tiled pane belongs to, together with
// every pane of it in the tiles view, which a kill-session would miss.
func CloseTiledSession(paneID string) error {
	panes, err := listTilePanes()
	if err != nil {
		return err
	}
	p, _, ok := findTile(panes, paneID)
	if !ok || p.Home == "" {
		return fmt.Errorf("%s is not in the tiles view", paneID)
	}
	for _, q := range panes {
		if q.inTiles() && q.Slot != "" && q.Home == p.Home {
			exec.Command("tmux", "kill-pane", "-t", q.ID).Run()
		}
	}
	exec.Command("tmux", "kill-session", "-t", "="+p.Home).Run()
	return retileOrClose()
}

// GoToTile closes the tiles view and takes the client to where the pane lives.
func GoToTile(paneID, client string) error {
	if err := RestoreTiles(); err != nil {
		return err
	}
	args := []string{"switch-client", "-t", paneID}
	if client != "" {
		args = []string{"switch-client", "-c", client, "-t", paneID}
	}
	return exec.Command("tmux", args...).Run()
}

// findTile returns a pane in the tiles view and, when it still exists, its
// stand-in.
func findTile(panes []tilePane, paneID string) (tilePane, *tilePane, bool) {
	var found *tilePane
	byID := make(map[string]*tilePane, len(panes))
	for i := range panes {
		byID[panes[i].ID] = &panes[i]
		if panes[i].ID == paneID && panes[i].inTiles() && panes[i].Slot != "" {
			found = &panes[i]
		}
	}
	if found == nil {
		return tilePane{}, nil, false
	}
	return *found, byID[found.Slot], true
}

// retileOrClose evens out the remaining tiles, or closes the view when none
// are left.
func retileOrClose() error {
	panes, err := listTilePanes()
	if err != nil {
		return err
	}
	for _, p := range panes {
		if p.inTiles() && p.Slot != "" {
			exec.Command("tmux", "select-layout", "-t", "="+TilesSessionName+":", "tiled").Run()
			return nil
		}
	}
	return RestoreTiles()
}

// borrowPane swaps a into the stand-in's place. The options go on first so a
// crash right after the swap still leaves enough to put it back.
func borrowPane(a AgentPane, standIn string) bool {
	agent := a.Pane.ID
	setPaneOption(standIn, "remain-on-exit", "on")
	setPaneOption(standIn, optTileFor, agent)
	setPaneOption(agent, optTileHome, a.SessionName)
	setPaneOption(agent, optTileSlot, standIn)
	if exec.Command("tmux", "swap-pane", "-d", "-s", agent, "-t", standIn).Run() != nil {
		unsetPaneOption(agent, optTileHome)
		unsetPaneOption(agent, optTileSlot)
		return false
	}
	return true
}

// tilePaneFormat lists what RestoreTiles needs about every pane.
const tilePaneFormat = "#{pane_id}\t#{session_name}\t#{" + optTileHome + "}\t#{" + optTileSlot + "}\t#{" + optTileFor + "}"

type tilePane struct {
	ID, Session string
	Home, Slot  string // set on a borrowed pane
	HoldsFor    string // set on a stand-in
}

func (p tilePane) inTiles() bool { return p.Session == TilesSessionName }

// inAtmuxView reports a pane in any atmux view's session, such as the live
// browser's display slot, rather than in one of the user's own windows.
func (p tilePane) inAtmuxView() bool { return strings.HasPrefix(p.Session, "_atmux_") }

func parseTilePanes(output string) []tilePane {
	var panes []tilePane
	for _, line := range strings.Split(output, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) != 5 {
			continue
		}
		panes = append(panes, tilePane{ID: f[0], Session: f[1], Home: f[2], Slot: f[3], HoldsFor: f[4]})
	}
	return panes
}

func listTilePanes() ([]tilePane, error) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", tilePaneFormat).Output()
	if err != nil {
		return nil, err
	}
	return parseTilePanes(string(out)), nil
}

// tilesPlan is what putting the panes back takes.
type tilesPlan struct {
	SwapBack []tilePane // borrowed panes whose stand-in still holds their place
	SendHome []tilePane // borrowed panes whose stand-in is gone
	Release  []tilePane // panes carrying borrow options, to clear once home
}

func planRestore(panes []tilePane) tilesPlan {
	byID := make(map[string]tilePane, len(panes))
	for _, p := range panes {
		byID[p.ID] = p
	}
	var plan tilesPlan
	for _, p := range panes {
		if p.Slot == "" {
			continue
		}
		plan.Release = append(plan.Release, p)
		if !p.inTiles() {
			// Never swapped, or already back.
			continue
		}
		// Follow the stand-in only into the user's own windows: another atmux
		// view may have borrowed it in the meantime.
		if slot, ok := byID[p.Slot]; ok && !slot.inAtmuxView() {
			plan.SwapBack = append(plan.SwapBack, p)
		} else {
			plan.SendHome = append(plan.SendHome, p)
		}
	}
	return plan
}

// orphanedStandIns are stand-ins left in the user's windows after the panes
// went back: the pane they held a place for has exited. One lent on to
// another atmux view is left for that view to return.
func orphanedStandIns(panes []tilePane) []string {
	var ids []string
	for _, p := range panes {
		if p.HoldsFor != "" && !p.inAtmuxView() {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

// RestoreTiles puts every borrowed pane back where it came from and closes the
// tiles view. Safe to call at any time, any number of times: it reads what is
// borrowed from the panes themselves.
func RestoreTiles() error {
	panes, err := listTilePanes()
	if err != nil {
		// No server, so nothing is borrowed.
		return nil
	}
	plan := planRestore(panes)

	stuck := false
	for _, p := range plan.SwapBack {
		// swap-pane is its own inverse: re-check right before swapping so a
		// second restore running at the same moment cannot undo this one.
		if paneSession(p.ID) != TilesSessionName {
			continue
		}
		if exec.Command("tmux", "swap-pane", "-d", "-s", p.ID, "-t", p.Slot).Run() != nil {
			plan.SendHome = append(plan.SendHome, p)
		}
	}
	for _, p := range plan.SendHome {
		if paneSession(p.ID) != TilesSessionName {
			continue
		}
		if !sendPaneHome(p) {
			stuck = true
		}
	}
	for _, p := range plan.Release {
		if paneSession(p.ID) == TilesSessionName {
			continue
		}
		unsetPaneOption(p.ID, optTileHome)
		unsetPaneOption(p.ID, optTileSlot)
	}

	if after, err := listTilePanes(); err == nil {
		for _, id := range orphanedStandIns(after) {
			exec.Command("tmux", "kill-pane", "-t", id).Run()
		}
	}
	if stuck {
		// Closing the view would kill a pane that could not be moved out.
		return fmt.Errorf("some panes could not be put back; they are still in %s", TilesSessionName)
	}
	// Closing a session detaches its clients; send any still looking at the
	// tiles to where the first pane went instead.
	DisarmTilesHooks()
	home := ""
	if len(plan.Release) > 0 {
		home = plan.Release[0].Home
	}
	moveClientsOutOfTiles(home)
	exec.Command("tmux", "kill-session", "-t", TilesSessionName).Run()
	exec.Command("tmux", "unbind-key", "-a", "-T", tilesKeyTable).Run()
	return nil
}

func moveClientsOutOfTiles(home string) {
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_name}\t#{client_session}").Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		name, session, ok := strings.Cut(line, "\t")
		if !ok || session != TilesSessionName {
			continue
		}
		if home == "" || exec.Command("tmux", "switch-client", "-c", name, "-t", "="+home).Run() != nil {
			exec.Command("tmux", "switch-client", "-c", name, "-l").Run()
		}
	}
}

// RestoreTilesIfUnviewed puts the panes back once no client is looking at the
// tiles view. The hooks call it on every session change and detach.
func RestoreTilesIfUnviewed() error {
	out, err := exec.Command("tmux", "list-clients", "-F", "#{client_session}").Output()
	if err != nil {
		return nil
	}
	for _, s := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(s) == TilesSessionName {
			return nil
		}
	}
	return RestoreTiles()
}

// sendPaneHome moves a borrowed pane whose stand-in is gone into a new window
// of its own session, recreating the session if that is gone too.
func sendPaneHome(p tilePane) bool {
	home := p.Home
	if home == "" {
		return false
	}
	var spare string
	if exec.Command("tmux", "has-session", "-t", "="+home).Run() != nil {
		out, err := exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{window_id}", "-s", home).Output()
		if err != nil {
			return false
		}
		spare = strings.TrimSpace(string(out))
	}
	if exec.Command("tmux", "break-pane", "-d", "-s", p.ID, "-t", "="+home+":").Run() != nil {
		return false
	}
	if spare != "" {
		exec.Command("tmux", "kill-window", "-t", spare).Run()
	}
	return true
}

func globalOption(name string) string {
	out, err := exec.Command("tmux", "show-options", "-gv", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func paneSession(paneID string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", paneID, "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func setPaneOption(paneID, name, value string) {
	exec.Command("tmux", "set-option", "-p", "-t", paneID, name, value).Run()
}

func unsetPaneOption(paneID, name string) {
	exec.Command("tmux", "set-option", "-p", "-u", "-t", paneID, name).Run()
}

// tmuxDoubleQuote quotes s as one argument in a tmux command string, such as a
// hook. Inside double quotes tmux treats backslash, double quote and dollar
// specially.
func tmuxDoubleQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}

// DisarmTilesHooks stops leaving the tiles view from putting the panes back,
// for when atmux itself is about to rebuild it.
func DisarmTilesHooks() {
	for _, name := range tilesHooks {
		exec.Command("tmux", "set-hook", "-gu", name).Run()
	}
}
