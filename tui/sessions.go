package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/tmux"
)

type SessionsOptions struct {
	AltScreen bool
}

// RunSessionsList runs a simple session list UI that attaches on click.
func RunSessionsList(opts SessionsOptions) error {
	m := newSessionsModel()
	programOptions := []tea.ProgramOption{
		tea.WithMouseCellMotion(),
	}
	if opts.AltScreen {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, programOptions...)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if model, ok := finalModel.(sessionsModel); ok && model.attachSession != "" {
		return tmux.AttachToSession(model.attachSession)
	}
	return nil
}

type sessionsModel struct {
	lines         []tmux.SessionLine
	width         int
	height        int
	selectedIndex int
	attachSession string
	lastError     error
}

func newSessionsModel() sessionsModel {
	return sessionsModel{selectedIndex: 0}
}

func (m sessionsModel) Init() tea.Cmd {
	return func() tea.Msg {
		lines, err := tmux.ListSessionsRaw()
		return sessionsLoadedMsg{lines: lines, err: err}
	}
}

type sessionsLoadedMsg struct {
	lines []tmux.SessionLine
	err   error
}

func (m sessionsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoadedMsg:
		m.lines = msg.lines
		m.lastError = msg.err
		if m.selectedIndex >= len(m.lines) {
			m.selectedIndex = len(m.lines) - 1
		}
		if m.selectedIndex < 0 {
			m.selectedIndex = 0
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
			}
			return m, nil
		case "down", "j":
			if m.selectedIndex < len(m.lines)-1 {
				m.selectedIndex++
			}
			return m, nil
		case "enter":
			if m.selectedIndex >= 0 && m.selectedIndex < len(m.lines) {
				m.attachSession = m.lines[m.selectedIndex].Name
				return m, tea.Quit
			}
			return m, nil
		}
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			headerHeight := 2
			if msg.Y >= headerHeight && msg.Y < headerHeight+len(m.lines) {
				clicked := msg.Y - headerHeight
				if clicked >= 0 && clicked < len(m.lines) {
					m.selectedIndex = clicked
					m.attachSession = m.lines[clicked].Name
					return m, tea.Quit
				}
			}
		}
	}
	return m, nil
}

func (m sessionsModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	title := lipgloss.NewStyle().Bold(true).Render("tmux list-sessions (click to attach)")
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render("Up/Down to select, Enter to attach, q to quit")

	if m.lastError != nil {
		err := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", err)
	}

	if len(m.lines) == 0 {
		empty := lipgloss.NewStyle().Foreground(dimColor).Render("No active tmux sessions")
		return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", empty)
	}

	var rows []string
	for i, line := range m.lines {
		row := line.Line
		if i == m.selectedIndex {
			row = selectedStyle.Render(row)
		}
		rows = append(rows, row)
	}

	content := strings.Join(rows, "\n")
	body := lipgloss.NewStyle().Width(m.width - 2).Render(content)
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", body)
}
