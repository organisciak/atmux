package config

import (
	"fmt"
	"os"
	"strings"
)

// AppendRemoteProject appends a remote project entry block to a config file.
func AppendRemoteProject(configPath string, project RemoteProjectConfig) error {
	normalized, err := NormalizeRemoteProject(project)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(configPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if stat.Size() > 0 {
		// Ensure there is a blank line between existing content and appended block.
		if _, err := f.Seek(-1, 2); err != nil {
			return err
		}
		last := make([]byte, 1)
		if _, err := f.Read(last); err != nil {
			return err
		}
		if _, err := f.Seek(0, 2); err != nil {
			return err
		}
		if last[0] != '\n' {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
	}

	block := strings.Builder{}
	block.WriteString("\n")
	block.WriteString(fmt.Sprintf("# Remote project: %s\n", normalized.Name))
	block.WriteString(fmt.Sprintf("remote_project:%s\n", normalized.Name))
	block.WriteString(fmt.Sprintf("remote_project_host:%s\n", normalized.Host))
	block.WriteString(fmt.Sprintf("remote_project_dir:%s\n", normalized.WorkingDir))
	block.WriteString(fmt.Sprintf("remote_project_session:%s\n", normalized.SessionName))

	_, err = f.WriteString(block.String())
	return err
}
