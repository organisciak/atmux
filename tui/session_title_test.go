package tui

import (
	"strings"
	"testing"

	"github.com/porganisciak/agent-tmux/tmux"
)

func TestTitleLabel_KeepsGlyphUndimmed(t *testing.T) {
	m := modelWithHosts(&stubHostExecutor{})
	label := m.titleLabel(tmux.SessionLine{Title: "◐ Design remote sessions"})

	if !strings.Contains(label, "Design remote sessions") {
		t.Fatalf("title text missing: %q", label)
	}
	// The glyph is the activity indicator, so it must survive rendering.
	if !strings.Contains(label, "◐") {
		t.Fatalf("activity glyph missing: %q", label)
	}
}

func TestTitleLabel_EmptyForSessionsWithoutAgents(t *testing.T) {
	m := modelWithHosts(&stubHostExecutor{})
	if got := m.titleLabel(tmux.SessionLine{}); got != "" {
		t.Fatalf("expected no label without a title, got %q", got)
	}
	if got := m.titleLabel(tmux.SessionLine{Title: "   "}); got != "" {
		t.Fatalf("expected no label for a blank title, got %q", got)
	}
}

func TestAppendTitle_DroppedWhenItWouldWrap(t *testing.T) {
	m := modelWithHosts(&stubHostExecutor{})
	m.width = 30

	row := "  1. agent-foo: 1 windows"
	got := m.appendTitle(row, tmux.SessionLine{Title: "✳ A title far too long for this width"})

	// The title is the least critical column, so it goes rather than wrapping.
	if got != row {
		t.Fatalf("expected the row unchanged on a narrow terminal, got %q", got)
	}
}

func TestAppendTitle_IncludedWhenItFits(t *testing.T) {
	m := modelWithHosts(&stubHostExecutor{})
	m.width = 200

	row := "  1. agent-foo: 1 windows"
	got := m.appendTitle(row, tmux.SessionLine{Title: "✳ Short title"})

	if !strings.Contains(got, "Short title") {
		t.Fatalf("expected the title appended, got %q", got)
	}
	if !strings.HasPrefix(got, row) {
		t.Fatalf("expected the original row preserved, got %q", got)
	}
}

func TestAppendTitle_UnknownWidthStillRenders(t *testing.T) {
	// Before the first WindowSizeMsg the width is zero; the title should not be
	// suppressed forever because of that.
	m := modelWithHosts(&stubHostExecutor{})
	m.width = 0

	got := m.appendTitle("  1. agent-foo", tmux.SessionLine{Title: "✳ Visible"})
	if !strings.Contains(got, "Visible") {
		t.Fatalf("expected the title with unknown width, got %q", got)
	}
}
