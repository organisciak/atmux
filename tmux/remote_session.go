package tmux

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Remote sessions opened from inside tmux all land in one local session,
// "remote", one window per host and session. Opening the same remote session
// again goes back to its window instead of stacking another ssh client into
// whatever session happened to be current.
//
// The session and its windows are found by option, not by name, so renaming
// either keeps working.
const (
	RemoteSessionName = "remote"
	// optRemoteSession marks the local session that holds remote attaches.
	optRemoteSession = "@atmux_remotes"
	// optRemoteWindow names the remote session a window is attached to, as
	// "<host alias>:<session>".
	optRemoteWindow = "@atmux_remote"
)

// remoteWindowTag identifies a remote session across hosts.
func remoteWindowTag(host, session string) string { return host + ":" + session }

// remoteWindowName is what the window is called in the local status line.
func remoteWindowName(host, session string) string { return host + "/" + session }

// findRemoteWindow returns the window already attached to tag, from
// `list-windows -a -F "#{window_id}\t#{@atmux_remote}"` output.
func findRemoteWindow(listWindows, tag string) string {
	for _, line := range strings.Split(listWindows, "\n") {
		id, got, ok := strings.Cut(line, "\t")
		if ok && got == tag && id != "" {
			return id
		}
	}
	return ""
}

// findRemoteSession returns the session holding remote attaches, and a free
// name for one if there is none yet, from
// `list-sessions -F "#{session_id}\t#{@atmux_remotes}\t#{session_name}"`
// output. A local session the user named "remote" themselves is left alone.
func findRemoteSession(listSessions string) (id, freeName string) {
	taken := map[string]bool{}
	for _, line := range strings.Split(listSessions, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 {
			continue
		}
		if f[1] == "1" && f[0] != "" {
			return f[0], ""
		}
		taken[f[2]] = true
	}
	name := RemoteSessionName
	for i := 2; taken[name]; i++ {
		name = RemoteSessionName + "-" + strconv.Itoa(i)
	}
	return "", name
}

// holdOnFailure keeps the window open when the connection fails, so the error
// can be read and the window does not vanish before the client switches to it.
// A normal detach or the remote session ending exits 0 and closes the window.
func holdOnFailure(command, host string) string {
	msg := fmt.Sprintf("\\n  atmux: the connection to %s ended with exit status %%d.\\n  Press Enter to close this window.\\n", host)
	return command + " || { s=$?; printf " + shellSingleQuote(msg) + " \"$s\"; read _; }"
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// openRemoteWindow switches the client to the local window attached to the
// remote session, creating the window (and the "remote" session) if needed.
func openRemoteWindow(host, session, command string) error {
	tag := remoteWindowTag(host, session)
	out, err := exec.Command("tmux", "list-windows", "-a", "-F", "#{window_id}\t#{"+optRemoteWindow+"}").Output()
	if err != nil {
		return fmt.Errorf("list windows: %w", err)
	}
	if id := findRemoteWindow(string(out), tag); id != "" {
		return switchToRemoteWindow(id, host)
	}

	out, err = exec.Command("tmux", "list-sessions", "-F", "#{session_id}\t#{"+optRemoteSession+"}\t#{session_name}").Output()
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	sessionID, name := findRemoteSession(string(out))
	command = holdOnFailure(command, host)
	windowName := remoteWindowName(host, session)

	var created []byte
	if sessionID == "" {
		created, err = exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{session_id}\t#{window_id}",
			"-s", name, "-n", windowName, command).Output()
		if err != nil {
			return fmt.Errorf("create the %s session: %w", name, err)
		}
		sid, wid, _ := strings.Cut(strings.TrimSpace(string(created)), "\t")
		exec.Command("tmux", "set-option", "-t", sid, optRemoteSession, "1").Run()
		// When the last remote window closes the session goes with it; land
		// on another session then, not outside tmux.
		exec.Command("tmux", "set-option", "-t", sid, "detach-on-destroy", "off").Run()
		created = []byte(wid)
	} else {
		created, err = exec.Command("tmux", "new-window", "-d", "-P", "-F", "#{window_id}",
			"-t", sessionID+":", "-n", windowName, command).Output()
		if err != nil {
			return fmt.Errorf("open a window for %s: %w", tag, err)
		}
	}
	id := strings.TrimSpace(string(created))
	// automatic-rename would replace the name with "ssh".
	exec.Command("tmux", "set-option", "-w", "-t", id, "automatic-rename", "off").Run()
	exec.Command("tmux", "set-option", "-w", "-t", id, optRemoteWindow, tag).Run()
	return switchToRemoteWindow(id, host)
}

func switchToRemoteWindow(id, host string) error {
	if err := exec.Command("tmux", "switch-client", "-t", id).Run(); err != nil {
		return fmt.Errorf("could not open the window for %s (it may have closed): %w", host, err)
	}
	return nil
}
