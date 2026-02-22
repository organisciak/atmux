package tui

import (
	"strconv"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/porganisciak/agent-tmux/tmux"
)

func TestLineJumpStateMultiDigit(t *testing.T) {
	var jump lineJumpState
	now := time.Now()

	idx, ok := jump.consumeDigitAt('1', 12, now)
	if !ok || idx != 0 {
		t.Fatalf("expected first digit to jump to index 0, got idx=%d ok=%v", idx, ok)
	}

	idx, ok = jump.consumeDigitAt('0', 12, now.Add(100*time.Millisecond))
	if !ok || idx != 9 {
		t.Fatalf("expected second digit to jump to index 9 (line 10), got idx=%d ok=%v", idx, ok)
	}
}

func TestLineJumpStateTimeoutResetsSequence(t *testing.T) {
	var jump lineJumpState
	now := time.Now()

	if _, ok := jump.consumeDigitAt('1', 12, now); !ok {
		t.Fatalf("expected first digit to be accepted")
	}

	// After timeout, entering 0 should not continue "10".
	if _, ok := jump.consumeDigitAt('0', 12, now.Add(lineJumpTimeout+time.Millisecond)); ok {
		t.Fatalf("expected timed-out 0 input to be rejected as a new sequence")
	}

	idx, ok := jump.consumeDigitAt('2', 12, now.Add(lineJumpTimeout+2*time.Millisecond))
	if !ok || idx != 1 {
		t.Fatalf("expected follow-up digit to start a new sequence at index 1, got idx=%d ok=%v", idx, ok)
	}
}

func TestLineJumpStateFallsBackToLatestDigit(t *testing.T) {
	var jump lineJumpState
	now := time.Now()

	if _, ok := jump.consumeDigitAt('1', 12, now); !ok {
		t.Fatalf("expected first digit to be accepted")
	}

	// "15" is out of range for 12 items, so fallback should jump to line 5.
	idx, ok := jump.consumeDigitAt('5', 12, now.Add(100*time.Millisecond))
	if !ok || idx != 4 {
		t.Fatalf("expected fallback to latest digit (index 4), got idx=%d ok=%v", idx, ok)
	}
}

func TestSessionsModelDigitJumpMultiDigit(t *testing.T) {
	m := newSessionsModel(nil, false, false)
	m.lines = makeSessionLines(12)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(sessionsModel)
	if m.selectedIndex != 0 {
		t.Fatalf("expected selected index 0 after key '1', got %d", m.selectedIndex)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	m = updated.(sessionsModel)
	if m.selectedIndex != 9 {
		t.Fatalf("expected selected index 9 after keys '1''0', got %d", m.selectedIndex)
	}
}

func TestOpenModelDigitJumpDoesNotAutoSelect(t *testing.T) {
	m := newOpenModel()
	m.activeSessions = makeSessionLines(12)
	m.activeTab = tabActive

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(openModel)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	m = updated.(openModel)

	if m.selectedIndex != 9 {
		t.Fatalf("expected selected index 9 after keys '1''0', got %d", m.selectedIndex)
	}
	if m.selectedSession != "" {
		t.Fatalf("expected no session selection without Enter, got %q", m.selectedSession)
	}
}

func TestLandingModelDigitJumpFocusesSessionsSection(t *testing.T) {
	m := newLandingModel("agent-current")
	m.sessions = makeSessionLines(12)
	m.focusedSection = sectionResume

	updated, _ := m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(landingModel)

	updated, _ = m.handleKeyMsg(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})
	m = updated.(landingModel)

	if m.focusedSection != sectionSessions {
		t.Fatalf("expected focus to move to sessions section, got %d", m.focusedSection)
	}
	if m.selectedIndex != 9 {
		t.Fatalf("expected selected index 9 after keys '1''0', got %d", m.selectedIndex)
	}
}

func makeSessionLines(n int) []tmux.SessionLine {
	lines := make([]tmux.SessionLine, 0, n)
	for i := 1; i <= n; i++ {
		name := "agent-" + strconv.Itoa(i)
		lines = append(lines, tmux.SessionLine{
			Name: name,
			Line: name + ": 1 windows",
		})
	}
	return lines
}
