package tmux

import (
	"os/exec"
	"runtime"
)

// OpenURL launches the platform's default browser for the given URL.
// Best-effort: errors are returned so callers can surface them but no
// retry is attempted.
func OpenURL(url string) error {
	if url == "" {
		return nil
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
