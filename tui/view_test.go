package tui

import (
	"testing"

	"github.com/porganisciak/agent-tmux/tmux"
)

func TestRenderTreeAddsEscapeButton(t *testing.T) {
	m := NewModel(Options{})
	m.width = 120
	m.height = 40
	m.calculateLayout()

	m.tree = &tmux.Tree{
		Sessions: []tmux.TmuxSession{
			{
				Name:     "sess",
				Attached: true,
				Windows: []tmux.Window{
					{
						Index:  0,
						Name:   "win",
						Active: true,
						Panes: []tmux.Pane{
							{
								Index:  0,
								Title:  "pane",
								Active: true,
								Target: "sess:0.0",
							},
						},
					},
				},
			},
		},
	}
	m.rebuildFlatNodes()
	m.calculateButtonZones()

	// Expected: 1 help (top) + 1 ATT (session) + 1 ATT (window) + 1 SEND + 1 ESC + 1 ATT (pane) +
	//           5 status bar hints (refresh, attach, killhint, focusinput, help) = 11 zones
	if len(m.buttonZones) != 11 {
		types := make([]string, 0, len(m.flatNodes))
		for _, node := range m.flatNodes {
			types = append(types, node.Type)
		}
		t.Fatalf("expected 11 button zones, got %d (nodes=%v)", len(m.buttonZones), types)
	}

	actions := map[string]int{}
	for _, zone := range m.buttonZones {
		actions[zone.action]++
		if zone.action == buttonActionSend || zone.action == buttonActionEscape {
			if zone.target != "sess:0.0" {
				t.Fatalf("expected target sess:0.0 for %s, got %q", zone.action, zone.target)
			}
		}
	}

	if actions[buttonActionSend] != 1 || actions[buttonActionEscape] != 1 || actions[buttonActionAttach] != 4 || actions[buttonActionHelp] != 2 ||
		actions[buttonActionRefresh] != 1 || actions[buttonActionKillHint] != 1 || actions[buttonActionFocusInput] != 1 {
		t.Fatalf("expected send=1, escape=1, attach=4, help=2, refresh=1, killhint=1, focusinput=1, got %+v", actions)
	}
}
