package claude

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Transcript is what atmux needs from a session's transcript: enough to say
// what the session is about and where it left off, without reading history.
type Transcript struct {
	// Recap is Claude's most recent "while you were away" summary.
	Recap   string
	RecapAt time.Time

	// LastPrompt is the most recent thing the user typed.
	LastPrompt   string
	LastPromptAt time.Time

	// LastReply is the opening paragraph of Claude's most recent reply, a
	// fallback for sessions whose recap is missing or out of date. Replies
	// tend to lead with their conclusion, so the opening is the summary.
	LastReply string

	// Empty means the session has no conversation yet.
	Empty bool
}

// RecapIsCurrent reports whether the recap still describes the session: a
// prompt sent after it means the work has moved on.
func (t Transcript) RecapIsCurrent() bool {
	return t.Recap != "" && !t.LastPromptAt.After(t.RecapAt)
}

func (t Transcript) complete() bool {
	return t.Recap != "" && t.LastReply != "" && !t.LastPromptAt.IsZero() && t.LastPrompt != ""
}

// Tail sizes to scan. Transcripts run to tens of megabytes, but everything we
// want is near the end; the larger window covers a turn whose tool output
// pushed the last reply further back.
var transcriptTailSizes = []int64{256 << 10, 4 << 20}

// recapSuffix is the settings hint Claude appends to every recap.
const recapSuffix = "(disable recaps in /config)"

// TranscriptPath finds a session's transcript. Project directories are named
// by an encoding of the working directory that has changed between Claude
// Code releases, so we match on the session id instead.
func TranscriptPath(sessionID string) string {
	if sessionID == "" || strings.ContainsAny(sessionID, `/\*?[`) {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(Dir(), "projects", "*", sessionID+".jsonl"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// ReadTranscript summarises the transcript for a session. A session with no
// transcript file yet is reported as Empty.
func ReadTranscript(sessionID string) Transcript {
	path := TranscriptPath(sessionID)
	if path == "" {
		return Transcript{Empty: true}
	}
	return readTranscriptFile(path)
}

// transcriptCache remembers each transcript's summary until the file changes.
// The overview refreshes every couple of seconds, and most sessions sit idle,
// so nearly every read is a hit.
var transcriptCache = struct {
	sync.Mutex
	entries map[string]cachedTranscript
}{entries: make(map[string]cachedTranscript)}

type cachedTranscript struct {
	size    int64
	modTime time.Time
	t       Transcript
}

func readTranscriptFile(path string) Transcript {
	f, err := os.Open(path)
	if err != nil {
		return Transcript{Empty: true}
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Transcript{}
	}

	transcriptCache.Lock()
	c, ok := transcriptCache.entries[path]
	transcriptCache.Unlock()
	if ok && c.size == info.Size() && c.modTime.Equal(info.ModTime()) {
		return c.t
	}

	t := scanTranscriptFile(f, info.Size())

	transcriptCache.Lock()
	transcriptCache.entries[path] = cachedTranscript{size: info.Size(), modTime: info.ModTime(), t: t}
	transcriptCache.Unlock()
	return t
}

func scanTranscriptFile(f *os.File, fileSize int64) Transcript {

	var t Transcript
	for _, size := range transcriptTailSizes {
		whole := size >= fileSize
		start := fileSize - size
		if whole {
			start = 0
		}
		buf := make([]byte, fileSize-start)
		if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
			return Transcript{}
		}
		t = scanTranscript(buf, !whole)
		// Only the whole file can prove a session has no prompts.
		t.Empty = whole && t.LastPromptAt.IsZero() && t.LastPrompt == ""
		if whole || t.complete() {
			break
		}
	}
	return t
}

// entry covers the transcript line shapes we read. Everything else is skipped.
type entry struct {
	Type       string    `json:"type"`
	Subtype    string    `json:"subtype"`
	Content    string    `json:"content"`
	IsMeta     bool      `json:"isMeta"`
	Sidechain  bool      `json:"isSidechain"`
	Timestamp  time.Time `json:"timestamp"`
	LastPrompt string    `json:"lastPrompt"`
	Message    *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// scanTranscript walks lines newest-first, keeping the first of each thing it
// finds. partial means buf starts mid-line, so the first line is dropped.
func scanTranscript(buf []byte, partial bool) Transcript {
	lines := bytes.Split(buf, []byte("\n"))
	if partial && len(lines) > 0 {
		lines = lines[1:]
	}

	var t Transcript
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if len(line) == 0 || !mightMatter(line) {
			continue
		}
		var e entry
		if json.Unmarshal(line, &e) != nil || e.Sidechain {
			continue
		}

		switch {
		case e.Type == "system" && e.Subtype == "away_summary":
			if t.Recap == "" {
				t.Recap = cleanRecap(e.Content)
				t.RecapAt = e.Timestamp
			}
		case e.Type == "last-prompt":
			// Written after each prompt, but without a timestamp; the
			// matching user entry supplies that.
			if t.LastPrompt == "" {
				t.LastPrompt = e.LastPrompt
			}
		case e.Type == "user" && !e.IsMeta && e.Message != nil:
			if t.LastPromptAt.IsZero() {
				if text, ok := promptText(e.Message.Content); ok {
					t.LastPromptAt = e.Timestamp
					if t.LastPrompt == "" {
						t.LastPrompt = text
					}
				}
			}
		case e.Type == "assistant" && e.Message != nil:
			if t.LastReply == "" {
				t.LastReply = replyText(e.Message.Content)
			}
		}

		if t.complete() {
			break
		}
	}
	return t
}

// mightMatter skips the bulk of a transcript — tool calls and results, file
// snapshots — without paying for a JSON decode.
func mightMatter(line []byte) bool {
	return bytes.Contains(line, []byte(`"away_summary"`)) ||
		bytes.Contains(line, []byte(`"type":"last-prompt"`)) ||
		(bytes.Contains(line, []byte(`"type":"user"`)) && !bytes.Contains(line, []byte(`"tool_result"`))) ||
		(bytes.Contains(line, []byte(`"type":"assistant"`)) && bytes.Contains(line, []byte(`"type":"text"`)))
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// promptText returns what the user typed. Tool results also arrive as user
// messages, and slash commands and hook output are wrapped in markup; neither
// is a prompt.
func promptText(raw json.RawMessage) (string, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, isTypedPrompt(s)
	}
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	for _, b := range blocks {
		if b.Type == "text" && isTypedPrompt(b.Text) {
			return b.Text, true
		}
	}
	return "", false
}

func isTypedPrompt(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && !strings.HasPrefix(s, "<")
}

func replyText(raw json.RawMessage) string {
	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" {
			if p := FirstParagraph(b.Text); p != "" {
				return p
			}
		}
	}
	return ""
}

// FirstParagraph returns the first paragraph of markdown as plain text,
// skipping headings and code fences, with inline emphasis removed.
func FirstParagraph(md string) string {
	var para []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, trimmed)
	}
	return plainInline(strings.Join(para, " "))
}

var inlineMarkup = strings.NewReplacer("**", "", "__", "", "`", "")

func plainInline(s string) string {
	return strings.TrimSpace(inlineMarkup.Replace(s))
}

func cleanRecap(s string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), recapSuffix))
}
