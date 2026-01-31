package tmux

import "testing"

func TestParseSessionLine(t *testing.T) {
	line := "agent-foo: 2 windows (created Fri Jan 30 10:00:00 2026) [80x24]"
	parsed := parseSessionLine(line)
	if parsed.Name != "agent-foo" {
		t.Fatalf("expected name agent-foo, got %q", parsed.Name)
	}
	if parsed.Line != line {
		t.Fatalf("expected line %q, got %q", line, parsed.Line)
	}
}
