package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/porganisciak/agent-tmux/tmux"
	"github.com/spf13/cobra"
)

var tilesCmd = &cobra.Command{
	Use:     "tiles",
	Aliases: []string{"mosaic"},
	Short:   "Your most recent Claude Code panes side by side in one window",
	Long: `Shows your most recently active Claude Code panes as tiles in one window,
as many as fit (tiles of at least 80x20), working ones first. The tiles are
the real panes: type into them, scroll them, zoom one with prefix z.
Right-click a tile to zoom it, put it back, go to its window, or close its
session.

tmux can show a pane in only one place, so each pane is borrowed: a stand-in
holds its place in its own window, which otherwise stays exactly as it was.
Leave the tiles view (switch session, prefix s, detach) and every pane goes
back. Running atmux tiles again picks the panes afresh.

Only this machine's panes can be tiled; panes on remote hosts stay put.`,
	RunE: runTiles,
}

var (
	tilesCount         int
	tilesRestore       bool
	tilesStandIn       bool
	tilesPutBack       string
	tilesGo            string
	tilesClient        string
	tilesClose         string
	tilesRestoreIfLeft bool
)

func init() {
	rootCmd.AddCommand(tilesCmd)
	tilesCmd.Flags().IntVarP(&tilesCount, "count", "n", 0, "Number of tiles (default: as many as fit)")
	tilesCmd.Flags().BoolVar(&tilesRestore, "restore", false, "Put every borrowed pane back now")
	tilesCmd.Flags().BoolVar(&tilesStandIn, "stand-in", false, "Hold a borrowed pane's place (internal)")
	tilesCmd.Flags().BoolVar(&tilesRestoreIfLeft, "restore-if-left", false, "Put panes back if nobody is viewing the tiles (internal)")
	tilesCmd.Flags().StringVar(&tilesPutBack, "put-back", "", "Put one tiled pane back in its window (tile menu)")
	tilesCmd.Flags().StringVar(&tilesGo, "go", "", "Close the tiles and go to this pane's window (tile menu)")
	tilesCmd.Flags().StringVar(&tilesClient, "client", "", "Client to move with --go (tile menu)")
	tilesCmd.Flags().StringVar(&tilesClose, "close", "", "Kill the session this tiled pane belongs to (tile menu)")
	for _, name := range []string{"put-back", "go", "client", "close"} {
		tilesCmd.Flags().MarkHidden(name)
	}
	tilesCmd.Flags().MarkHidden("stand-in")
	tilesCmd.Flags().MarkHidden("restore-if-left")
}

func runTiles(cmd *cobra.Command, args []string) error {
	switch {
	case tilesRestore:
		return tmux.RestoreTiles()
	case tilesRestoreIfLeft:
		return tmux.RestoreTilesIfUnviewed()
	case tilesStandIn:
		return runTilesStandIn()
	case tilesPutBack != "":
		return tmux.PutBackTile(tilesPutBack)
	case tilesGo != "":
		return tmux.GoToTile(tilesGo, tilesClient)
	case tilesClose != "":
		return tmux.CloseTiledSession(tilesClose)
	}

	if !tmuxServerRunning() {
		return fmt.Errorf("tmux server not running - start a tmux session first")
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	inTmux := os.Getenv("TMUX") != ""
	if inTmux && currentClientSession() == tmux.TilesSessionName {
		// Rebuilding closes the view this client is in, which would detach
		// it. Step out first, without the hooks reacting to it.
		tmux.DisarmTilesHooks()
		if exec.Command("tmux", "switch-client", "-l").Run() != nil {
			exec.Command("tmux", "switch-client", "-n").Run()
		}
	}

	width, height := clientSize()
	count := tilesCount
	if count <= 0 {
		// One row goes to the status line.
		count = tmux.TileCount(width, height-1)
	}

	// Put back anything still borrowed before choosing, so those panes are
	// listed under their own sessions again.
	if err := tmux.RestoreTiles(); err != nil {
		return err
	}
	agents := tmux.ListAgentPanes([]tmux.TmuxExecutor{tmux.NewLocalExecutor()})
	tmux.LoadTranscripts(agents)
	recent := tmux.RecentAgents(agents, time.Now())
	if len(recent) == 0 {
		return fmt.Errorf("no Claude Code panes on this machine to show")
	}
	if len(recent) > count {
		recent = recent[:count]
	}

	borrowed, err := tmux.OpenTiles(tmux.TilesOptions{
		Agents:  recent,
		Width:   width,
		Height:  height - 1,
		Command: shellJoin(self, "tiles"),
	})
	if err != nil {
		return err
	}
	if inTmux {
		if err := exec.Command("tmux", "switch-client", "-t", tmux.TilesSessionName).Run(); err != nil {
			return err
		}
		exec.Command("tmux", "display-message", "-d", "5000",
			fmt.Sprintf("atmux tiles: %d panes. Right-click a tile for options; leave this session to put them back.", borrowed)).Run()
		return nil
	}
	return tmux.AttachToSession(tmux.TilesSessionName)
}

// runTilesStandIn is what holds a borrowed pane's place in its own window.
func runTilesStandIn() error {
	fmt.Print("\n  This Claude Code pane is in the atmux tiles view.\n" +
		"  It comes back here when you leave that view.\n\n" +
		"  Press Enter to bring it back now.\n")
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		// No terminal to read from: keep holding the place.
		for {
			time.Sleep(time.Hour)
		}
	}
	return tmux.RestoreTiles()
}

func currentClientSession() string {
	out, err := exec.Command("tmux", "display-message", "-p", "#{client_session}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// clientSize is the size of the terminal the tiles will fill.
func clientSize() (width, height int) {
	if os.Getenv("TMUX") != "" {
		out, err := exec.Command("tmux", "display-message", "-p", "#{client_width} #{client_height}").Output()
		if err == nil {
			if w, h, ok := parseSize(string(out)); ok {
				return w, h
			}
		}
	}
	stty := exec.Command("stty", "size")
	stty.Stdin = os.Stdin
	if out, err := stty.Output(); err == nil {
		if h, w, ok := parseSize(string(out)); ok {
			return w, h
		}
	}
	return 160, 48
}

func parseSize(s string) (a, b int, ok bool) {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0, 0, false
	}
	a, err1 := strconv.Atoi(f[0])
	b, err2 := strconv.Atoi(f[1])
	return a, b, err1 == nil && err2 == nil && a > 0 && b > 0
}

// shellJoin quotes a command line for sh.
func shellJoin(args ...string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}
