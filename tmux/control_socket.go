package tmux

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// defaultControlPersist is how long an idle SSH ControlMaster stays alive
	// after its last client disconnects. Sockets are shared across atmux
	// invocations, so this value is what makes a second invocation cheap.
	defaultControlPersist = "4h"

	// controlPersistEnv overrides defaultControlPersist.
	controlPersistEnv = "ATMUX_SSH_CONTROL_PERSIST"

	// maxSocketPathLen keeps control socket paths inside sockaddr_un.sun_path.
	// macOS allows 104 bytes including the NUL terminator; long usernames plus
	// a long cache directory otherwise break SSH with an opaque error.
	maxSocketPathLen = 100
)

// controlPersist returns the ControlPersist value to pass to ssh.
func controlPersist() string {
	if v := strings.TrimSpace(os.Getenv(controlPersistEnv)); v != "" {
		return v
	}
	return defaultControlPersist
}

// controlSocketName derives a stable, short filename for a host's control
// socket. The hash keeps the name fixed-width regardless of how long the
// user@host string is.
func controlSocketName(host string, port int) string {
	sum := sha256.Sum256([]byte(host + ":" + strconv.Itoa(port)))
	return hex.EncodeToString(sum[:])[:12] + ".sock"
}

// controlSocketDirs lists candidate directories for control sockets, most
// preferred first. The /tmp fallback exists for homes deep enough that the
// cache path would overflow sun_path.
func controlSocketDirs() []string {
	var dirs []string
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "atmux", "ssh"))
	} else if cache, err := os.UserCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(cache, "atmux", "ssh"))
	}
	dirs = append(dirs, filepath.Join("/tmp", "atmux-"+strconv.Itoa(os.Getuid()), "ssh"))
	return dirs
}

// controlSocketPath returns the persistent control socket path for a host,
// creating its parent directory. The path is stable across processes so that
// a socket opened by one atmux invocation is reused by the next.
func controlSocketPath(host string, port int) (string, error) {
	name := controlSocketName(host, port)

	var lastErr error
	for _, dir := range controlSocketDirs() {
		path := filepath.Join(dir, name)
		if len(path) > maxSocketPathLen {
			lastErr = fmt.Errorf("control socket path %q exceeds %d bytes", path, maxSocketPathLen)
			continue
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			lastErr = err
			continue
		}
		// Re-assert the mode: anyone who can write to this directory can
		// hijack the control socket and ride the authenticated connection.
		if err := os.Chmod(dir, 0700); err != nil {
			lastErr = err
			continue
		}
		return path, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no usable control socket directory")
	}
	return "", fmt.Errorf("failed to resolve control socket path for %s: %w", host, lastErr)
}
