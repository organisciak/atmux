package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleTranscript = `{"type":"user","message":{"role":"user","content":"Make the walls thicker"},"timestamp":"2026-09-25T15:00:00Z"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"## Plan\n\nThicker **walls** in ` + "`cad.py`" + `.\nSecond line.\n\nDetails follow."}]},"timestamp":"2026-09-25T15:01:00Z"}
{"type":"system","subtype":"away_summary","content":"Walls are thicker and committed. Next: print a test cup. (disable recaps in /config)","timestamp":"2026-09-25T16:00:00Z"}
{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]},"timestamp":"2026-09-25T16:01:00Z"}
{"type":"user","isMeta":true,"message":{"role":"user","content":"<command-name>/clear</command-name>"},"timestamp":"2026-09-25T16:02:00Z"}
`

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadTranscriptFindsRecapPromptAndReply(t *testing.T) {
	tr := readTranscriptFile(writeTranscript(t, sampleTranscript))

	if tr.Recap != "Walls are thicker and committed. Next: print a test cup." {
		t.Errorf("recap = %q", tr.Recap)
	}
	// Tool results and meta entries are not prompts.
	if tr.LastPrompt != "Make the walls thicker" {
		t.Errorf("last prompt = %q", tr.LastPrompt)
	}
	if tr.LastReply != "Thicker walls in cad.py. Second line." {
		t.Errorf("last reply = %q", tr.LastReply)
	}
	if !tr.RecapIsCurrent() {
		t.Error("recap written after the last prompt should be current")
	}
	if tr.Empty {
		t.Error("transcript with a prompt reported empty")
	}
}

func TestRecapGoesStaleAfterANewPrompt(t *testing.T) {
	body := sampleTranscript + `{"type":"user","message":{"role":"user","content":"Now the lid"},"timestamp":"2026-09-25T17:00:00Z"}` + "\n"
	tr := readTranscriptFile(writeTranscript(t, body))
	if tr.RecapIsCurrent() {
		t.Error("recap older than the last prompt reported current")
	}
	if tr.LastPrompt != "Now the lid" {
		t.Errorf("last prompt = %q", tr.LastPrompt)
	}
}

func TestTranscriptWithoutPromptsIsEmpty(t *testing.T) {
	tr := readTranscriptFile(writeTranscript(t, `{"type":"permission-mode","permissionMode":"auto"}`+"\n"))
	if !tr.Empty {
		t.Error("transcript with no prompts should be empty")
	}
}

func TestReadTranscriptScansBeyondTheFirstTailWindow(t *testing.T) {
	filler := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"` + strings.Repeat("x", 1000) + `"}]}}` + "\n"
	body := sampleTranscript + strings.Repeat(filler, 400) // ~400KB, past the first window
	tr := readTranscriptFile(writeTranscript(t, body))
	if tr.Recap == "" || tr.LastPrompt == "" {
		t.Fatalf("missed entries before the first tail window: %+v", tr)
	}
}

func TestReadTranscriptCacheSeesAppends(t *testing.T) {
	path := writeTranscript(t, sampleTranscript)
	if readTranscriptFile(path).LastPrompt != "Make the walls thicker" {
		t.Fatal("first read wrong")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"type":"user","message":{"role":"user","content":"Now the lid"},"timestamp":"2026-09-25T17:00:00Z"}` + "\n")
	f.Close()
	if got := readTranscriptFile(path).LastPrompt; got != "Now the lid" {
		t.Fatalf("cache served a stale read: %q", got)
	}
}

func TestFirstParagraphSkipsHeadingsAndFences(t *testing.T) {
	md := "```\ncode\n```\n# Title\n\nFirst **bold** line\ncontinues.\n\nSecond."
	if got := FirstParagraph(md); got != "First bold line continues." {
		t.Fatalf("got %q", got)
	}
}
