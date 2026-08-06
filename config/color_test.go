package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeColor(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"#42b883", "#42b883", false},
		{"42b883", "#42b883", false},
		{"#ABC", "#aabbcc", false},
		{"ABC", "#aabbcc", false},
		{"colour39", "colour39", false},
		{"COLOUR255", "colour255", false},
		{"red", "red", false},
		{"BrightBlue", "brightblue", false},
		{"default", "default", false},
		{"", "", true},
		{"not-a-color", "", true},
		{"#12345", "", true},
		{"colour9999", "", true},
		{"chartreuse", "", true}, // not a tmux named color
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := NormalizeColor(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeColor(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestContrastingFg(t *testing.T) {
	cases := map[string]string{
		"#000000": "white",
		"#111111": "white",
		"#ffffff": "black",
		"#42b883": "black", // bright green -> dark text
		"#1857a4": "white", // dark blue -> light text
		"red":     "white", // non-hex defaults to white
		"colour39": "white",
		"":         "white",
	}
	for bg, want := range cases {
		if got := ContrastingFg(bg); got != want {
			t.Errorf("ContrastingFg(%q) = %q, want %q", bg, got, want)
		}
	}
}

func TestRandomSurpriseColorAvoidsCurrent(t *testing.T) {
	current := SurprisePalette[0]
	for i := 0; i < 20; i++ {
		got, err := RandomSurpriseColor(current)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.EqualFold(got, current) {
			t.Fatalf("RandomSurpriseColor returned the avoided color %q", got)
		}
	}
}

func TestParseColorDirective(t *testing.T) {
	path := writeTempConfig(t, `
color:#42b883
`)
	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Color != "#42b883" {
		t.Fatalf("expected color #42b883, got %q", cfg.Color)
	}
}

func TestParseColorDirectiveInvalid(t *testing.T) {
	path := writeTempConfig(t, `
color:not-a-color
`)
	if _, err := Parse(path); err == nil {
		t.Fatalf("expected error for invalid color")
	}
}

func TestMergeConfigsColorLocalOverridesGlobal(t *testing.T) {
	g := &Config{Color: "#111111"}
	l := &Config{Color: "#222222"}
	merged := mergeConfigs(g, l)
	if merged.Color != "#222222" {
		t.Fatalf("expected local color to win, got %q", merged.Color)
	}

	// Local empty falls back to global.
	merged = mergeConfigs(g, &Config{})
	if merged.Color != "#111111" {
		t.Fatalf("expected global color to apply, got %q", merged.Color)
	}
}

func TestSetLocalDirectiveCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent-tmux.conf")
	if err := SetLocalDirective(path, "color", "#42b883"); err != nil {
		t.Fatalf("SetLocalDirective: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(body), "color:#42b883") {
		t.Fatalf("file missing color directive: %s", body)
	}
}

func TestSetLocalDirectiveUpdatesInPlace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent-tmux.conf")
	initial := `# header
window:dev
color:#aaaaaa
pane:nvim .
`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetLocalDirective(path, "color", "#42b883"); err != nil {
		t.Fatalf("SetLocalDirective: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(body)
	if strings.Contains(s, "#aaaaaa") {
		t.Fatalf("old color still present: %s", s)
	}
	if !strings.Contains(s, "color:#42b883") {
		t.Fatalf("new color missing: %s", s)
	}
	// Other directives preserved.
	if !strings.Contains(s, "window:dev") || !strings.Contains(s, "pane:nvim .") {
		t.Fatalf("non-color directives clobbered: %s", s)
	}
}

func TestSetLocalDirectiveAppendsWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent-tmux.conf")
	initial := "window:dev\npane:nvim .\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetLocalDirective(path, "color", "red"); err != nil {
		t.Fatalf("SetLocalDirective: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "color:red") {
		t.Fatalf("color not appended: %s", s)
	}
	if !strings.Contains(s, "window:dev") {
		t.Fatalf("existing directives lost: %s", s)
	}
}

func TestSetLocalDirectiveIgnoresCommentedDirective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent-tmux.conf")
	initial := "# color:example\nwindow:dev\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetLocalDirective(path, "color", "#42b883"); err != nil {
		t.Fatalf("SetLocalDirective: %v", err)
	}
	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, "# color:example") {
		t.Fatalf("comment line was modified: %s", s)
	}
	if !strings.Contains(s, "color:#42b883") {
		t.Fatalf("new color not added: %s", s)
	}
}

func TestRemoveLocalDirective(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agent-tmux.conf")
	initial := "window:dev\ncolor:#42b883\npane:nvim .\n"
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := RemoveLocalDirective(path, "color"); err != nil {
		t.Fatalf("RemoveLocalDirective: %v", err)
	}
	body, _ := os.ReadFile(path)
	s := string(body)
	if strings.Contains(s, "color:") {
		t.Fatalf("color directive still present: %s", s)
	}
	if !strings.Contains(s, "window:dev") || !strings.Contains(s, "pane:nvim .") {
		t.Fatalf("other directives lost: %s", s)
	}
}

func TestRemoveLocalDirectiveMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.conf")
	if err := RemoveLocalDirective(path, "color"); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}
