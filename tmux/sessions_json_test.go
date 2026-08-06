package tmux

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildSessionsPayload_NoSessionsEmitsEmptyArray(t *testing.T) {
	payload := BuildSessionsPayload(nil, "test", time.Now(), false)

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// A host with no tmux server must produce [] rather than null, so readers
	// can tell "no sessions" from "field missing" without special-casing.
	if !strings.Contains(string(encoded), `"sessions":[]`) {
		t.Fatalf("expected an empty sessions array, got %s", encoded)
	}
}

func TestBuildSessionsPayload_CarriesSessionFields(t *testing.T) {
	lines := []SessionLine{{
		Name:       "agent-foo",
		Line:       "agent-foo: 2 windows (created Fri Jan 30 10:00:00 2026) (attached)",
		Activity:   1735000000,
		WorkingDir: "/home/me/foo",
		Color:      "#ff0000",
		Attached:   true,
	}}

	payload := BuildSessionsPayload(lines, "test", time.Now(), false)
	if len(payload.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(payload.Sessions))
	}

	s := payload.Sessions[0]
	if s.Name != "agent-foo" {
		t.Errorf("name = %q", s.Name)
	}
	if !s.Attached {
		t.Error("expected attached to survive into the payload")
	}
	if s.WorkingDir != "/home/me/foo" {
		t.Errorf("working dir = %q", s.WorkingDir)
	}
	if s.Activity != 1735000000 {
		t.Errorf("activity = %d", s.Activity)
	}
	if s.Color != "#ff0000" {
		t.Errorf("color = %q", s.Color)
	}
}

func TestBuildSessionsPayload_OmitsBeadsWhenNotRequested(t *testing.T) {
	lines := []SessionLine{{Name: "agent-foo", WorkingDir: t.TempDir()}}

	payload := BuildSessionsPayload(lines, "test", time.Now(), false)
	if payload.Sessions[0].BeadsOpen != nil {
		t.Fatal("beads must be skipped when not requested; counting is slow enough to matter over SSH")
	}
}

func TestBuildSessionsPayload_OmitsBeadsForNonBeadsProject(t *testing.T) {
	// A directory with no .beads is not a beads project; that must stay
	// distinguishable from a project with zero open issues.
	lines := []SessionLine{{Name: "agent-foo", WorkingDir: t.TempDir()}}

	payload := BuildSessionsPayload(lines, "test", time.Now(), true)
	if payload.Sessions[0].BeadsOpen != nil {
		t.Fatalf("expected no beads count, got %d", *payload.Sessions[0].BeadsOpen)
	}
}

func TestBuildSessionsPayload_StampsSchemaAndVersion(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	payload := BuildSessionsPayload(nil, "v1.2.3", now, false)

	if payload.SchemaVersion != SessionsSchemaVersion {
		t.Errorf("schema version = %d, want %d", payload.SchemaVersion, SessionsSchemaVersion)
	}
	if payload.AtmuxVersion != "v1.2.3" {
		t.Errorf("atmux version = %q, want %q", payload.AtmuxVersion, "v1.2.3")
	}
	if !payload.GeneratedAt.Equal(now) {
		t.Errorf("generated_at = %v, want %v", payload.GeneratedAt, now)
	}
}

func TestSessionsPayload_RoundTrips(t *testing.T) {
	count := 3
	original := SessionsPayload{
		SchemaVersion: SessionsSchemaVersion,
		AtmuxVersion:  "v1.2.3",
		GeneratedAt:   time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC),
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

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded SessionsPayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(decoded.Sessions) != 1 {
		t.Fatalf("expected 1 session after round trip, got %d", len(decoded.Sessions))
	}
	got := decoded.Sessions[0]
	if got.Name != "agent-foo" || !got.Attached || got.Activity != 1735000000 {
		t.Fatalf("session did not survive the round trip: %+v", got)
	}
	if got.BeadsOpen == nil || *got.BeadsOpen != 3 {
		t.Fatalf("beads count did not survive the round trip: %v", got.BeadsOpen)
	}
}

func TestBuildSessionsPayload_ZeroBeadsIsDistinctFromAbsent(t *testing.T) {
	// The pointer exists precisely so a beads project with nothing open renders
	// as "0" while a non-beads project renders as nothing at all.
	zero := 0
	withZero := SessionJSON{Name: "a", BeadsOpen: &zero}
	withNone := SessionJSON{Name: "b"}

	encodedZero, _ := json.Marshal(withZero)
	encodedNone, _ := json.Marshal(withNone)

	if !strings.Contains(string(encodedZero), `"beads_open":0`) {
		t.Fatalf("zero count must be emitted, got %s", encodedZero)
	}
	if strings.Contains(string(encodedNone), "beads_open") {
		t.Fatalf("absent count must be omitted, got %s", encodedNone)
	}
}
