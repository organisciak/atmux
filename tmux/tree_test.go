package tmux

import (
	"errors"
	"strings"
	"testing"
)

// fakeExecutor returns canned output based on the tmux subcommand.
type fakeExecutor struct {
	host      string
	remote    bool
	responses map[string]fakeResponse // key = first arg (e.g. "list-sessions")
}

type fakeResponse struct {
	output []byte
	err    error
}

func (f *fakeExecutor) Run(args ...string) error {
	if len(args) > 0 {
		if r, ok := f.responses[args[0]]; ok {
			return r.err
		}
	}
	return nil
}

func (f *fakeExecutor) Output(args ...string) ([]byte, error) {
	if len(args) > 0 {
		if r, ok := f.responses[args[0]]; ok {
			return r.output, r.err
		}
	}
	return nil, nil
}

func (f *fakeExecutor) RunWithDir(dir string, args ...string) error { return nil }
func (f *fakeExecutor) Interactive(args ...string) error            { return nil }
func (f *fakeExecutor) RunGeneric(cmd string, args ...string) ([]byte, error) {
	return nil, nil
}
func (f *fakeExecutor) HostLabel() string { return f.host }
func (f *fakeExecutor) IsRemote() bool    { return f.remote }
func (f *fakeExecutor) Close() error      { return nil }

func TestFetchTreeWithExecutors_LocalOnly(t *testing.T) {
	local := &fakeExecutor{
		host:   "",
		remote: false,
		responses: map[string]fakeResponse{
			"list-sessions": {
				output: []byte("mysession:0\n"),
			},
			"list-windows": {
				output: []byte("@1:0:bash:1\n"),
			},
			"list-panes": {
				output: []byte("%1:0:title:bash:1:80:24\n"),
			},
		},
	}

	results := FetchTreeWithExecutors([]TmuxExecutor{local})

	if len(results) != 1 {
		t.Fatalf("expected 1 host tree, got %d", len(results))
	}
	ht := results[0]
	if ht.Err != nil {
		t.Fatalf("unexpected error: %v", ht.Err)
	}
	if ht.Host != "" {
		t.Fatalf("expected empty host, got %q", ht.Host)
	}
	if ht.Tree == nil {
		t.Fatal("expected non-nil tree")
	}
	if len(ht.Tree.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(ht.Tree.Sessions))
	}
	if ht.Tree.Sessions[0].Name != "mysession" {
		t.Fatalf("expected session name 'mysession', got %q", ht.Tree.Sessions[0].Name)
	}
	if len(ht.Tree.Sessions[0].Windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(ht.Tree.Sessions[0].Windows))
	}
	if len(ht.Tree.Sessions[0].Windows[0].Panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(ht.Tree.Sessions[0].Windows[0].Panes))
	}
}

func TestFetchTreeWithExecutors_MultiHost(t *testing.T) {
	local := &fakeExecutor{
		host:   "",
		remote: false,
		responses: map[string]fakeResponse{
			"list-sessions": {output: []byte("local-sess:0\n")},
			"list-windows":  {output: []byte("@1:0:bash:1\n")},
			"list-panes":    {output: []byte("%1:0:title:bash:1:80:24\n")},
		},
	}
	remote := &fakeExecutor{
		host:   "devbox",
		remote: true,
		responses: map[string]fakeResponse{
			"list-sessions": {output: []byte("remote-sess:1\n")},
			"list-windows":  {output: []byte("@2:0:zsh:0\n")},
			"list-panes":    {output: []byte("%2:0:remote-title:zsh:1:120:40\n")},
		},
	}

	results := FetchTreeWithExecutors([]TmuxExecutor{local, remote})

	if len(results) != 2 {
		t.Fatalf("expected 2 host trees, got %d", len(results))
	}

	// Check local
	if results[0].Err != nil {
		t.Fatalf("local error: %v", results[0].Err)
	}
	if results[0].Tree == nil || len(results[0].Tree.Sessions) != 1 {
		t.Fatal("expected 1 local session")
	}
	if results[0].Tree.Sessions[0].Name != "local-sess" {
		t.Fatalf("expected 'local-sess', got %q", results[0].Tree.Sessions[0].Name)
	}

	// Check remote
	if results[1].Err != nil {
		t.Fatalf("remote error: %v", results[1].Err)
	}
	if results[1].Host != "devbox" {
		t.Fatalf("expected host 'devbox', got %q", results[1].Host)
	}
	if results[1].Tree == nil || len(results[1].Tree.Sessions) != 1 {
		t.Fatal("expected 1 remote session")
	}
	if results[1].Tree.Sessions[0].Name != "remote-sess" {
		t.Fatalf("expected 'remote-sess', got %q", results[1].Tree.Sessions[0].Name)
	}
}

func TestFetchTreeWithExecutors_RemoteFailureNonFatal(t *testing.T) {
	local := &fakeExecutor{
		host:   "",
		remote: false,
		responses: map[string]fakeResponse{
			"list-sessions": {output: []byte("ok-sess:0\n")},
			"list-windows":  {output: []byte("@1:0:bash:1\n")},
			"list-panes":    {output: []byte("%1:0:title:bash:1:80:24\n")},
		},
	}
	broken := &fakeExecutor{
		host:   "broken-host",
		remote: true,
		responses: map[string]fakeResponse{
			"list-sessions": {err: errors.New("connection refused")},
		},
	}

	results := FetchTreeWithExecutors([]TmuxExecutor{local, broken})

	if len(results) != 2 {
		t.Fatalf("expected 2 host trees, got %d", len(results))
	}

	// Local should succeed
	if results[0].Err != nil {
		t.Fatalf("local should succeed: %v", results[0].Err)
	}
	if results[0].Tree == nil || len(results[0].Tree.Sessions) != 1 {
		t.Fatal("expected 1 local session")
	}

	// Remote should fail gracefully
	if results[1].Err == nil {
		t.Fatal("expected error for broken remote")
	}
	if !strings.Contains(results[1].Err.Error(), "connection refused") {
		t.Fatalf("expected 'connection refused' error, got: %v", results[1].Err)
	}
	if results[1].Tree != nil {
		t.Fatal("expected nil tree for broken remote")
	}
}

func TestFetchTreeWithExecutors_NoServerRunning(t *testing.T) {
	// A remote with no tmux server should return an empty tree, not an error
	noServer := &fakeExecutor{
		host:   "empty-host",
		remote: true,
		responses: map[string]fakeResponse{
			"list-sessions": {
				output: nil,
				err:    errors.New("no server running on /tmp/tmux-501/default"),
			},
		},
	}

	results := FetchTreeWithExecutors([]TmuxExecutor{noServer})

	if len(results) != 1 {
		t.Fatalf("expected 1 host tree, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("no-server should not be an error: %v", results[0].Err)
	}
	if results[0].Tree == nil {
		t.Fatal("expected non-nil tree (empty)")
	}
	if len(results[0].Tree.Sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(results[0].Tree.Sessions))
	}
}

func TestCapturePaneWithExecutor(t *testing.T) {
	exec := &fakeExecutor{
		responses: map[string]fakeResponse{
			"capture-pane": {output: []byte("$ hello world\n$ _\n")},
		},
	}

	content, err := CapturePaneWithExecutor("mysess:0.0", exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "hello world") {
		t.Fatalf("expected 'hello world' in output, got %q", content)
	}
}

func TestCapturePaneWithExecutor_Error(t *testing.T) {
	exec := &fakeExecutor{
		responses: map[string]fakeResponse{
			"capture-pane": {err: errors.New("pane not found")},
		},
	}

	_, err := CapturePaneWithExecutor("bad:0.0", exec)
	if err == nil {
		t.Fatal("expected error")
	}
}
