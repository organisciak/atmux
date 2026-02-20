package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
)

// OnboardResult contains the outcome of the onboard interaction.
type OnboardResult struct {
	Completed     bool
	Agents        []config.AgentConfig
	KeybindAdded  bool
	KeybindError  string
}

// RunOnboard runs the interactive onboard TUI.
func RunOnboard() (*OnboardResult, error) {
	m := newOnboardModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(onboardModel); ok {
		return &OnboardResult{
			Completed:    model.completed,
			Agents:       model.buildAgents(),
			KeybindAdded: model.keybindAdded,
			KeybindError: model.keybindError,
		}, nil
	}
	return &OnboardResult{}, nil
}

type agentChoice struct {
	name     string
	command  string
	enabled  bool
	yolo     bool
	flags    string
}

type onboardModel struct {
	width        int
	height       int
	step         int // 0=welcome, 1=agent selection, 2=flags, 3=confirm, 4=keybinding
	cursor       int
	agents       []agentChoice
	completed    bool
	keybindAdded bool
	keybindError string

	// Command editing in the review step
	editingCommands bool              // true when in command edit mode
	commandInputs   []textinput.Model // one text input per enabled agent
	editCursor      int               // which command input is focused
}

func newOnboardModel() onboardModel {
	return onboardModel{
		step: 0,
		agents: []agentChoice{
			{name: "Claude", command: "claude", enabled: true, yolo: true},
			{name: "Codex", command: "codex", enabled: true, yolo: true},
			{name: "Gemini CLI", command: "gemini", enabled: false, yolo: false},
		},
	}
}

func (m onboardModel) Init() tea.Cmd {
	return nil
}

func (m onboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// When editing commands, handle text input
		if m.editingCommands {
			return m.handleEditingKeys(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "enter":
			return m.handleEnter()

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case "down", "j":
			m.cursor++
			if max := m.maxCursor(); m.cursor > max {
				m.cursor = max
			}
			return m, nil

		case " ", "space":
			return m.handleSpace()

		case "tab":
			return m.handleTab()

		case "backspace", "esc":
			if m.step > 0 {
				m.step--
				m.cursor = 0
			}
			return m, nil
		}
	}
	return m, nil
}

func (m onboardModel) maxCursor() int {
	switch m.step {
	case 1: // Agent selection
		return len(m.agents) // agents + Continue button
	case 2: // Flags
		count := 0
		for _, a := range m.agents {
			if a.enabled {
				count++
			}
		}
		return count // each enabled agent has a YOLO toggle + Continue
	case 3: // Confirm
		return 3 // Edit Commands, Save & Continue, Save & Edit, Skip
	case 4: // Keybind
		return 1 // Yes and No buttons
	default:
		return 0
	}
}

func (m onboardModel) handleEnter() (tea.Model, tea.Cmd) {
	switch m.step {
	case 0: // Welcome -> Agent selection
		m.step = 1
		m.cursor = 0
		return m, nil

	case 1: // Agent selection -> Flags
		if m.cursor == len(m.agents) {
			// Continue button
			m.step = 2
			m.cursor = 0
		}
		return m, nil

	case 2: // Flags -> Confirm
		enabledCount := 0
		for _, a := range m.agents {
			if a.enabled {
				enabledCount++
			}
		}
		if m.cursor == enabledCount {
			// Continue button
			m.step = 3
			m.cursor = 0
		}
		return m, nil

	case 3: // Confirm
		if m.cursor == 0 {
			// Edit Commands - switch to inline editing mode
			m.initCommandInputs()
			m.editingCommands = true
			m.editCursor = 0
			if len(m.commandInputs) > 0 {
				m.commandInputs[0].Focus()
			}
			return m, nil
		} else if m.cursor == 1 {
			// Save & Continue
			if err := m.saveConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save config: %v\n", err)
			}
			m.completed = true
			m.step = 4
			m.cursor = 0
			return m, nil
		} else if m.cursor == 2 {
			// Save & Edit - save config then go back to agent selection
			if err := m.saveConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save config: %v\n", err)
			}
			m.completed = true
			m.step = 1
			m.cursor = 0
			return m, nil
		}
		// Skip - just go to keybinding step anyway
		m.step = 4
		m.cursor = 0
		return m, nil

	case 4: // Keybind
		if m.cursor == 0 {
			// Yes - add keybinding
			if err := m.addKeybinding(); err != nil {
				m.keybindError = err.Error()
			} else {
				m.keybindAdded = true
			}
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m onboardModel) handleSpace() (tea.Model, tea.Cmd) {
	switch m.step {
	case 1: // Toggle agent enabled
		if m.cursor < len(m.agents) {
			m.agents[m.cursor].enabled = !m.agents[m.cursor].enabled
		}
	case 2: // Toggle YOLO
		idx := 0
		for i := range m.agents {
			if m.agents[i].enabled {
				if idx == m.cursor {
					m.agents[i].yolo = !m.agents[i].yolo
					break
				}
				idx++
			}
		}
	}
	return m, nil
}

func (m onboardModel) handleTab() (tea.Model, tea.Cmd) {
	// Tab cycles through steps forward
	if m.step < 3 {
		m.step++
		m.cursor = 0
	}
	return m, nil
}

// initCommandInputs creates text inputs pre-filled with the generated commands.
func (m *onboardModel) initCommandInputs() {
	agents := m.buildAgents()
	m.commandInputs = make([]textinput.Model, len(agents))
	for i, a := range agents {
		ti := textinput.New()
		ti.SetValue(a.Command)
		ti.CharLimit = 256
		ti.Width = 50
		if i == 0 {
			ti.Focus()
		}
		m.commandInputs[i] = ti
	}
}

// applyCommandEdits writes the edited commands back to the agent choices.
// It replaces the command and flags for each enabled agent with the edited text.
func (m *onboardModel) applyCommandEdits() {
	idx := 0
	for i := range m.agents {
		if !m.agents[i].enabled {
			continue
		}
		if idx < len(m.commandInputs) {
			edited := strings.TrimSpace(m.commandInputs[idx].Value())
			if edited != "" {
				// Store the full command, clear individual flags since user has full control
				m.agents[i].command = edited
				m.agents[i].yolo = false
				m.agents[i].flags = ""
			}
			idx++
		}
	}
}

// handleEditingKeys handles keyboard input when editing commands in the review step.
func (m onboardModel) handleEditingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		// Cancel editing, discard changes
		m.editingCommands = false
		m.commandInputs = nil
		return m, nil
	case "tab", "down":
		// Move to next input
		if len(m.commandInputs) > 0 {
			m.commandInputs[m.editCursor].Blur()
			m.editCursor = (m.editCursor + 1) % len(m.commandInputs)
			m.commandInputs[m.editCursor].Focus()
		}
		return m, nil
	case "shift+tab", "up":
		// Move to previous input
		if len(m.commandInputs) > 0 {
			m.commandInputs[m.editCursor].Blur()
			m.editCursor = (m.editCursor - 1 + len(m.commandInputs)) % len(m.commandInputs)
			m.commandInputs[m.editCursor].Focus()
		}
		return m, nil
	case "enter":
		// Confirm edits
		m.applyCommandEdits()
		m.editingCommands = false
		m.commandInputs = nil
		return m, nil
	}

	// Pass key to the focused text input
	if m.editCursor >= 0 && m.editCursor < len(m.commandInputs) {
		var cmd tea.Cmd
		m.commandInputs[m.editCursor], cmd = m.commandInputs[m.editCursor].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m onboardModel) buildAgents() []config.AgentConfig {
	var agents []config.AgentConfig
	for _, a := range m.agents {
		if !a.enabled {
			continue
		}
		cmd := a.command
		if a.yolo {
			if a.command == "claude" {
				cmd += " --dangerously-skip-permissions"
			} else if a.command == "codex" {
				cmd += " --yolo"
			}
			// Gemini CLI: no known yolo flag yet, use base command as-is
		}
		if a.flags != "" {
			cmd += " " + a.flags
		}
		agents = append(agents, config.AgentConfig{Command: cmd})
	}
	return agents
}

func (m onboardModel) saveConfig() error {
	agents := m.buildAgents()

	// Build config content
	var lines []string
	lines = append(lines, "# atmux global configuration")
	lines = append(lines, "# Generated by atmux onboard")
	lines = append(lines, "")
	lines = append(lines, "# Core agent panes")
	for _, a := range agents {
		lines = append(lines, "agent:"+a.Command)
	}
	lines = append(lines, "")

	content := strings.Join(lines, "\n")

	// Get global config path
	path, err := config.GlobalConfigPath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir, err := config.SettingsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(content), 0644)
}

func (m onboardModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	switch m.step {
	case 0:
		return m.viewWelcome()
	case 1:
		return m.viewAgentSelection()
	case 2:
		return m.viewFlags()
	case 3:
		return m.viewConfirm()
	case 4:
		return m.viewKeybind()
	default:
		return ""
	}
}

func (m onboardModel) viewWelcome() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	subtitleStyle := lipgloss.NewStyle().Foreground(dimColor)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	content := lipgloss.JoinVertical(lipgloss.Center,
		titleStyle.Render("Welcome to atmux!"),
		"",
		"This wizard will help you configure your AI coding agents.",
		"",
		subtitleStyle.Render("You can always change these settings later by running:"),
		subtitleStyle.Render("  atmux init --global"),
		subtitleStyle.Render("or by editing your .agent-tmux.conf file directly."),
		"",
		selectedStyle.Render("Press Enter to continue"),
	)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content))
}

func (m onboardModel) viewAgentSelection() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	uncheckStyle := lipgloss.NewStyle().Foreground(dimColor)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	var lines []string
	lines = append(lines, titleStyle.Render("Select Your Agents"))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("Space to toggle, Enter to continue"))
	lines = append(lines, "")

	for i, agent := range m.agents {
		checkbox := "[ ]"
		style := uncheckStyle
		if agent.enabled {
			checkbox = "[✓]"
			style = checkStyle
		}

		line := fmt.Sprintf("%s %s", checkbox, agent.name)
		if i == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = style.Render("  " + line)
		}
		lines = append(lines, line)
	}

	// Show caution footnote if Gemini is enabled
	geminiEnabled := false
	for _, agent := range m.agents {
		if agent.command == "gemini" && agent.enabled {
			geminiEnabled = true
			break
		}
	}
	if geminiEnabled {
		lines = append(lines, "")
		cautionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		lines = append(lines, cautionStyle.Render("⚠ Gemini CLI support is experimental and has not been extensively tested."))
	}

	lines = append(lines, "")
	continueBtn := "  Continue →"
	if m.cursor == len(m.agents) {
		continueBtn = selectedStyle.Render("> Continue →")
	}
	lines = append(lines, continueBtn)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content))
}

func (m onboardModel) viewFlags() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	uncheckStyle := lipgloss.NewStyle().Foreground(dimColor)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	var lines []string
	lines = append(lines, titleStyle.Render("Configure Agent Flags"))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("Space to toggle auto-approve mode"))
	lines = append(lines, "")

	idx := 0
	for _, agent := range m.agents {
		if !agent.enabled {
			continue
		}

		yoloLabel := "Auto-approve (YOLO mode)"
		if agent.command == "claude" {
			yoloLabel = "--dangerously-skip-permissions"
		} else if agent.command == "codex" {
			yoloLabel = "--yolo"
		} else if agent.command == "gemini" {
			yoloLabel = "Auto-approve (not yet supported)"
		}

		checkbox := "[ ]"
		style := uncheckStyle
		if agent.yolo {
			checkbox = "[✓]"
			style = checkStyle
		}

		line := fmt.Sprintf("%s %s: %s", checkbox, agent.name, yoloLabel)
		if idx == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = style.Render("  " + line)
		}
		lines = append(lines, line)
		idx++
	}

	lines = append(lines, "")
	continueBtn := "  Continue →"
	if m.cursor == idx {
		continueBtn = selectedStyle.Render("> Continue →")
	}
	lines = append(lines, continueBtn)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content))
}

func (m onboardModel) viewConfirm() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	var lines []string
	lines = append(lines, titleStyle.Render("Review Configuration"))
	lines = append(lines, "")

	if m.editingCommands {
		// Edit mode: show text inputs for each command
		lines = append(lines, "Edit your agent commands:")
		lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("Tab/↑↓ to switch, Enter to confirm, Esc to cancel"))
		lines = append(lines, "")

		idx := 0
		for _, a := range m.agents {
			if !a.enabled {
				continue
			}
			if idx < len(m.commandInputs) {
				label := lipgloss.NewStyle().Bold(true).Render(a.name + ": ")
				input := m.commandInputs[idx].View()
				lines = append(lines, "  "+label+input)
			}
			idx++
		}
	} else {
		// Normal mode: show generated commands read-only
		agents := m.buildAgents()
		lines = append(lines, "Your agents window will contain:")
		lines = append(lines, "")
		for _, a := range agents {
			lines = append(lines, "  "+codeStyle.Render(a.Command))
		}
	}

	lines = append(lines, "")

	path, _ := config.GlobalConfigPath()
	lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("Config will be saved to:"))
	lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("  "+path))
	lines = append(lines, "")

	if !m.editingCommands {
		editCmdBtn := "  Edit Commands"
		saveBtn := "  Save & Continue"
		editBtn := "  Save & Edit"
		skipBtn := "  Skip (don't save)"
		if m.cursor == 0 {
			editCmdBtn = selectedStyle.Render("> Edit Commands")
		} else if m.cursor == 1 {
			saveBtn = selectedStyle.Render("> Save & Continue")
		} else if m.cursor == 2 {
			editBtn = selectedStyle.Render("> Save & Edit")
		} else {
			skipBtn = selectedStyle.Render("> Skip (don't save)")
		}
		lines = append(lines, editCmdBtn)
		lines = append(lines, saveBtn)
		lines = append(lines, editBtn)
		lines = append(lines, skipBtn)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content))
}

func (m onboardModel) viewKeybind() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	codeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2)

	var lines []string
	lines = append(lines, titleStyle.Render("Add tmux Keybinding?"))
	lines = append(lines, "")
	lines = append(lines, "Would you like to add a tmux keybinding for quick access?")
	lines = append(lines, "")
	lines = append(lines, "This will add to ~/.tmux.conf:")
	lines = append(lines, "  "+codeStyle.Render("bind-key S run-shell \"atmux browse\""))
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(dimColor).Render("Press prefix + S to open the session browser."))
	lines = append(lines, "")

	yesBtn := "  Yes, add keybinding"
	noBtn := "  No, skip"
	if m.cursor == 0 {
		yesBtn = selectedStyle.Render("> Yes, add keybinding")
	} else {
		noBtn = selectedStyle.Render("> No, skip")
	}
	lines = append(lines, yesBtn)
	lines = append(lines, noBtn)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		boxStyle.Render(content))
}

// addKeybinding adds the tmux keybinding to ~/.tmux.conf
// This reuses the logic from cmd/keybind.go
func (m *onboardModel) addKeybinding() error {
	// Get tmux config path
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	tmuxConfPath := filepath.Join(home, ".tmux.conf")

	// Build the binding line (default: prefix + S -> atmux browse)
	keybindKey := "S"
	keybindCommand := "browse"
	bindingLine := fmt.Sprintf("bind-key %s run-shell \"atmux %s\"", keybindKey, keybindCommand)
	commentLine := "# atmux: open session browser popup"
	fullBinding := fmt.Sprintf("\n%s\n%s\n", commentLine, bindingLine)

	// Read existing config (if any)
	existingContent := ""
	if _, err := os.Stat(tmuxConfPath); err == nil {
		content, err := os.ReadFile(tmuxConfPath)
		if err != nil {
			return fmt.Errorf("could not read %s: %w", tmuxConfPath, err)
		}
		existingContent = string(content)
	}

	// Check if exact binding already exists
	if strings.Contains(existingContent, bindingLine) {
		// Already exists, nothing to do
		return nil
	}

	// Check for duplicate bindings (warn but proceed in onboard)
	if isDuplicate, _ := findDuplicateKeybinding(existingContent, keybindKey); isDuplicate {
		// In onboard flow, we proceed anyway (user explicitly chose to add)
	}

	// Append to file
	f, err := os.OpenFile(tmuxConfPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("could not open %s for writing: %w", tmuxConfPath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(fullBinding); err != nil {
		return fmt.Errorf("could not write to %s: %w", tmuxConfPath, err)
	}

	return nil
}

// findDuplicateKeybinding checks if the key is already bound in the config
func findDuplicateKeybinding(content, key string) (bool, string) {
	// Match bind-key or bind followed by the key
	pattern := regexp.MustCompile(`(?m)^\s*bind(?:-key)?\s+` + regexp.QuoteMeta(key) + `\s+.*$`)
	match := pattern.FindString(content)
	if match != "" {
		return true, strings.TrimSpace(match)
	}
	return false, ""
}
