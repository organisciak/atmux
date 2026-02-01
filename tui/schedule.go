package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/scheduler"
)

// ScheduleResult contains the outcome of the schedule TUI
type ScheduleResult struct {
	Message string
}

// RunScheduleTUI runs the schedule management TUI
func RunScheduleTUI() (*ScheduleResult, error) {
	m := newScheduleModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(scheduleModel); ok {
		return &ScheduleResult{
			Message: model.message,
		}, nil
	}
	return &ScheduleResult{}, nil
}

type scheduleModel struct {
	jobs          []scheduler.ScheduledJob
	cursor        int
	width         int
	height        int
	message       string
	confirmDelete bool
	deleteID      string
	lastError     error
}

func newScheduleModel() scheduleModel {
	return scheduleModel{}
}

// jobsLoadedMsg is sent when jobs are loaded
type jobsLoadedMsg struct {
	jobs []scheduler.ScheduledJob
	err  error
}

func (m scheduleModel) Init() tea.Cmd {
	return func() tea.Msg {
		store, err := scheduler.Load()
		if err != nil {
			return jobsLoadedMsg{err: err}
		}
		return jobsLoadedMsg{jobs: store.Jobs}
	}
}

func (m scheduleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case jobsLoadedMsg:
		m.jobs = msg.jobs
		m.lastError = msg.err
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.MouseMsg:
		return m.handleMouseMsg(msg)
	}
	return m, nil
}

func (m scheduleModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle delete confirmation mode
	if m.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			return m.doDelete()
		case "n", "N", "esc":
			m.confirmDelete = false
			m.deleteID = ""
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.jobs)-1 {
			m.cursor++
		}
		return m, nil

	case "a":
		// Launch add wizard
		return m, m.launchAddWizard()

	case "d":
		// Start delete confirmation
		if len(m.jobs) > 0 {
			m.confirmDelete = true
			m.deleteID = m.jobs[m.cursor].ID
		}
		return m, nil

	case " ":
		// Toggle enable/disable
		if len(m.jobs) > 0 {
			return m.toggleJob()
		}
		return m, nil

	case "enter":
		// Edit job (future feature)
		return m, nil

	case "r":
		// Refresh job list
		return m, m.Init()
	}
	return m, nil
}

func (m scheduleModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	// Calculate which job was clicked based on Y position
	// Title takes ~3 lines, then each job takes ~4 lines
	startY := 4
	jobHeight := 4
	clickedIdx := (msg.Y - startY) / jobHeight

	if clickedIdx >= 0 && clickedIdx < len(m.jobs) {
		m.cursor = clickedIdx
	}

	return m, nil
}

func (m scheduleModel) launchAddWizard() tea.Cmd {
	return func() tea.Msg {
		// Run the add wizard in a subprogram
		result, err := RunScheduleAddTUI()
		if err != nil {
			return jobsLoadedMsg{err: err}
		}
		if result != nil && result.Added {
			// Reload jobs
			store, err := scheduler.Load()
			if err != nil {
				return jobsLoadedMsg{err: err}
			}
			return jobsLoadedMsg{jobs: store.Jobs}
		}
		// Reload anyway to refresh
		store, _ := scheduler.Load()
		return jobsLoadedMsg{jobs: store.Jobs}
	}
}

func (m scheduleModel) toggleJob() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.jobs) {
		return m, nil
	}

	job := m.jobs[m.cursor]
	store, err := scheduler.Load()
	if err != nil {
		m.lastError = err
		return m, nil
	}

	storedJob, err := store.GetByID(job.ID)
	if err != nil {
		m.lastError = err
		return m, nil
	}

	storedJob.Enabled = !storedJob.Enabled
	if storedJob.Enabled {
		// Recalculate next run
		nextRun, err := scheduler.NextRun(storedJob.Schedule)
		if err == nil {
			storedJob.NextRun = nextRun
		}
	}

	if err := store.Update(*storedJob); err != nil {
		m.lastError = err
		return m, nil
	}

	if err := store.Save(); err != nil {
		m.lastError = err
		return m, nil
	}

	// Update local copy
	m.jobs[m.cursor].Enabled = storedJob.Enabled
	m.jobs[m.cursor].NextRun = storedJob.NextRun

	return m, nil
}

func (m scheduleModel) doDelete() (tea.Model, tea.Cmd) {
	store, err := scheduler.Load()
	if err != nil {
		m.lastError = err
		m.confirmDelete = false
		return m, nil
	}

	if err := store.Remove(m.deleteID); err != nil {
		m.lastError = err
		m.confirmDelete = false
		return m, nil
	}

	if err := store.Save(); err != nil {
		m.lastError = err
		m.confirmDelete = false
		return m, nil
	}

	// Remove from local list
	for i, job := range m.jobs {
		if job.ID == m.deleteID {
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			break
		}
	}

	// Adjust cursor
	if m.cursor >= len(m.jobs) && m.cursor > 0 {
		m.cursor--
	}

	m.confirmDelete = false
	m.deleteID = ""
	m.message = "Schedule deleted."

	return m, nil
}

func (m scheduleModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Title
	title := m.renderTitle()
	sections = append(sections, title)

	// Error message
	if m.lastError != nil {
		sections = append(sections, wizPreviewErrStyle.Render("Error: "+m.lastError.Error()))
	}

	// Delete confirmation
	if m.confirmDelete {
		sections = append(sections, schedConfirmStyle.Render("Delete this schedule? (y/n)"))
	}

	// Job list
	if len(m.jobs) == 0 {
		emptyStyle := schedStatusDimStyle.Padding(1, 2)
		sections = append(sections, emptyStyle.Render("No scheduled commands.\nPress 'a' to add one."))
	} else {
		for i, job := range m.jobs {
			jobView := m.renderJob(job, i == m.cursor)
			sections = append(sections, jobView)
		}
	}

	// Status bar
	statusBar := m.renderStatusBar()
	sections = append(sections, statusBar)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m scheduleModel) renderTitle() string {
	titleStyle := schedTitleStyle.
		Width(m.width).
		Align(lipgloss.Center).
		Padding(1, 0)

	addBtn := schedAddBtnStyle.Render("[+ Add]")

	title := titleStyle.Render("Scheduled Commands")
	// Add button in corner
	return lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", addBtn)
}

func (m scheduleModel) renderJob(job scheduler.ScheduledJob, selected bool) string {
	// Status indicator
	status := "●"
	var statusStyle lipgloss.Style
	if !job.Enabled {
		status = "○"
		statusStyle = schedStatusDimStyle
	} else {
		statusStyle = schedStatusActiveStyle
	}

	// ID
	idStyle := schedIDStyle
	if !job.Enabled {
		idStyle = schedStatusDimStyle
	}

	// Schedule description
	scheduleDesc := scheduler.CronToEnglish(job.Schedule)
	schedStyle := lipgloss.NewStyle()
	if !job.Enabled {
		schedStyle = schedStatusDimStyle.Strikethrough(true)
	}

	// First line: status, ID, schedule
	line1 := fmt.Sprintf("%s %s  %s",
		statusStyle.Render(status),
		idStyle.Render(job.ID),
		schedStyle.Render(scheduleDesc))

	if !job.Enabled {
		line1 += schedStatusDimStyle.Render("  (disabled)")
	}

	// Second line: target
	line2 := fmt.Sprintf("  %s %s", schedTargetStyle.Render("→"), job.Target)

	// Third line: command
	cmdDesc := job.Command
	if job.PreAction != scheduler.PreActionNone {
		cmdDesc = fmt.Sprintf("/%s then %s", job.PreAction, job.Command)
	}
	line3 := fmt.Sprintf("  %s", cmdDesc)

	content := lipgloss.JoinVertical(lipgloss.Left, line1, line2, line3)

	// Box style
	boxStyle := borderStyle.
		Width(m.width - 4).
		Padding(0, 1)

	if selected {
		boxStyle = activeBorderStyle.
			Width(m.width - 4).
			Padding(0, 1)
	}

	return boxStyle.Render(content)
}

func (m scheduleModel) renderStatusBar() string {
	var hints []string
	if m.confirmDelete {
		hints = []string{"y confirm", "n cancel"}
	} else {
		hints = []string{"↑↓ select", "Space toggle", "a add", "d delete", "q quit"}
	}

	separator := schedSeparatorStyle.Render(" │ ")

	var styledHints []string
	for _, hint := range hints {
		styledHints = append(styledHints, schedHintStyle.Render(hint))
	}

	hintsLine := strings.Join(styledHints, separator)

	return statusBarStyle.
		Width(m.width).
		Align(lipgloss.Center).
		Render(hintsLine)
}
