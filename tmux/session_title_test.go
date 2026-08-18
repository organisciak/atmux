package tmux

import "testing"

func TestParseSessionTitles_PicksAgentPanes(t *testing.T) {
	// Claude Code reports its own semver as pane_current_command, which is how
	// an agent pane is told apart from a shell that happens to have a title.
	output := "agent-foo\t✳ Teaching data collection\t2.1.234\n" +
		"agent-bar\t◐ Working on something\t2.1.220\n"

	titles := parseSessionTitles(output)
	if got := titles["agent-foo"]; got != "✳ Teaching data collection" {
		t.Errorf("agent-foo title = %q", got)
	}
	if got := titles["agent-bar"]; got != "◐ Working on something" {
		t.Errorf("agent-bar title = %q", got)
	}
}

func TestParseSessionTitles_IgnoresNonAgentPanes(t *testing.T) {
	// A shell's title is the hostname, and a dev server is not an agent.
	output := "agent-foo\tMCE-PWVGQ2C2DJ\tzsh\n" +
		"agent-foo\tsome-dev-server\tnode\n"

	if titles := parseSessionTitles(output); len(titles) != 0 {
		t.Fatalf("expected no agent titles, got %v", titles)
	}
}

func TestParseSessionTitles_FirstAgentPaneWins(t *testing.T) {
	output := "agent-foo\t✳ First\t2.1.234\n" +
		"agent-foo\t✳ Second\t2.1.234\n"

	if got := parseSessionTitles(output)["agent-foo"]; got != "✳ First" {
		t.Fatalf("expected the first agent pane to name the session, got %q", got)
	}
}

func TestParseSessionTitles_AbsentRatherThanEmpty(t *testing.T) {
	// A session with no agent must be absent from the map, so callers can tell
	// "no agent here" from "agent that has not named itself".
	output := "agent-foo\t\t2.1.234\n"
	titles := parseSessionTitles(output)
	if _, present := titles["agent-foo"]; present {
		t.Fatal("a blank title should not be recorded")
	}
}

func TestParseSessionTitles_SurvivesTitlesContainingColons(t *testing.T) {
	// Tab-separated precisely because titles can contain colons.
	output := "agent-foo\t✳ Fix bug: the parser breaks\t2.1.234\n"
	if got := parseSessionTitles(output)["agent-foo"]; got != "✳ Fix bug: the parser breaks" {
		t.Fatalf("title with colon mangled: %q", got)
	}
}

func TestParseSessionTitles_HandlesJunkLines(t *testing.T) {
	output := "\nnot-enough-fields\nagent-foo\t✳ Real\t2.1.234\n"
	if got := parseSessionTitles(output)["agent-foo"]; got != "✳ Real" {
		t.Fatalf("expected junk lines to be skipped, got %q", got)
	}
}

func TestSplitAgentTitle_SeparatesGlyph(t *testing.T) {
	cases := []struct{ in, glyph, text string }{
		{"✳ Teaching data collection", "✳", "Teaching data collection"},
		{"◐ Design remote session management", "◐", "Design remote session management"},
		{"⠂ concurrent-drafts-design-plan", "⠂", "concurrent-drafts-design-plan"},
		{"No glyph at all", "", "No glyph at all"},
		{"", "", ""},
	}
	for _, c := range cases {
		glyph, text := SplitAgentTitle(c.in)
		if glyph != c.glyph || text != c.text {
			t.Errorf("SplitAgentTitle(%q) = (%q,%q), want (%q,%q)", c.in, glyph, text, c.glyph, c.text)
		}
	}
}

func TestSplitAgentTitle_DoesNotEatRealFirstLetters(t *testing.T) {
	// A title starting with an ordinary word must keep every character.
	if _, text := SplitAgentTitle("Obsidian Dataview example"); text != "Obsidian Dataview example" {
		t.Fatalf("text = %q", text)
	}
}

func TestSessionTitleFormat_IsTabSeparated(t *testing.T) {
	// Colons appear in both titles and commands, so the separator must not be
	// one. Guard against someone "tidying" this back to colons.
	if want := "#{session_name}\t#{pane_title}\t#{pane_current_command}"; sessionTitleFormat != want {
		t.Fatalf("format changed to %q", sessionTitleFormat)
	}
}
