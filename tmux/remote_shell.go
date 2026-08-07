package tmux

// remoteShellMode records how commands have to be invoked on a host.
//
// SSH runs commands in a non-interactive, non-login shell. Its PATH is
// frequently just the system defaults — on macOS, literally
// /usr/bin:/bin:/usr/sbin:/sbin — which excludes /usr/local/bin and Homebrew.
// A host can therefore report "tmux: command not found" while tmux is
// installed and running sessions.
type remoteShellMode int

const (
	// remoteShellUnknown means the host has not been probed yet.
	remoteShellUnknown remoteShellMode = iota
	// remoteShellDirect means commands are on the non-interactive PATH.
	remoteShellDirect
	// remoteShellLogin means commands need a login shell to be found.
	remoteShellLogin
)

// loginShellFallback is the shell used to pick up a host's login profile.
// bash is chosen over the user's own shell because it is near-universally
// present and `-lc` behaves consistently; the profile it sources is what
// matters, not which shell sources it.
const loginShellFallback = "bash"

// wrapRemoteCommand renders a command and its arguments for the given mode.
func wrapRemoteCommand(mode remoteShellMode, command string, args []string) string {
	direct := remoteCommand(command, args)
	if mode != remoteShellLogin {
		return direct
	}
	return loginShellFallback + " -lc " + shellQuote(direct)
}
