package tmux

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildSSHInteractiveArgs_DefaultPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	got := e.buildSSHInteractiveArgs("attach-session", "-t", "mysess")
	want := []string{"-t", "-p", "22", "user@devbox", `tmux 'attach-session' '-t' 'mysess'`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildSSHInteractiveArgs_CustomPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 2222, "ssh", "devbox")
	got := e.buildSSHInteractiveArgs("attach-session", "-t", "work")
	want := []string{"-t", "-p", "2222", "user@devbox", `tmux 'attach-session' '-t' 'work'`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildSSHInteractiveArgs_NoTmuxArgs(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "ssh", "")
	got := e.buildSSHInteractiveArgs()
	want := []string{"-t", "-p", "22", "host", "tmux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildSSHInteractiveArgs_ReusesControlSocket(t *testing.T) {
	t.Setenv(controlPersistEnv, "4h")
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	e.controlPath = "/tmp/atmux-1/ssh/abc.sock"

	got := e.buildSSHInteractiveArgs("attach-session", "-t", "work")
	want := []string{
		"-t",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/tmp/atmux-1/ssh/abc.sock",
		"-o", "ControlPersist=4h",
		"-p", "22", "user@devbox", `tmux 'attach-session' '-t' 'work'`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSSHInteractiveArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestSSHArgs_IncludeControlPathWhenResolved(t *testing.T) {
	t.Setenv(controlPersistEnv, "4h")
	e := NewRemoteExecutor("user@devbox", 2222, "ssh", "devbox")

	// Before the socket is resolved there is nothing to point ssh at.
	if got := strings.Join(e.sshArgs(), " "); strings.Contains(got, "ControlPath") {
		t.Fatalf("expected no ControlPath before resolution, got %q", got)
	}

	e.controlPath = "/tmp/atmux-1/ssh/abc.sock"
	got := strings.Join(e.sshArgs(), " ")
	if !strings.Contains(got, "ControlPath=/tmp/atmux-1/ssh/abc.sock") {
		t.Fatalf("expected ControlPath in ssh args, got %q", got)
	}
	if !strings.Contains(got, "ControlPersist=4h") {
		t.Fatalf("expected ControlPersist in ssh args, got %q", got)
	}
}

func TestControlMasterAlive_NoSocket(t *testing.T) {
	if controlMasterAlive("", "host", 22) {
		t.Fatal("expected empty path to report not alive")
	}
	if controlMasterAlive("/tmp/atmux-does-not-exist.sock", "host", 22) {
		t.Fatal("expected missing socket to report not alive")
	}
}

func TestRemoveStaleSocket_IgnoresRegularFilesAndMissingPaths(t *testing.T) {
	// Must not panic or delete non-socket files that happen to share the path.
	removeStaleSocket("")
	removeStaleSocket("/tmp/atmux-does-not-exist.sock")

	dir := shortTempDir(t)
	regular := filepath.Join(dir, "not-a-socket")
	if err := os.WriteFile(regular, []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	removeStaleSocket(regular)
	if _, err := os.Stat(regular); err != nil {
		t.Fatalf("regular file should not be removed: %v", err)
	}
}

func TestDisconnect_NoSocketIsNoOp(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", shortTempDir(t))
	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	if err := e.Disconnect(); err != nil {
		t.Fatalf("Disconnect with no socket should succeed, got %v", err)
	}
}

func TestClose_LeavesControlSocketIntact(t *testing.T) {
	dir := shortTempDir(t)
	t.Setenv("XDG_CACHE_HOME", dir)

	e := NewRemoteExecutor("user@devbox", 22, "ssh", "devbox")
	path, err := controlSocketPath(e.Host, e.Port)
	if err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}
	e.controlPath = path

	// Stand in for a live master so we can assert Close does not remove it.
	if err := os.WriteFile(path, []byte("placeholder"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Close must leave the shared socket in place, got %v", err)
	}
}

func TestBuildMoshArgs_DefaultPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 22, "mosh", "devbox")
	got := e.buildMoshArgs("attach-session", "-t", "mysess")
	want := []string{"user@devbox", "--", "tmux", "attach-session", "-t", "mysess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildMoshArgs_CustomPort(t *testing.T) {
	e := NewRemoteExecutor("user@devbox", 2222, "mosh", "devbox")
	got := e.buildMoshArgs("attach-session", "-t", "mysess")
	want := []string{"--ssh=ssh -p 2222", "user@devbox", "--", "tmux", "attach-session", "-t", "mysess"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildMoshArgs_NoTmuxArgs(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "mosh", "")
	got := e.buildMoshArgs()
	want := []string{"host", "--", "tmux"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestNewRemoteExecutor_Defaults(t *testing.T) {
	e := NewRemoteExecutor("myhost", 0, "", "")
	if e.Port != defaultSSHPort {
		t.Fatalf("expected default port %d, got %d", defaultSSHPort, e.Port)
	}
	if e.AttachMethod != "ssh" {
		t.Fatalf("expected default attach method 'ssh', got %q", e.AttachMethod)
	}
	if e.Alias != "myhost" {
		t.Fatalf("expected alias 'myhost', got %q", e.Alias)
	}
}

func TestNewRemoteExecutor_CustomValues(t *testing.T) {
	e := NewRemoteExecutor("user@box", 2222, "mosh", "devbox")
	if e.Port != 2222 {
		t.Fatalf("expected port 2222, got %d", e.Port)
	}
	if e.AttachMethod != "mosh" {
		t.Fatalf("expected attach method 'mosh', got %q", e.AttachMethod)
	}
	if e.Alias != "devbox" {
		t.Fatalf("expected alias 'devbox', got %q", e.Alias)
	}
}

func TestMoshAvailable(t *testing.T) {
	// This test verifies the function runs without error.
	// The result depends on the test environment (mosh may or may not be installed).
	_ = moshAvailable()
}

func TestInteractiveRouting_SSHMethod(t *testing.T) {
	// Verify that with attach_method=ssh, Interactive calls through the SSH path.
	// We test this indirectly by checking buildSSHInteractiveArgs is producing correct output.
	e := NewRemoteExecutor("user@host", 22, "ssh", "")
	if e.AttachMethod != "ssh" {
		t.Fatalf("expected ssh attach method, got %q", e.AttachMethod)
	}
	args := e.buildSSHInteractiveArgs("attach-session", "-t", "test")
	if args[0] != "-t" {
		t.Fatalf("expected first arg '-t' for SSH, got %q", args[0])
	}
}

func TestInteractiveRouting_MoshMethod(t *testing.T) {
	// Verify that with attach_method=mosh, the mosh args path is used.
	e := NewRemoteExecutor("user@host", 22, "mosh", "")
	if e.AttachMethod != "mosh" {
		t.Fatalf("expected mosh attach method, got %q", e.AttachMethod)
	}
	args := e.buildMoshArgs("attach-session", "-t", "test")
	if args[0] != "user@host" {
		t.Fatalf("expected first mosh arg to be host, got %q", args[0])
	}
	if args[1] != "--" {
		t.Fatalf("expected second mosh arg '--', got %q", args[1])
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", "'simple'"},
		{"with space", "'with space'"},
		{"it's", "'it'\\''s'"},
		{"#{session_name}: #{session_windows} windows", "'#{session_name}: #{session_windows} windows'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.input); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRemoteCommand(t *testing.T) {
	got := remoteCommand("tmux", []string{"list-sessions", "-F", "#{session_name}: #{session_windows} windows"})
	want := "tmux 'list-sessions' '-F' '#{session_name}: #{session_windows} windows'"
	if got != want {
		t.Errorf("remoteCommand mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestHostLabel(t *testing.T) {
	e := NewRemoteExecutor("user@host", 22, "ssh", "my-alias")
	if got := e.HostLabel(); got != "my-alias" {
		t.Fatalf("expected HostLabel 'my-alias', got %q", got)
	}
}

func TestIsRemote(t *testing.T) {
	e := NewRemoteExecutor("host", 22, "ssh", "")
	if !e.IsRemote() {
		t.Fatal("expected IsRemote() to be true")
	}
}

// A host whose non-login PATH lacks tmux gets its attach wrapped in `bash -lc`.
// That wrapper must reach ssh as ONE argument: ssh joins everything after the
// host with spaces before handing it to the remote shell, so the split form
// {"bash", "-lc", "<cmd>"} arrives as `bash -lc tmux 'attach-session' ...`,
// where bash -c takes only `tmux` as its script and binds the rest to $0/$1/$2.
// The result is a bare `tmux`, which creates a new numeric session instead of
// attaching to the requested one.
func TestBuildSSHInteractiveArgs_LoginShellStaysOneArgument(t *testing.T) {
	e := NewRemoteExecutor("user@vps", 22, "ssh", "vps")
	e.setShellMode(remoteShellLogin)

	got := e.buildSSHInteractiveArgs("attach-session", "-t", "9")

	hostIdx := -1
	for i, a := range got {
		if a == "user@vps" {
			hostIdx = i
		}
	}
	if hostIdx == -1 {
		t.Fatalf("host missing from args: %v", got)
	}

	remote := got[hostIdx+1:]
	if len(remote) != 1 {
		t.Fatalf("remote command must be a single ssh argument, got %d: %v", len(remote), remote)
	}
	if !strings.HasPrefix(remote[0], loginShellFallback+" -lc ") {
		t.Fatalf("expected a login-shell wrapper, got %q", remote[0])
	}

	script := strings.TrimPrefix(remote[0], loginShellFallback+" -lc ")
	if !strings.HasPrefix(script, "'") || !strings.HasSuffix(script, "'") {
		t.Fatalf("script must be quoted as one shell word, got %q", script)
	}
	if !strings.Contains(script, "attach-session") || !strings.Contains(script, "9") {
		t.Fatalf("script lost its tmux arguments: %q", script)
	}
}

// mosh preserves argv boundaries (it shell-quotes each element when building the
// mosh-server invocation, which then execs the vector directly), so its
// login-shell form stays split — the opposite of the ssh case above.
func TestBuildMoshArgs_LoginShellKeepsSeparateArgv(t *testing.T) {
	e := NewRemoteExecutor("user@vps", 22, "mosh", "vps")
	e.setShellMode(remoteShellLogin)

	got := e.buildMoshArgs("attach-session", "-t", "9")

	want := []string{"user@vps", "--", loginShellFallback, "-lc",
		remoteCommand("tmux", []string{"attach-session", "-t", "9"})}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMoshArgs mismatch\n got: %v\nwant: %v", got, want)
	}
}
