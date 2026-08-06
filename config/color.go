package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// SurprisePalette is a curated set of vibrant, distinguishable hex colors
// used by `atmux color surprise`. Inspired by VS Code's Peacock defaults
// plus a few extras that read well as a tmux status bar.
var SurprisePalette = []string{
	"#1857a4", // azure blue
	"#42b883", // vue green
	"#dd0531", // angular red
	"#519aba", // typescript blue
	"#637777", // gatsby purple-grey
	"#215732", // node green
	"#832561", // magenta
	"#bd10e0", // electric violet
	"#e9a800", // amber
	"#007acc", // vscode blue
	"#c8553d", // burnt orange
	"#2f4858", // slate
	"#16a085", // teal
	"#8e44ad", // royal purple
	"#d35400", // pumpkin
}

var (
	hexColorRE    = regexp.MustCompile(`^#?[0-9a-fA-F]{3}$|^#?[0-9a-fA-F]{6}$`)
	tmuxColourRE  = regexp.MustCompile(`^colour\d{1,3}$`)
	namedTmuxRE   = regexp.MustCompile(`^[a-z]+$`)
	namedTmuxSet  = map[string]bool{
		"black": true, "red": true, "green": true, "yellow": true,
		"blue": true, "magenta": true, "cyan": true, "white": true,
		"brightblack": true, "brightred": true, "brightgreen": true,
		"brightyellow": true, "brightblue": true, "brightmagenta": true,
		"brightcyan": true, "brightwhite": true, "default": true,
		"terminal": true,
	}
)

// NormalizeColor validates and normalizes a color string. Accepts:
//   - hex: "#42b883", "42b883", "#abc", "abc"
//   - tmux indexed: "colour39", "colour255"
//   - tmux named: "red", "cyan", "brightblue", "default"
//
// Returns the canonical form (hex normalized to "#rrggbb", others lowercased).
func NormalizeColor(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return "", fmt.Errorf("color value is empty")
	}

	if hexColorRE.MatchString(v) {
		h := strings.TrimPrefix(v, "#")
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		return "#" + h, nil
	}

	if tmuxColourRE.MatchString(v) {
		return v, nil
	}

	if namedTmuxRE.MatchString(v) && namedTmuxSet[v] {
		return v, nil
	}

	return "", fmt.Errorf("invalid color %q (use hex like #42b883, named like red, or colour39)", value)
}

// ContrastingFg returns a tmux color value for foreground text that reads
// well against the given background color. For hex colors it computes
// luminance; for non-hex values it defaults to white.
func ContrastingFg(bg string) string {
	bg = strings.ToLower(strings.TrimSpace(bg))
	if !strings.HasPrefix(bg, "#") || len(bg) != 7 {
		return "white"
	}
	raw, err := hex.DecodeString(bg[1:])
	if err != nil || len(raw) != 3 {
		return "white"
	}
	// Perceptual luminance (Rec. 709 weights).
	l := 0.2126*float64(raw[0]) + 0.7152*float64(raw[1]) + 0.0722*float64(raw[2])
	if l > 140 {
		return "black"
	}
	return "white"
}

// RandomSurpriseColor picks a random color from SurprisePalette. If avoid is
// non-empty and the palette has more than one entry, the returned color will
// not equal avoid (so consecutive surprises always change).
func RandomSurpriseColor(avoid string) (string, error) {
	avoid = strings.ToLower(strings.TrimSpace(avoid))
	n := len(SurprisePalette)
	if n == 0 {
		return "", fmt.Errorf("empty surprise palette")
	}
	for attempt := 0; attempt < 8; attempt++ {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
		if err != nil {
			return "", err
		}
		c := SurprisePalette[idx.Int64()]
		if strings.ToLower(c) != avoid || n == 1 {
			return c, nil
		}
	}
	// Fallback: pick the next index after avoid.
	for i, c := range SurprisePalette {
		if strings.ToLower(c) == avoid {
			return SurprisePalette[(i+1)%n], nil
		}
	}
	return SurprisePalette[0], nil
}
