package tmux

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlSocketName_StableAndShort(t *testing.T) {
	a := controlSocketName("user@devbox", 22)
	b := controlSocketName("user@devbox", 22)
	if a != b {
		t.Fatalf("controlSocketName is not stable: %q vs %q", a, b)
	}
	if !strings.HasSuffix(a, ".sock") {
		t.Fatalf("expected .sock suffix, got %q", a)
	}
	if len(a) != 17 { // 12 hex chars + ".sock"
		t.Fatalf("expected fixed-width name, got %q (len %d)", a, len(a))
	}
}

func TestControlSocketName_DistinguishesHostAndPort(t *testing.T) {
	base := controlSocketName("user@devbox", 22)
	if other := controlSocketName("user@devbox", 2222); other == base {
		t.Fatal("expected different socket names for different ports")
	}
	if other := controlSocketName("user@other", 22); other == base {
		t.Fatal("expected different socket names for different hosts")
	}
}

func TestControlSocketName_LongHostStaysShort(t *testing.T) {
	long := strings.Repeat("a", 300) + "@" + strings.Repeat("b", 300)
	if got := controlSocketName(long, 22); len(got) != 17 {
		t.Fatalf("long host produced long socket name %q (len %d)", got, len(got))
	}
}

// shortTempDir returns a temp directory under /tmp. The default t.TempDir() on
// macOS lives under /var/folders/... and is long enough on its own to trip the
// sun_path fallback, which would mask what these tests are checking.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "atmux-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestControlSocketPath_CreatesPrivateDir(t *testing.T) {
	cache := shortTempDir(t)
	t.Setenv("XDG_CACHE_HOME", cache)

	path, err := controlSocketPath("user@devbox", 22)
	if err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}

	wantDir := filepath.Join(cache, "atmux", "ssh")
	if got := filepath.Dir(path); got != wantDir {
		t.Fatalf("expected dir %q, got %q", wantDir, got)
	}

	info, err := os.Stat(wantDir)
	if err != nil {
		t.Fatalf("socket dir not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Fatalf("expected socket dir mode 0700, got %o", perm)
	}
}

func TestControlSocketPath_TightensLoosePermissions(t *testing.T) {
	cache := shortTempDir(t)
	t.Setenv("XDG_CACHE_HOME", cache)

	// A pre-existing world-writable directory would let anyone hijack the
	// authenticated control socket.
	dir := filepath.Join(cache, "atmux", "ssh")
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if _, err := controlSocketPath("user@devbox", 22); err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Fatalf("expected permissions tightened to 0700, got %o", perm)
	}
}

func TestControlSocketPath_StableAcrossCalls(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", shortTempDir(t))

	first, err := controlSocketPath("user@devbox", 22)
	if err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}
	second, err := controlSocketPath("user@devbox", 22)
	if err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}
	if first != second {
		t.Fatalf("path is not stable across calls: %q vs %q", first, second)
	}
}

func TestControlSocketPath_FallsBackWhenPathTooLong(t *testing.T) {
	// A cache dir this deep would overflow sun_path; the /tmp fallback must
	// take over rather than returning an unusable path.
	deep := filepath.Join(shortTempDir(t), strings.Repeat("nested/", 20))
	t.Setenv("XDG_CACHE_HOME", deep)

	path, err := controlSocketPath("user@devbox", 22)
	if err != nil {
		t.Fatalf("controlSocketPath: %v", err)
	}
	if len(path) > maxSocketPathLen {
		t.Fatalf("fallback path still too long: %q (len %d)", path, len(path))
	}
	if !strings.HasPrefix(path, "/tmp/atmux-") {
		t.Fatalf("expected /tmp fallback, got %q", path)
	}
}

func TestControlPersist_DefaultAndOverride(t *testing.T) {
	t.Setenv(controlPersistEnv, "")
	if got := controlPersist(); got != defaultControlPersist {
		t.Fatalf("expected default %q, got %q", defaultControlPersist, got)
	}

	t.Setenv(controlPersistEnv, "30m")
	if got := controlPersist(); got != "30m" {
		t.Fatalf("expected override '30m', got %q", got)
	}
}
