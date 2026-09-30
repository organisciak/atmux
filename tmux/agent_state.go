package tmux

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/porganisciak/agent-tmux/claude"
)

// Activity is what an agent pane needs from the user, coarse enough to map
// onto a colour or a light.
type Activity string

const (
	// ActivityWorking: a turn is running. Nothing to do.
	ActivityWorking Activity = "working"
	// ActivityNeedsInput: blocked on a permission prompt, question, or dialog.
	ActivityNeedsInput Activity = "needs-input"
	// ActivityReady: the last turn finished recently; there is a reply to read.
	ActivityReady Activity = "ready"
	// ActivityDormant: finished long enough ago that it has gone cold.
	ActivityDormant Activity = "dormant"
	// ActivityFresh: started but never prompted.
	ActivityFresh Activity = "fresh"
	// ActivityUnknown: an agent pane whose state could not be read.
	ActivityUnknown Activity = "unknown"
)

// DormantAfter is how long an idle agent stays Ready before going Dormant.
const DormantAfter = 30 * time.Minute

// AgentPane is a Claude Code pane joined with what Claude says about itself.
type AgentPane struct {
	ClaudePane

	// Process is Claude's registry entry; Found is false when there was none
	// (a remote host, or a Claude Code too old to write one).
	Process claude.Process
	Found   bool

	// Transcript is filled in by LoadTranscripts.
	Transcript claude.Transcript

	// Status and Since are the registry's when Found, otherwise read off the
	// screen, in which case Since is zero.
	Status     claude.Status
	WaitingFor string
	Since      time.Time
}

// Activity classifies the pane for display. now is passed in so a whole list
// is classified against the same instant.
func (a AgentPane) Activity(now time.Time) Activity {
	switch a.Status {
	case claude.StatusBusy:
		return ActivityWorking
	case claude.StatusWaiting:
		return ActivityNeedsInput
	case claude.StatusIdle:
		if a.Transcript.Empty {
			return ActivityFresh
		}
		if !a.Since.IsZero() && now.Sub(a.Since) >= DormantAfter {
			return ActivityDormant
		}
		return ActivityReady
	}
	return ActivityUnknown
}

// agentPaneFormat lists every pane on a server in one call. Tab-separated
// because titles are free text.
const agentPaneFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_title}\t#{pane_current_command}\t#{pane_active}\t#{pane_pid}\t#{window_activity}"

// ListAgentPanes finds every Claude Code pane across executors and attaches
// its state.
//
// Local panes are matched to Claude's session registry by pane id. Anything
// the registry does not cover falls back to reading the pane's screen, which
// costs a capture per pane but works on any host and any Claude version.
func ListAgentPanes(executors []TmuxExecutor) []AgentPane {
	procs := claude.LiveProcesses()
	byPane := make(map[string]claude.Process)
	for _, p := range procs {
		if p.TmuxPane != "" {
			byPane[p.TmuxPane] = p
		}
	}

	// Claude records its pane only when it can see $TMUX_PANE; a process
	// started some other way is matched through its parent, the pane's shell.
	var byParent map[int]claude.Process

	var agents []AgentPane
	for _, exec := range executors {
		if re, ok := exec.(*RemoteExecutor); ok {
			if remote, ok := re.agentsViaAtmux(); ok {
				agents = append(agents, remote...)
				continue
			}
		}
		out, err := exec.Output("list-panes", "-a", "-F", agentPaneFormat)
		if err != nil {
			// An unreachable host or a stopped server has no agents to add.
			continue
		}
		for _, row := range parseAgentPanes(string(out), exec) {
			a := AgentPane{ClaudePane: row.pane}
			p, ok := byPane[row.pane.Pane.ID]
			if !ok && !exec.IsRemote() {
				if byParent == nil {
					byParent = processesByParent(procs)
				}
				p, ok = byParent[row.panePID]
			}
			if ok && !exec.IsRemote() {
				a.Process, a.Found = p, true
				a.Status, a.WaitingFor, a.Since = p.Status, p.WaitingFor, p.Since
			} else {
				a.Status, a.WaitingFor = screenStatus(capturePaneTail(row.pane))
				// The screen says what, not since when; the window's last
				// output is the closest stand-in for an idle agent.
				if a.Status == claude.StatusIdle {
					a.Since = row.lastActivity
				}
			}
			agents = append(agents, a)
		}
	}
	return agents
}

type agentPaneRow struct {
	pane         ClaudePane
	panePID      int
	lastActivity time.Time
}

func parseAgentPanes(output string, exec TmuxExecutor) []agentPaneRow {
	var rows []agentPaneRow
	for _, line := range strings.Split(output, "\n") {
		parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(parts) != 10 {
			continue
		}
		index, _ := strconv.Atoi(parts[4])
		pane := Pane{
			ID:      parts[0],
			Index:   index,
			Title:   parts[5],
			Command: parts[6],
			Active:  parts[7] == "1",
			Target:  parts[1] + ":" + parts[2] + "." + parts[4],
		}
		if !isClaudePane(pane) {
			continue
		}
		pid, _ := strconv.Atoi(parts[8])
		row := agentPaneRow{
			pane: ClaudePane{
				Pane:        pane,
				SessionName: parts[1],
				WindowName:  parts[3],
				Host:        exec.HostLabel(),
				Executor:    exec,
			},
			panePID: pid,
		}
		if secs, err := strconv.ParseInt(parts[9], 10, 64); err == nil && secs > 0 {
			row.lastActivity = time.Unix(secs, 0)
		}
		rows = append(rows, row)
	}
	return rows
}

// processesByParent indexes the registry entries that carry no pane id by
// their parent pid.
func processesByParent(procs []claude.Process) map[int]claude.Process {
	byParent := make(map[int]claude.Process)
	var unplaced []claude.Process
	for _, p := range procs {
		if p.TmuxPane == "" {
			unplaced = append(unplaced, p)
		}
	}
	if len(unplaced) == 0 {
		return byParent
	}
	parents := parentPIDs()
	for _, p := range unplaced {
		if ppid, ok := parents[p.PID]; ok {
			byParent[ppid] = p
		}
	}
	return byParent
}

// parentPIDs maps every process to its parent with one ps call.
func parentPIDs() map[int]int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	parents := make(map[int]int)
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			parents[pid] = ppid
		}
	}
	return parents
}

// LoadTranscripts reads recaps and last messages for the panes that have a
// registry entry. Transcripts only exist on the machine running Claude, so
// remote panes are left without.
func LoadTranscripts(agents []AgentPane) {
	for i := range agents {
		if agents[i].Found {
			agents[i].Transcript = claude.ReadTranscript(agents[i].Process.SessionID)
		}
	}
}

func capturePaneTail(cp ClaudePane) string {
	out, err := cp.Executor.Output("capture-pane", "-p", "-t", cp.Pane.ID)
	if err != nil {
		return ""
	}
	return string(out)
}

// screenStatus reads an agent's state off its screen, for panes the registry
// does not describe. Claude's footer offers "esc to interrupt" only while a
// turn runs, and permission prompts always end in a numbered choice.
func screenStatus(screen string) (claude.Status, string) {
	if strings.TrimSpace(screen) == "" {
		return claude.StatusUnknown, ""
	}
	tail := lastLines(screen, 12)
	switch {
	case strings.Contains(tail, "esc to interrupt"):
		return claude.StatusBusy, ""
	case strings.Contains(tail, "Do you want to") || strings.Contains(tail, "❯ 1."):
		return claude.StatusWaiting, "permission prompt"
	}
	return claude.StatusIdle, ""
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var kept []string
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			kept = append(kept, lines[i])
		}
	}
	return strings.Join(kept, "\n")
}
