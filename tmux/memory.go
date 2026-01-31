package tmux

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// PaneMemory represents memory usage for a single tmux pane.
type PaneMemory struct {
	Index    int
	PID      int
	RSSBytes int64
}

// WindowMemory represents memory usage for a tmux window and its panes.
type WindowMemory struct {
	Index int
	Name  string
	Panes []PaneMemory
}

// SessionMemory represents memory usage for a tmux session.
type SessionMemory struct {
	Name    string
	Windows []WindowMemory
}

type paneMemoryRow struct {
	sessionName string
	windowIndex int
	windowName  string
	paneIndex   int
	pid         int
}

// FetchSessionMemory returns memory usage for panes grouped by session and window.
// Best-effort: returns empty data when tmux or ps are unavailable.
func FetchSessionMemory() (map[string]SessionMemory, error) {
	cmd := exec.Command("tmux", "list-panes", "-a",
		"-F", "#{session_name}\t#{window_index}\t#{window_name}\t#{pane_index}\t#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return map[string]SessionMemory{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var rows []paneMemoryRow
	var pids []int
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		windowIndex, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		paneIndex, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}
		pid, err := strconv.Atoi(parts[4])
		if err != nil || pid <= 0 {
			continue
		}
		rows = append(rows, paneMemoryRow{
			sessionName: parts[0],
			windowIndex: windowIndex,
			windowName:  parts[2],
			paneIndex:   paneIndex,
			pid:         pid,
		})
		pids = append(pids, pid)
	}

	rssBytesByPID := rssBytesForPIDs(pids)

	type sessionBuild struct {
		name    string
		windows map[int]*WindowMemory
	}
	sessions := map[string]*sessionBuild{}

	for _, row := range rows {
		sess, ok := sessions[row.sessionName]
		if !ok {
			sess = &sessionBuild{name: row.sessionName, windows: map[int]*WindowMemory{}}
			sessions[row.sessionName] = sess
		}
		win, ok := sess.windows[row.windowIndex]
		if !ok {
			win = &WindowMemory{Index: row.windowIndex, Name: row.windowName}
			sess.windows[row.windowIndex] = win
		}
		win.Panes = append(win.Panes, PaneMemory{
			Index:    row.paneIndex,
			PID:      row.pid,
			RSSBytes: rssBytesByPID[row.pid],
		})
	}

	result := map[string]SessionMemory{}
	for name, sess := range sessions {
		windows := make([]WindowMemory, 0, len(sess.windows))
		for _, win := range sess.windows {
			sort.Slice(win.Panes, func(i, j int) bool {
				return win.Panes[i].Index < win.Panes[j].Index
			})
			windows = append(windows, *win)
		}
		sort.Slice(windows, func(i, j int) bool {
			return windows[i].Index < windows[j].Index
		})
		result[name] = SessionMemory{Name: name, Windows: windows}
	}

	return result, nil
}

func rssBytesForPIDs(pids []int) map[int]int64 {
	result := map[int]int64{}
	if len(pids) == 0 {
		return result
	}

	pidSet := map[int]struct{}{}
	for _, pid := range pids {
		if pid > 0 {
			pidSet[pid] = struct{}{}
		}
	}
	if len(pidSet) == 0 {
		return result
	}

	pidList := make([]int, 0, len(pidSet))
	for pid := range pidSet {
		pidList = append(pidList, pid)
	}
	sort.Ints(pidList)

	pidStrings := make([]string, 0, len(pidList))
	for _, pid := range pidList {
		pidStrings = append(pidStrings, strconv.Itoa(pid))
	}

	cmd := exec.Command("ps", "-o", "pid=,rss=", "-p", strings.Join(pidStrings, ","))
	output, err := cmd.Output()
	if err != nil {
		return result
	}

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		rssKB, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || rssKB < 0 {
			continue
		}
		result[pid] = rssKB * 1024
	}

	return result
}
