package cmd

import (
	"os"
	"path/filepath"
)

const hidePathsHelpText = "Hide working directory paths. I don't know why you'd hide paths. I did it for taking screenshots without showing everything off in my filesystem!"

func displayPathForList(path string, hidePaths bool, shorten bool) string {
	if hidePaths {
		return hidePath(path)
	}
	if shorten {
		return shortenPath(path)
	}
	return path
}

func hidePath(path string) string {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "<path hidden>"
	}
	return filepath.Join("<path hidden>", base)
}

// shortenPath replaces home directory with ~ for cleaner display.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if len(path) > len(home) && path[:len(home)] == home {
		return "~" + path[len(home):]
	}
	// Also handle trailing slash case.
	if path == home {
		return "~"
	}
	return path
}
