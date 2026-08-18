package tmux

import "strings"

// sessionTitleFormat asks tmux for every pane's title in one call, so the
// session list costs a single extra round trip per host rather than one per
// session. Tab-separated because titles and commands can both contain colons.
const sessionTitleFormat = "#{session_name}\t#{pane_title}\t#{pane_current_command}"

// FetchSessionTitles returns each session's agent title, keyed by session name.
//
// Claude Code keeps the terminal title set to the current conversation's name,
// which tmux exposes as pane_title. Reading it costs nothing on the agent's
// side and works whether or not the pane is the focused one.
//
// Sessions with no agent pane are absent from the map rather than present with
// an empty string, so callers can tell "no agent here" from "agent with no
// title yet".
func FetchSessionTitles(exec TmuxExecutor) map[string]string {
	output, err := exec.Output("list-panes", "-a", "-F", sessionTitleFormat)
	if err != nil {
		// A host with no server, or one that cannot answer, simply has no
		// titles to contribute. The session list is still useful without them.
		return nil
	}
	return parseSessionTitles(string(output))
}

func parseSessionTitles(output string) map[string]string {
	titles := make(map[string]string)

	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(parts) < 3 {
			continue
		}

		session := parts[0]
		pane := Pane{Title: parts[1], Command: parts[2]}
		if !isClaudePane(pane) {
			continue
		}

		title := strings.TrimSpace(pane.Title)
		if title == "" {
			continue
		}
		// First agent pane in a session wins; a session running several agents
		// is named by the one tmux lists first, which is stable across calls.
		if _, seen := titles[session]; !seen {
			titles[session] = title
		}
	}

	if len(titles) == 0 {
		return nil
	}
	return titles
}

// agentTitleGlyphs are the status glyphs Claude Code prefixes to its terminal
// title: a star when idle, braille/arc spinner frames while working.
const agentTitleGlyphs = "✳✻✽✢·✶◐◑◒◓⠂⠄⠆⠇⠋⠙⠸⠴⠦⠧●○"

// SplitAgentTitle separates the leading status glyph from the title text.
// The glyph is a live activity indicator, so callers that want a stable string
// (search, sorting, storage) should use the text and treat the glyph as
// presentation.
func SplitAgentTitle(title string) (glyph, text string) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", ""
	}

	runes := []rune(trimmed)
	if !strings.ContainsRune(agentTitleGlyphs, runes[0]) {
		return "", trimmed
	}
	return string(runes[0]), strings.TrimSpace(string(runes[1:]))
}
