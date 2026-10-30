package tmux

import (
	"reflect"
	"testing"
	"time"

	"github.com/porganisciak/agent-tmux/claude"
)

func TestTileCount(t *testing.T) {
	cases := []struct{ w, h, want int }{
		{80, 24, 1},
		{60, 15, 1}, // smaller than one tile still gets one
		{190, 49, 4},
		{320, 80, 16},
	}
	for _, c := range cases {
		if got := TileCount(c.w, c.h); got != c.want {
			t.Errorf("TileCount(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestRecentAgentsPutsActiveFirstThenNewest(t *testing.T) {
	now := time.Now()
	agent := func(id, session string, status claude.Status, ago time.Duration) AgentPane {
		return AgentPane{
			ClaudePane: ClaudePane{Pane: Pane{ID: id}, SessionName: session, Executor: NewLocalExecutor()},
			Status:     status,
			Since:      now.Add(-ago),
		}
	}
	agents := []AgentPane{
		agent("%1", "old", claude.StatusIdle, 3*time.Hour),
		agent("%2", "recent", claude.StatusIdle, 2*time.Minute),
		agent("%3", "busy", claude.StatusBusy, 40*time.Minute), // working since long ago still counts as now
		agent("%4", "_atmux_live", claude.StatusIdle, time.Minute),
		agent("%5", "waiting", claude.StatusWaiting, 10*time.Minute),
		{ClaudePane: ClaudePane{Pane: Pane{ID: "%6"}, SessionName: "remote", Executor: &RemoteExecutor{Host: "vps"}}, Status: claude.StatusBusy},
	}
	var got []string
	for _, a := range RecentAgents(agents, now) {
		got = append(got, a.SessionName)
	}
	want := []string{"busy", "waiting", "recent", "old"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPlanRestore(t *testing.T) {
	panes := parseTilePanes(
		// Borrowed, stand-in in place: swap back.
		"%1\t_atmux_tiles\twork\t%10\t\n" +
			"%10\twork\t\t\t%1\n" +
			// Borrowed, stand-in killed: send home.
			"%2\t_atmux_tiles\tdocs\t%11\t\n" +
			// Options set but never swapped (crash before swap-pane): just release.
			"%3\tsite\tsite\t%12\t\n" +
			"%12\t_atmux_tiles\t\t\t%3\n" +
			// Stand-in lent on to the live browser: send home instead.
			"%5\t_atmux_tiles\tnotes\t%13\t\n" +
			"%13\t_atmux_live\t\t\t%5\n" +
			// Home session renamed while tiled: the stand-in moved with it.
			"%6\t_atmux_tiles\told-name\t%14\t\n" +
			"%14\tnew-name\t\t\t%6\n" +
			// Unrelated pane.
			"%4\twork\t\t\t\n")
	plan := planRestore(panes)
	ids := func(ps []tilePane) []string {
		var out []string
		for _, p := range ps {
			out = append(out, p.ID)
		}
		return out
	}
	if got := ids(plan.SwapBack); !reflect.DeepEqual(got, []string{"%1", "%6"}) {
		t.Errorf("SwapBack = %v", got)
	}
	if got := ids(plan.SendHome); !reflect.DeepEqual(got, []string{"%2", "%5"}) {
		t.Errorf("SendHome = %v", got)
	}
	if got := ids(plan.Release); !reflect.DeepEqual(got, []string{"%1", "%2", "%3", "%5", "%6"}) {
		t.Errorf("Release = %v", got)
	}
}

func TestOrphanedStandInsAreTheOnesLeftInOtherWindows(t *testing.T) {
	panes := parseTilePanes(
		"%10\twork\t\t\t%1\n" + // its pane exited while borrowed
			"%11\t_atmux_tiles\t\t\t%2\n" + // back in the view, closed with it
			"%12\t_atmux_live\t\t\t%3\n" + // lent on; the live browser returns it
			"%4\twork\t\t\t\n")
	if got := orphanedStandIns(panes); !reflect.DeepEqual(got, []string{"%10"}) {
		t.Fatalf("got %v", got)
	}
}

func TestTmuxDoubleQuote(t *testing.T) {
	got := tmuxDoubleQuote(`'/Users/me/my "bin"/atmux' tiles $HOME\x`)
	want := `"'/Users/me/my \"bin\"/atmux' tiles \$HOME\\x"`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestTilesSessionIsHiddenFromSessionLists(t *testing.T) {
	out := "1790789277\t/tmp\t1\t_atmux_tiles: 1 windows (created now) (attached)\n" +
		"1790789277\t/tmp\t0\twork: 2 windows (created now)\n"
	lines := parseSessionLines(out)
	if len(lines) != 1 || lines[0].Name != "work" {
		t.Fatalf("got %+v", lines)
	}
}

func TestCopyKeyTableRetargetsEveryBinding(t *testing.T) {
	in := "bind-key    -T root MouseDown1Pane select-pane -t = \\; send-keys -M\n" +
		"bind-key -r -T root C-Up resize-pane -U\n\n"
	want := "bind-key    -T atmux-tiles MouseDown1Pane select-pane -t = \\; send-keys -M\n" +
		"bind-key -r -T atmux-tiles C-Up resize-pane -U\n"
	if got := copyKeyTable(in, "root", "atmux-tiles"); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// display-menu reads name, key, command triples, except that a separator is
// the empty name alone; a stray argument shifts every later entry.
func TestTileMenuItemsAreWellFormed(t *testing.T) {
	items := tileMenuItems(func(flags string) string { return "run-shell " + tmuxDoubleQuote("atmux tiles "+flags) })
	var names []string
	for i := 0; i < len(items); {
		if items[i] == "" {
			i++
			continue
		}
		if i+2 >= len(items) {
			t.Fatalf("entry %q is missing its key or command", items[i])
		}
		names = append(names, items[i])
		if len(items[i+1]) != 1 {
			t.Fatalf("entry %q has key %q", items[i], items[i+1])
		}
		i += 3
	}
	want := []string{"Zoom", "Put back in its window", "Go to its window", "Close its session"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("entries %v, want %v", names, want)
	}
}

func TestFindTile(t *testing.T) {
	panes := parseTilePanes(
		"%1\t_atmux_tiles\twork\t%10\t\n" +
			"%10\twork\t\t\t%1\n" +
			"%2\twork\t\t\t\n")
	p, slot, ok := findTile(panes, "%1")
	if !ok || p.Home != "work" || slot == nil || slot.ID != "%10" {
		t.Fatalf("findTile(%%1) = %+v, %v, %v", p, slot, ok)
	}
	if _, _, ok := findTile(panes, "%2"); ok {
		t.Fatal("a pane that is not tiled was found")
	}
}
