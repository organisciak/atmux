package tmux

import (
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/history"
)

// FirstRunPrep checks whether workingDir has ever had an atmux session, and if
// not, applies first-run hardening to cfg:
//   - When no color: directive is set, picks a random surprise color and
//     persists it to the local .agent-tmux.conf so future runs stay stable.
//
// Returns the (possibly mutated) cfg, a firstRun flag for Create, and any
// non-fatal error from history/config IO. Callers should treat errors as
// "assume not first run" and proceed with the original cfg.
func FirstRunPrep(workingDir string, cfg *config.Config) (*config.Config, bool, error) {
	firstRun, err := hasNoHistoryFor(workingDir)
	if err != nil {
		return cfg, false, err
	}
	if !firstRun {
		return cfg, false, nil
	}

	if cfg == nil {
		cfg = &config.Config{}
	}

	if strings.TrimSpace(cfg.Color) == "" {
		if color, cerr := config.RandomSurpriseColor(""); cerr == nil {
			localPath := filepath.Join(workingDir, config.DefaultConfigName)
			if werr := config.SetLocalDirective(localPath, "color", color); werr == nil {
				cfg.Color = color
			}
		}
	}
	return cfg, true, nil
}

func hasNoHistoryFor(workingDir string) (bool, error) {
	store, err := history.Open()
	if err != nil {
		return false, err
	}
	defer store.Close()

	has, err := store.HasEntryForWorkingDir(workingDir)
	if err != nil {
		return false, err
	}
	return !has, nil
}
