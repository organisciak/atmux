package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
)

// SchedulerOptions configures the scheduler TUI
type SchedulerOptions struct {
	AltScreen bool
}

// RunScheduler runs the scheduler management TUI
func RunScheduler(opts SchedulerOptions) error {
	m := newSchedulerModel()
	programOptions := []tea.ProgramOption{
		tea.WithMouseCellMotion(),
	}
	if opts.AltScreen {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, programOptions...)
	_, err := p.Run()
	return err
}

// schedulerModel is the main scheduler list model
type schedulerModel struct {
	jobs          []config.ScheduledJob
	width         int
	height        int
	selectedIndex int
	schedule      *config.Schedule
	lastError     error

	// Confirm delete state
	confirmDelete bool
	deleteJobID   string

	// Sub-model for add/edit wizard
	wizardActive bool
	wizard       *scheduleWizardModel
}

func newSchedulerModel() schedulerModel {
	return schedulerModel{
		selectedIndex: 0,
	}
}

// Init initializes the model
func (m schedulerModel) Init() tea.Cmd {
	return loadSchedule
}

// loadSchedule loads the schedule from disk
func loadSchedule() tea.Msg {
	schedule, err := config.LoadSchedule()
	return scheduleLoadedMsg{schedule: schedule, err: err}
}

// scheduleLoadedMsg is sent when schedule is loaded
type scheduleLoadedMsg struct {
	schedule *config.Schedule
	err      error
}

// jobDeletedMsg is sent after a job is deleted
type jobDeletedMsg struct {
	id  string
	err error
}

// jobToggledMsg is sent after a job is toggled
type jobToggledMsg struct {
	id  string
	err error
}

// Update handles messages
func (m schedulerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// If wizard is active, delegate to it
	if m.wizardActive {
		return m.updateWizard(msg)
	}

	switch msg := msg.(type) {
	case scheduleLoadedMsg:
		if msg.err != nil {
			m.lastError = msg.err
		} else {
			m.schedule = msg.schedule
			m.jobs = msg.schedule.SortedJobs()
		}
		m.clampSelection()
		return m, nil

	case jobDeletedMsg:
		if msg.err != nil {
			m.lastError = msg.err
		}
		return m, loadSchedule

	case jobToggledMsg:
		if msg.err != nil {
			m.lastError = msg.err
		}
		return m, loadSchedule

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

func (m schedulerModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle delete confirmation
	if m.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			m.confirmDelete = false
			id := m.deleteJobID
			return m, func() tea.Msg {
				schedule, _ := config.LoadSchedule()
				err := schedule.DeleteJob(id)
				return jobDeletedMsg{id: id, err: err}
			}
		case "n", "N", "esc":
			m.confirmDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.selectedIndex > 0 {
			m.selectedIndex--
		}
		return m, nil

	case "down", "j":
		if m.selectedIndex < len(m.jobs)-1 {
			m.selectedIndex++
		}
		return m, nil

	case "a":
		// Add new job
		m.wizardActive = true
		m.wizard = newScheduleWizardModel(nil)
		return m, m.wizard.Init()

	case "enter":
		// Edit selected job
		if m.selectedIndex >= 0 && m.selectedIndex < len(m.jobs) {
			job := m.jobs[m.selectedIndex]
			m.wizardActive = true
			m.wizard = newScheduleWizardModel(&job)
			return m, m.wizard.Init()
		}
		return m, nil

	case "e":
		// Toggle enabled
		if m.selectedIndex >= 0 && m.selectedIndex < len(m.jobs) {
			job := m.jobs[m.selectedIndex]
			return m, func() tea.Msg {
				schedule, _ := config.LoadSchedule()
				err := schedule.ToggleJob(job.ID)
				return jobToggledMsg{id: job.ID, err: err}
			}
		}
		return m, nil

	case "d", "x":
		// Delete job
		if m.selectedIndex >= 0 && m.selectedIndex < len(m.jobs) {
			m.confirmDelete = true
			m.deleteJobID = m.jobs[m.selectedIndex].ID
		}
		return m, nil
	}

	return m, nil
}

func (m schedulerModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		// Calculate which job was clicked
		headerHeight := 5 // title + subtitle + hints + separator + header
		if msg.Y >= headerHeight && msg.Y < headerHeight+len(m.jobs) {
			clicked := msg.Y - headerHeight
			if clicked >= 0 && clicked < len(m.jobs) {
				if m.selectedIndex == clicked {
					// Double-click to edit
					job := m.jobs[clicked]
					m.wizardActive = true
					m.wizard = newScheduleWizardModel(&job)
					return m, m.wizard.Init()
				}
				m.selectedIndex = clicked
			}
		}
	}
	return m, nil
}

func (m schedulerModel) updateWizard(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.wizard == nil {
		m.wizardActive = false
		return m, nil
	}

	newWizard, cmd := m.wizard.Update(msg)
	wizard := newWizard.(scheduleWizardModel)
	m.wizard = &wizard

	// Check if wizard is done
	if m.wizard.done {
		m.wizardActive = false
		if m.wizard.cancelled {
			return m, nil
		}
		// Save the job
		job := m.wizard.buildJob()
		return m, func() tea.Msg {
			schedule, err := config.LoadSchedule()
			if err != nil {
				return jobDeletedMsg{err: err}
			}
			if job.ID == "" {
				err = schedule.AddJob(job)
			} else {
				err = schedule.UpdateJob(job)
			}
			return scheduleLoadedMsg{schedule: schedule, err: err}
		}
	}

	return m, cmd
}

func (m *schedulerModel) clampSelection() {
	if m.selectedIndex >= len(m.jobs) {
		m.selectedIndex = len(m.jobs) - 1
	}
	if m.selectedIndex < 0 {
		m.selectedIndex = 0
	}
}

// View renders the scheduler
func (m schedulerModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// If wizard is active, show it instead
	if m.wizardActive && m.wizard != nil {
		return m.wizard.View()
	}

	var sections []string

	// Title
	title := schedTitleStyle.Render("Scheduled Commands")
	sections = append(sections, title)

	// Subtitle
	subtitle := lipgloss.NewStyle().Foreground(dimColor).Render("Manage commands sent to tmux panes on a schedule")
	sections = append(sections, subtitle)

	// Hints
	hints := schedHintStyle.Render("[a]dd [Enter]edit [e]nable/disable [d]elete [q]uit")
	sections = append(sections, hints)

	// Error display
	if m.lastError != nil {
		errStr := lipgloss.NewStyle().Foreground(errorColor).Render("Error: " + m.lastError.Error())
		sections = append(sections, errStr)
	}

	sections = append(sections, "")

	// Delete confirmation
	if m.confirmDelete {
		confirmBox := m.renderDeleteConfirm()
		sections = append(sections, confirmBox)
		sections = append(sections, "")
	}

	// Jobs list
	if len(m.jobs) == 0 {
		empty := lipgloss.NewStyle().Foreground(dimColor).Italic(true).Render("No scheduled jobs. Press 'a' to add one.")
		sections = append(sections, empty)
	} else {
		// Header row
		header := m.renderJobHeader()
		sections = append(sections, header)
		sections = append(sections, schedSeparatorStyle.Render(strings.Repeat("-", min(m.width-4, 100))))

		for i, job := range m.jobs {
			row := m.renderJobRow(job, i == m.selectedIndex)
			sections = append(sections, row)
		}
	}

	// Tips at bottom
	sections = append(sections, "")
	sections = append(sections, RenderTipForContext(TipScheduler))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m schedulerModel) renderDeleteConfirm() string {
	var jobName string
	for _, j := range m.jobs {
		if j.ID == m.deleteJobID {
			if j.Name != "" {
				jobName = j.Name
			} else {
				jobName = j.Command
			}
			break
		}
	}

	text := fmt.Sprintf("Delete job '%s'? [y/n]", truncate(jobName, 30))
	return schedConfirmStyle.Render(text)
}

func (m schedulerModel) renderJobHeader() string {
	statusCol := lipgloss.NewStyle().Width(8).Render("Status")
	schedCol := lipgloss.NewStyle().Width(20).Render("Schedule")
	targetCol := lipgloss.NewStyle().Width(20).Render("Target")
	commandCol := lipgloss.NewStyle().Width(30).Render("Command")
	nextCol := lipgloss.NewStyle().Width(15).Render("Next Run")

	return lipgloss.JoinHorizontal(lipgloss.Top, statusCol, schedCol, targetCol, commandCol, nextCol)
}

func (m schedulerModel) renderJobRow(job config.ScheduledJob, selected bool) string {
	// Status indicator
	var status string
	if job.Enabled {
		status = schedStatusActiveStyle.Render("[ON] ")
	} else {
		status = schedStatusDimStyle.Render("[OFF]")
	}
	statusCol := lipgloss.NewStyle().Width(8).Render(status)

	// Schedule description
	schedDesc := config.CronToEnglish(job.CronExpr)
	schedCol := lipgloss.NewStyle().Width(20).Render(truncate(schedDesc, 19))

	// Target
	targetCol := schedTargetStyle.Width(20).Render(truncate(job.Target, 19))

	// Command
	cmdDisplay := job.Command
	if job.Name != "" {
		cmdDisplay = job.Name + ": " + job.Command
	}
	commandCol := lipgloss.NewStyle().Width(30).Render(truncate(cmdDisplay, 29))

	// Next run
	nextRun := config.FormatNextRun(job.CronExpr)
	if !job.Enabled {
		nextRun = "-"
	}
	nextCol := lipgloss.NewStyle().Width(15).Render(nextRun)

	row := lipgloss.JoinHorizontal(lipgloss.Top, statusCol, schedCol, targetCol, commandCol, nextCol)

	if selected {
		return selectedStyle.Render("> ") + row
	}
	return "  " + row
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
