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
	if got := ids(plan.SwapBack); !reflect.DeepEqual(got, []string{"%1"}) {
		t.Errorf("SwapBack = %v", got)
	}
	if got := ids(plan.SendHome); !reflect.DeepEqual(got, []string{"%2"}) {
		t.Errorf("SendHome = %v", got)
	}
	if got := ids(plan.Release); !reflect.DeepEqual(got, []string{"%1", "%2", "%3"}) {
		t.Errorf("Release = %v", got)
	}
}

func TestOrphanedStandInsAreTheOnesLeftInOtherWindows(t *testing.T) {
	panes := parseTilePanes(
		"%10\twork\t\t\t%1\n" + // its pane exited while borrowed
			"%11\t_atmux_tiles\t\t\t%2\n" + // back in the view, closed with it
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
