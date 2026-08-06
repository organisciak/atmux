package tmux

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
)

// BeadsOpenCount returns the number of open beads issues in dir.
//
// The second return value reports whether dir is a beads project at all, which
// is distinct from a project with zero open issues: the UI shows nothing for
// the former and "0" for the latter.
func BeadsOpenCount(dir string) (int, bool) {
	if dir == "" {
		return 0, false
	}
	if _, err := os.Stat(filepath.Join(dir, ".beads")); err != nil {
		return 0, false
	}

	cmd := exec.Command("bd", "count", "--status=open", "--json")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return 0, false
	}

	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return 0, false
	}
	return result.Count, true
}
