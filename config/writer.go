package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// SetLocalDirective updates an existing single-value directive in the file at
// path, or appends it if absent. If the file doesn't exist, it's created with a
// short header followed by the directive. Comment lines starting with the same
// directive (e.g. "# color:foo") are ignored.
func SetLocalDirective(path, directive, value string) error {
	directive = strings.TrimSpace(directive)
	if directive == "" {
		return fmt.Errorf("directive is empty")
	}
	newLine := directive + ":" + value

	if !Exists(path) {
		body := fmt.Sprintf("# atmux (agent-tmux) project configuration\n# See `atmux init` for the full template.\n\n%s\n", newLine)
		return os.WriteFile(path, []byte(body), 0644)
	}

	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return err
	}

	replaced := false
	prefix := directive + ":"
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = newLine
			replaced = true
			break
		}
	}

	if !replaced {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, newLine)
	}

	return writeLines(path, lines, trailingNewline)
}

// RemoveLocalDirective removes any non-comment line beginning with
// "<directive>:" from the file at path. Missing files and missing directives
// are no-ops.
func RemoveLocalDirective(path, directive string) error {
	if !Exists(path) {
		return nil
	}
	lines, trailingNewline, err := readLines(path)
	if err != nil {
		return err
	}

	prefix := strings.TrimSpace(directive) + ":"
	out := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") && strings.HasPrefix(trimmed, prefix) {
			removed = true
			continue
		}
		out = append(out, line)
	}
	if !removed {
		return nil
	}
	return writeLines(path, out, trailingNewline)
}

func readLines(path string) ([]string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, false, err
	}

	// Detect trailing newline by re-reading the last byte.
	trailing := false
	info, err := os.Stat(path)
	if err == nil && info.Size() > 0 {
		f2, err := os.Open(path)
		if err == nil {
			defer f2.Close()
			if _, err := f2.Seek(-1, 2); err == nil {
				buf := make([]byte, 1)
				if _, err := f2.Read(buf); err == nil && buf[0] == '\n' {
					trailing = true
				}
			}
		}
	}
	return lines, trailing, nil
}

func writeLines(path string, lines []string, trailingNewline bool) error {
	body := strings.Join(lines, "\n")
	if trailingNewline || len(lines) > 0 {
		body += "\n"
	}
	return os.WriteFile(path, []byte(body), 0644)
}
