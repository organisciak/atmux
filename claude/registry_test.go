package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseRegistryEntry(t *testing.T) {
	p, ok := parseRegistryEntry([]byte(`{"pid":36468,"sessionId":"61292fad","cwd":"/p","kind":"interactive",
		"tmux":"agent-3d-print:@117.%121","name":"3d-print-9e","status":"waiting",
		"waitingFor":"permission prompt","statusUpdatedAt":1790351231157}`))
	if !ok {
		t.Fatal("entry rejected")
	}
	if p.TmuxPane != "%121" || p.Status != StatusWaiting || p.WaitingFor != "permission prompt" {
		t.Fatalf("unexpected process %+v", p)
	}
	if !p.Since.Equal(time.UnixMilli(1790351231157)) {
		t.Fatalf("since = %v", p.Since)
	}
}

func TestParseRegistryEntrySkipsBackgroundJobsAndJunk(t *testing.T) {
	for _, data := range []string{
		`{"pid":1,"kind":"bg"}`,
		`{"kind":"interactive"}`,
		`not json`,
	} {
		if _, ok := parseRegistryEntry([]byte(data)); ok {
			t.Errorf("accepted %s", data)
		}
	}
}

func TestLiveProcessesDropsDeadPIDs(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("10.json", `{"pid":10,"status":"idle"}`)
	write("20.json", `{"pid":20,"status":"busy"}`)
	write("20.abc.key", `ignored`)

	procs := liveProcessesIn(dir, func(pid int) bool { return pid == 20 })
	if len(procs) != 1 || procs[0].PID != 20 || procs[0].Status != StatusBusy {
		t.Fatalf("procs = %+v", procs)
	}
}

func TestPaneFromTmuxField(t *testing.T) {
	cases := map[string]string{
		"agent-3d-print:@117.%121": "%121",
		"":                         "",
		"weird":                    "",
	}
	for in, want := range cases {
		if got := paneFromTmuxField(in); got != want {
			t.Errorf("paneFromTmuxField(%q) = %q, want %q", in, got, want)
		}
	}
}
