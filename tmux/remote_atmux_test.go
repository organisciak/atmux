package tmux

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestRemoteAtmuxArgs_Direct(t *testing.T) {
	command, args := remoteAtmuxArgs(remoteAtmuxDirect, "sessions", "--json")
	if command != "atmux" {
		t.Fatalf("command = %q, want atmux", command)
	}
	if !reflect.DeepEqual(args, []string{"sessions", "--json"}) {
		t.Fatalf("args = %v", args)
	}
}

func TestRemoteAtmuxArgs_LoginShell(t *testing.T) {
	// SSH runs a non-login shell, which on many setups skips the profile that
	// puts atmux on PATH. The fallback must send one shell-quotable string.
	command, args := remoteAtmuxArgs(remoteAtmuxLoginShell, "sessions", "--json")
	if command != loginShellFallback {
		t.Fatalf("command = %q, want %q", command, loginShellFallback)
	}
	want := []string{"-lc", "atmux sessions --json"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestParseSessionsPayload_TagsHostAndCarriesMetadata(t *testing.T) {
	count := 7
	payload := SessionsPayload{
		SchemaVersion: SessionsSchemaVersion,
		AtmuxVersion:  "v1.0.0",
		GeneratedAt:   time.Now(),
		Sessions: []SessionJSON{{
			Name:       "agent-foo",
			Display:    "agent-foo: 2 windows",
			Attached:   true,
			WorkingDir: "/home/me/foo",
			Activity:   1735000000,
			Color:      "#ff0000",
			BeadsOpen:  &count,
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	lines, err := ParseSessionsPayload(data, "devbox")
	if err != nil {
		t.Fatalf("ParseSessionsPayload: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	got := lines[0]
	if got.Host != "devbox" {
		t.Errorf("host = %q, want devbox", got.Host)
	}
	// The point of the whole feature: remote rows carry what plain
	// `tmux list-sessions` cannot.
	if got.WorkingDir != "/home/me/foo" {
		t.Errorf("working dir = %q", got.WorkingDir)
	}
	if got.Color != "#ff0000" {
		t.Errorf("color = %q", got.Color)
	}
	if got.Beads == nil || *got.Beads != 7 {
		t.Errorf("beads = %v, want 7", got.Beads)
	}
	if !got.Attached {
		t.Error("expected attached")
	}
}

func TestParseSessionsPayload_RejectsNewerSchema(t *testing.T) {
	data, err := json.Marshal(SessionsPayload{SchemaVersion: SessionsSchemaVersion + 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// A payload we cannot read must be refused so the caller falls back to
	// plain tmux rather than guessing at unknown fields.
	if _, err := ParseSessionsPayload(data, "devbox"); err == nil {
		t.Fatal("expected an error for a newer schema version")
	}
}

func TestParseSessionsPayload_AcceptsOlderSchema(t *testing.T) {
	data, err := json.Marshal(SessionsPayload{SchemaVersion: 1})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := ParseSessionsPayload(data, "devbox"); err != nil {
		t.Fatalf("older schema must still parse: %v", err)
	}
}

func TestParseSessionsPayload_RejectsGarbage(t *testing.T) {
	// A host that printed a shell warning before the JSON, or no atmux at all.
	if _, err := ParseSessionsPayload([]byte("bash: atmux: command not found"), "devbox"); err == nil {
		t.Fatal("expected an error for non-JSON output")
	}
}

func TestParseSessionsPayload_EmptyIsNotAnError(t *testing.T) {
	data, err := json.Marshal(SessionsPayload{SchemaVersion: SessionsSchemaVersion})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	lines, err := ParseSessionsPayload(data, "devbox")
	if err != nil {
		t.Fatalf("ParseSessionsPayload: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no lines, got %d", len(lines))
	}
}

func TestRemoteAtmuxMode_CachedAcrossCalls(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	if e.atmuxMode() != remoteAtmuxUnknown {
		t.Fatal("a fresh executor should be unprobed")
	}

	e.setAtmuxMode(remoteAtmuxAbsent)
	if e.atmuxMode() != remoteAtmuxAbsent {
		t.Fatal("mode should persist so the probe runs once per host")
	}
	if e.RemoteAtmuxAvailable() {
		t.Fatal("an absent atmux must not report as available")
	}

	e.setAtmuxMode(remoteAtmuxLoginShell)
	if !e.RemoteAtmuxAvailable() {
		t.Fatal("a login-shell atmux is still available")
	}
}

func TestSessionsViaAtmux_SkipsWorkWhenKnownAbsent(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	e.setAtmuxMode(remoteAtmuxAbsent)

	// Must short-circuit without dialing: a host known to lack atmux should
	// not pay a round trip on every refresh.
	start := time.Now()
	lines, ok := e.sessionsViaAtmux()
	if ok || lines != nil {
		t.Fatal("expected no result for a host known to lack atmux")
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("expected an immediate return, took %v", elapsed)
	}
}

func TestSessionsPayload_RoundTripsRealLocalData(t *testing.T) {
	// Exercises the actual wire format end to end: real tmux output through the
	// producer and back out of the consumer, as a remote host would.
	lines, err := ListSessionsRawWithExecutor(NewLocalExecutor())
	if err != nil {
		t.Skipf("no local tmux available: %v", err)
	}
	if len(lines) == 0 {
		t.Skip("no local sessions to round trip")
	}

	data, err := json.Marshal(BuildSessionsPayload(lines, "test", time.Now(), false))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	decoded, err := ParseSessionsPayload(data, "devbox")
	if err != nil {
		t.Fatalf("ParseSessionsPayload: %v", err)
	}
	if len(decoded) != len(lines) {
		t.Fatalf("round trip changed session count: %d -> %d", len(lines), len(decoded))
	}

	for i := range lines {
		if decoded[i].Name != lines[i].Name {
			t.Errorf("session %d: name %q -> %q", i, lines[i].Name, decoded[i].Name)
		}
		if decoded[i].WorkingDir != lines[i].WorkingDir {
			t.Errorf("session %d: working dir %q -> %q", i, lines[i].WorkingDir, decoded[i].WorkingDir)
		}
		if decoded[i].Attached != lines[i].Attached {
			t.Errorf("session %d: attached %v -> %v", i, lines[i].Attached, decoded[i].Attached)
		}
		if decoded[i].Activity != lines[i].Activity {
			t.Errorf("session %d: activity %d -> %d", i, lines[i].Activity, decoded[i].Activity)
		}
	}
}
