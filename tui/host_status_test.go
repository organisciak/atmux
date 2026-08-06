package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/tmux"
)

// stubHostExecutor stands in for a remote host with a fixed reachability state.
type stubHostExecutor struct {
	label string
	state tmux.HostState
}

func (s *stubHostExecutor) Run(args ...string) error                 { return nil }
func (s *stubHostExecutor) Output(args ...string) ([]byte, error)    { return nil, nil }
func (s *stubHostExecutor) RunWithDir(dir string, a ...string) error { return nil }
func (s *stubHostExecutor) Interactive(args ...string) error         { return nil }
func (s *stubHostExecutor) RunGeneric(c string, a ...string) ([]byte, error) {
	return nil, nil
}
func (s *stubHostExecutor) HostLabel() string         { return s.label }
func (s *stubHostExecutor) IsRemote() bool            { return s.label != "" }
func (s *stubHostExecutor) HostState() tmux.HostState { return s.state }
func (s *stubHostExecutor) ResetBackoff()             {}
func (s *stubHostExecutor) Close() error              { return nil }

func modelWithHosts(execs ...tmux.TmuxExecutor) sessionsModel {
	return newSessionsModel(execs, false, true)
}

func TestHostStatusLine_ConnectingBeforeFetchCompletes(t *testing.T) {
	exec := &stubHostExecutor{label: "devbox"}
	m := modelWithHosts(exec)

	if got := m.hostStatusLine(exec); !strings.Contains(got, "Connecting") {
		t.Fatalf("expected a connecting notice before the fetch lands, got %q", got)
	}
}

func TestHostStatusLine_HealthyHostWithNoSessions(t *testing.T) {
	exec := &stubHostExecutor{label: "devbox", state: tmux.HostState{Status: tmux.HostOK}}
	m := modelWithHosts(exec)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true}}

	if got := m.hostStatusLine(exec); got != "No active sessions" {
		t.Fatalf("expected a healthy empty host to say so, got %q", got)
	}
}

func TestHostStatusLine_UnreachableShowsReason(t *testing.T) {
	exec := &stubHostExecutor{label: "devbox", state: tmux.HostState{
		Status: tmux.HostUnreachable,
		Err:    errors.New("connection timed out"),
	}}
	m := modelWithHosts(exec)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true, err: errors.New("boom")}}

	got := m.hostStatusLine(exec)
	if !strings.Contains(got, "unreachable") {
		t.Fatalf("expected an unreachable reason, got %q", got)
	}
}

func TestHostStatusLine_TmuxMissingIsDistinctFromUnreachable(t *testing.T) {
	missing := &stubHostExecutor{label: "a", state: tmux.HostState{Status: tmux.HostTmuxMissing}}
	down := &stubHostExecutor{label: "b", state: tmux.HostState{Status: tmux.HostUnreachable}}
	m := modelWithHosts(missing, down)
	m.hostFetch = map[string]hostFetchState{
		"a": {done: true, err: errors.New("x")},
		"b": {done: true, err: errors.New("x")},
	}

	if m.hostStatusLine(missing) == m.hostStatusLine(down) {
		t.Fatal("a host without tmux must not read the same as one that is unreachable")
	}
}

func TestHostStatusLine_OmitsLastSeenForReachableHost(t *testing.T) {
	// A host that answers SSH but has no tmux was seen moments ago; saying so
	// next to the real reason is noise.
	exec := &stubHostExecutor{label: "devbox", state: tmux.HostState{
		Status: tmux.HostTmuxMissing,
		LastOK: time.Now(),
	}}
	m := modelWithHosts(exec)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true, err: errors.New("127")}}

	if got := m.hostStatusLine(exec); strings.Contains(got, "last seen") {
		t.Fatalf("expected no last-seen for a reachable host, got %q", got)
	}
}

func TestHostStatusLine_IncludesLastSeenWhenKnown(t *testing.T) {
	exec := &stubHostExecutor{label: "devbox", state: tmux.HostState{
		Status: tmux.HostUnreachable,
		LastOK: time.Now().Add(-2 * time.Hour),
	}}
	m := modelWithHosts(exec)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true, err: errors.New("down")}}

	if got := m.hostStatusLine(exec); !strings.Contains(got, "last seen") {
		t.Fatalf("expected a last-seen hint for a host that used to work, got %q", got)
	}
}

func TestSilentHostSections_KeepsMissingHostVisible(t *testing.T) {
	local := &stubHostExecutor{}
	remote := &stubHostExecutor{label: "devbox", state: tmux.HostState{Status: tmux.HostUnreachable}}
	m := modelWithHosts(local, remote)
	m.hostFetch = map[string]hostFetchState{"": {done: true}, "devbox": {done: true, err: errors.New("down")}}

	shown := []tmux.SessionLine{{Name: "agent-local"}}
	sections := m.silentHostSections(shown, lipgloss.NewStyle())

	joined := strings.Join(sections, "\n")
	if !strings.Contains(joined, "devbox") {
		t.Fatalf("an unreachable host must stay visible, got %q", joined)
	}
	if !strings.Contains(joined, "unreachable") {
		t.Fatalf("expected the reason alongside the host, got %q", joined)
	}
}

func TestSilentHostSections_SkipsHostsThatReturnedRows(t *testing.T) {
	remote := &stubHostExecutor{label: "devbox", state: tmux.HostState{Status: tmux.HostOK}}
	m := modelWithHosts(remote)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true}}

	shown := []tmux.SessionLine{{Name: "agent-foo", Host: "devbox"}}
	if sections := m.silentHostSections(shown, lipgloss.NewStyle()); len(sections) != 0 {
		t.Fatalf("a host with rows needs no placeholder, got %v", sections)
	}
}

func TestSilentHostSections_SkipsLocal(t *testing.T) {
	// The local host is never "unreachable"; a placeholder for it would be noise.
	local := &stubHostExecutor{}
	m := modelWithHosts(local)
	m.hostFetch = map[string]hostFetchState{"": {done: true}}

	if sections := m.silentHostSections(nil, lipgloss.NewStyle()); len(sections) != 0 {
		t.Fatalf("expected no placeholder for local, got %v", sections)
	}
}

func TestSilentHostSections_QuietWhileSearching(t *testing.T) {
	remote := &stubHostExecutor{label: "devbox", state: tmux.HostState{Status: tmux.HostUnreachable}}
	m := modelWithHosts(remote)
	m.hostFetch = map[string]hostFetchState{"devbox": {done: true, err: errors.New("down")}}
	m.searchActive = true

	if sections := m.silentHostSections(nil, lipgloss.NewStyle()); len(sections) != 0 {
		t.Fatalf("search deliberately narrows the list; expected no notices, got %v", sections)
	}
}

func TestHostFailureReason_PrefersClassificationOverRawError(t *testing.T) {
	ht := tmux.HostTree{
		Host:     "devbox",
		Err:      errors.New("exit status 127"),
		Executor: &stubHostExecutor{label: "devbox", state: tmux.HostState{Status: tmux.HostTmuxMissing}},
	}

	got := hostFailureReason(ht)
	if !strings.Contains(got, "tmux not installed") {
		t.Fatalf("expected the classified reason, got %q", got)
	}
	if !strings.Contains(got, "exit status 127") {
		t.Fatalf("expected the raw error kept as detail, got %q", got)
	}
}

func TestHostFailureReason_FallsBackWithoutExecutor(t *testing.T) {
	ht := tmux.HostTree{Host: "devbox", Err: errors.New("dial timeout")}
	if got := hostFailureReason(ht); !strings.Contains(got, "unreachable") {
		t.Fatalf("expected a fallback reason, got %q", got)
	}
}
