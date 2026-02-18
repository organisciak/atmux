package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/porganisciak/agent-tmux/config"
	"github.com/spf13/cobra"
)

var (
	remoteProjectHost    string
	remoteProjectDir     string
	remoteProjectSession string
)

var remoteProjectCmd = &cobra.Command{
	Use:     "remote-project [name]",
	Aliases: []string{"rproj"},
	Short:   "Create a reusable remote project in global config",
	Long: `Create a reusable remote project entry in the global atmux config.

The project is stored in ~/.config/atmux/config so it can be shared and edited
as plain text with other global atmux settings.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRemoteProject,
}

func init() {
	rootCmd.AddCommand(remoteProjectCmd)
	remoteProjectCmd.Flags().StringVar(&remoteProjectHost, "host", "",
		"Remote host alias/host for this project (required)")
	remoteProjectCmd.Flags().StringVar(&remoteProjectDir, "dir", "",
		"Remote working directory for this project (required)")
	remoteProjectCmd.Flags().StringVar(&remoteProjectSession, "session", "",
		"Optional tmux session name (defaults to agent-<project>)")
	remoteProjectCmd.MarkFlagRequired("host") //nolint:errcheck
	remoteProjectCmd.MarkFlagRequired("dir")  //nolint:errcheck
}

func runRemoteProject(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) > 0 {
		name = strings.TrimSpace(args[0])
	}
	if name == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to determine project name from cwd: %w", err)
		}
		name = filepath.Base(cwd)
	}

	project, err := config.NormalizeRemoteProject(config.RemoteProjectConfig{
		Name:        name,
		Host:        remoteProjectHost,
		WorkingDir:  remoteProjectDir,
		SessionName: remoteProjectSession,
	})
	if err != nil {
		return fmt.Errorf("invalid remote project: %w", err)
	}

	globalPath, err := config.GlobalConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get global config path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0755); err != nil {
		return fmt.Errorf("failed to create global config directory: %w", err)
	}
	if !config.Exists(globalPath) {
		if err := os.WriteFile(globalPath, []byte(config.GlobalTemplate()), 0644); err != nil {
			return fmt.Errorf("failed to initialize global config: %w", err)
		}
	}

	cfg, err := config.Parse(globalPath)
	if err != nil {
		return fmt.Errorf("failed to parse global config %s: %w", globalPath, err)
	}
	for _, existing := range cfg.RemoteProjects {
		if strings.EqualFold(existing.Name, project.Name) {
			return fmt.Errorf("remote project %q already exists in %s", project.Name, globalPath)
		}
	}

	if err := config.AppendRemoteProject(globalPath, project); err != nil {
		return fmt.Errorf("failed to write remote project to %s: %w", globalPath, err)
	}

	fmt.Printf("Created remote project %q in %s\n", project.Name, globalPath)
	fmt.Printf("  host:    %s\n", project.Host)
	fmt.Printf("  dir:     %s\n", project.WorkingDir)
	fmt.Printf("  session: %s\n", project.SessionName)
	fmt.Printf("Edit %s to adjust remote_project_* values.\n", globalPath)

	return nil
}
