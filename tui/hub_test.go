package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/porganisciak/agent-tmux/tmux"
)

func hubTestModel() sessionsModel {
	m := newSessionsModelWithOpts(SessionsOptions{
		Executors: []tmux.TmuxExecutor{tmux.NewLocalExecutor()},
		Views: []HubView{
			{Name: "Agents", Summary: "Cards for every Claude.", Command: "atmux agents", Key: "prefix a"},
			{Name: "Tiles", Summary: "Panes side by side.", Command: "atmux tiles"},
		},
	})
	m.width, m.height = 100, 30
	m.lines = []tmux.SessionLine{{Name: "alpha"}, {Name: "beta"}}
	return m
}

func sendKey(t *testing.T, m sessionsModel, msg tea.KeyMsg) (sessionsModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(sessionsModel), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestViewTabsCycleWithArrowsAndTab(t *testing.T) {
	m := hubTestModel()
	steps := []struct {
		key  tea.KeyMsg
		want int
	}{
		{tea.KeyMsg{Type: tea.KeyRight}, 1},
		{tea.KeyMsg{Type: tea.KeyTab}, 2},
		{tea.KeyMsg{Type: tea.KeyRight}, 0}, // wraps back to the list
		{tea.KeyMsg{Type: tea.KeyLeft}, 2},
		{tea.KeyMsg{Type: tea.KeyShiftTab}, 1},
	}
	for i, s := range steps {
		m, _ = sendKey(t, m, s.key)
		if m.viewTab != s.want {
			t.Fatalf("step %d: viewTab = %d, want %d", i, m.viewTab, s.want)
		}
	}
}

func TestEnterOnAViewTabOpensIt(t *testing.T) {
	m := hubTestModel()
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.openView != "Agents" || !isQuit(cmd) {
		t.Fatalf("openView = %q, quit = %v; want Agents and quit", m.openView, isQuit(cmd))
	}
	if m.attachSession != "" {
		t.Fatalf("attachSession = %q; Enter on a view tab must not attach the hidden selection", m.attachSession)
	}
}

func TestEnterOnTheListStillAttaches(t *testing.T) {
	m := hubTestModel()
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.openView != "" || m.attachSession != "alpha" {
		t.Fatalf("openView = %q, attachSession = %q; want the first session attached", m.openView, m.attachSession)
	}
}

func TestViewTabKeepsKeysAwayFromTheHiddenList(t *testing.T) {
	m := hubTestModel()
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRight})

	m, _ = sendKey(t, m, runes("b"))
	if m.searchActive {
		t.Fatal("typing on a view tab started a search of the hidden list")
	}
	m, _ = sendKey(t, m, runes("x"))
	if m.confirmKill {
		t.Fatal("x on a view tab asked to kill the hidden selection")
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedIndex != 0 {
		t.Fatalf("down on a view tab moved the hidden selection to %d", m.selectedIndex)
	}

	m, cmd := sendKey(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.viewTab != 0 || isQuit(cmd) {
		t.Fatalf("esc on a view tab: viewTab = %d, quit = %v; want back on the list", m.viewTab, isQuit(cmd))
	}
	_, cmd = sendKey(t, m, runes("q"))
	if !isQuit(cmd) {
		t.Fatal("q no longer quits")
	}
}

func TestLettersStillSearchTheList(t *testing.T) {
	m := hubTestModel()
	m, _ = sendKey(t, m, runes("l"))
	if !m.searchActive || m.searchQuery != "l" || m.viewTab != 0 {
		t.Fatalf("searchActive = %v, query = %q, viewTab = %d; letters belong to search", m.searchActive, m.searchQuery, m.viewTab)
	}
	// While searching, arrows edit the search, not the tabs.
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.viewTab != 0 {
		t.Fatalf("right while searching switched to tab %d", m.viewTab)
	}
}

func TestClickingTabsSelectsThenOpens(t *testing.T) {
	m := hubTestModel()
	strip := ansi.Strip(renderViewStrip(m.views, 0, m.width))
	x := strings.Index(strip, "Tiles")
	click := tea.MouseMsg{X: x + 1, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}

	next, cmd := m.Update(click)
	m = next.(sessionsModel)
	if m.viewTab != 2 || isQuit(cmd) {
		t.Fatalf("first click: viewTab = %d, quit = %v; want Tiles selected", m.viewTab, isQuit(cmd))
	}
	next, cmd = m.Update(click)
	m = next.(sessionsModel)
	if m.openView != "Tiles" || !isQuit(cmd) {
		t.Fatalf("second click: openView = %q; want Tiles opened", m.openView)
	}
}

func TestClickingTheSessionsTabGoesBackToTheList(t *testing.T) {
	m := hubTestModel()
	m.viewTab = 1
	next, _ := m.Update(tea.MouseMsg{X: 0, Y: 0, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
	if got := next.(sessionsModel).viewTab; got != 0 {
		t.Fatalf("viewTab = %d, want 0", got)
	}
}

func TestStartsOnTheRequestedView(t *testing.T) {
	m := newSessionsModelWithOpts(SessionsOptions{
		Executors: []tmux.TmuxExecutor{tmux.NewLocalExecutor()},
		Views:     []HubView{{Name: "Agents"}, {Name: "Tiles"}},
		View:      "Tiles",
	})
	if m.viewTab != 2 {
		t.Fatalf("viewTab = %d, want 2", m.viewTab)
	}
}

func TestViewCardShowsWhatTheViewIsAndHowToReachIt(t *testing.T) {
	m := hubTestModel()
	m.viewTab = 1
	out := ansi.Strip(m.View())
	for _, want := range []string{"Sessions  Agents  Tiles", "Cards for every Claude.", "prefix a", "atmux agents", "Enter or click to open"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "alpha") {
		t.Errorf("view tab still shows the session list:\n%s", out)
	}
}

func TestViewStripFitsTheWidth(t *testing.T) {
	views := []HubView{{Name: "Agents"}, {Name: "Tiles"}, {Name: "Browse"}}
	if w := ansi.StringWidth(renderViewStrip(views, 1, 20)); w > 20 {
		t.Fatalf("strip is %d wide at width 20", w)
	}
}

func TestListWithoutViewsKeepsItsTitle(t *testing.T) {
	m := hubTestModel()
	m.views = nil
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.viewTab != 0 {
		t.Fatal("right switched tabs with no views")
	}
	if first := strings.TrimSpace(strings.SplitN(ansi.Strip(m.View()), "\n", 2)[0]); first != "Sessions" {
		t.Fatalf("title = %q, want Sessions", first)
	}
}
