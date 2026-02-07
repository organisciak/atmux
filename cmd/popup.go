package cmd

import (
	"os"
	"os/exec"
	"strings"
)

// tmuxClientIsPopup returns true when the current tmux client is a popup.
// If the format isn't supported, it safely returns false.
func tmuxClientIsPopup() bool {
	if os.Getenv("TMUX") == "" {
		return false
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{popup}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}
