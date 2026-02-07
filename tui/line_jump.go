package tui

import (
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const lineJumpTimeout = 1200 * time.Millisecond

// lineJumpState tracks numeric key sequences used to jump to list lines.
type lineJumpState struct {
	input  string
	lastAt time.Time
}

// consumeKey consumes a key event and resolves it to a 0-based line index.
func (s *lineJumpState) consumeKey(msg tea.KeyMsg, maxItems int) (int, bool) {
	digit, ok := keyDigit(msg)
	if !ok {
		s.reset()
		return 0, false
	}
	return s.consumeDigitAt(digit, maxItems, time.Now())
}

func (s *lineJumpState) consumeDigitAt(digit rune, maxItems int, now time.Time) (int, bool) {
	if maxItems <= 0 || digit < '0' || digit > '9' {
		s.reset()
		return 0, false
	}

	if s.input == "" || now.Sub(s.lastAt) > lineJumpTimeout {
		// Line numbers are 1-based, so a sequence cannot start with 0.
		if digit == '0' {
			s.reset()
			return 0, false
		}
		s.input = string(digit)
	} else {
		s.input += string(digit)
	}
	s.lastAt = now

	if idx, ok := lineIndexFromInput(s.input, maxItems); ok {
		return idx, true
	}

	// If the combined number is out of range, try falling back to just the
	// latest digit so quick corrections still work naturally.
	fallback := string(digit)
	if idx, ok := lineIndexFromInput(fallback, maxItems); ok {
		s.input = fallback
		return idx, true
	}

	s.reset()
	return 0, false
}

func (s *lineJumpState) reset() {
	s.input = ""
	s.lastAt = time.Time{}
}

func lineIndexFromInput(input string, maxItems int) (int, bool) {
	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > maxItems {
		return 0, false
	}
	return n - 1, true
}

func keyDigit(msg tea.KeyMsg) (rune, bool) {
	if len(msg.Runes) != 1 {
		return 0, false
	}
	d := msg.Runes[0]
	if d < '0' || d > '9' {
		return 0, false
	}
	return d, true
}
