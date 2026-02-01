package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/scheduler"
	"github.com/porganisciak/agent-tmux/tmux"
)

const (
	fieldSchedule = iota
	fieldTarget
	fieldCommand
	fieldPreAction
	fieldSave
)

type scheduleFormModel struct {
	width  int
	height int

	// Current focus
	currentField int

	// Schedule section
	scheduleMode   string // "preset" or "custom"
	presets        []scheduler.CronPreset
	presetCursor   int
	selectedCron   string
	cronFields     [5]string // [minute, hour, day, month, weekday]
	cronFieldFocus int

	// Target section
	tree           *tmux.Tree
	treeNodes      []*tmux.TreeNode
	treeExpanded   bool
	treeCursor     int
	selectedTarget string

	// Command section
	commandInput string

	// Pre-action section
	preActionCursor int
	preAction       scheduler.PreAction

	// Save button
	saveCursor int // 0=save, 1=cancel

	// Result
	added bool
	jobID string
}

func newScheduleFormModel() scheduleFormModel {
	presets := scheduler.CommonPresets()
	return scheduleFormModel{
		currentField:   fieldSchedule,
		scheduleMode:   "preset",
		presets:        presets,
		presetCursor:   0,
		cronFields:     [5]string{"0", "9", "*", "*", "*"},
		preAction:      scheduler.PreActionNone,
		treeExpanded:   false,
		saveCursor:     0,
	}
}

func (m scheduleFormModel) Init() tea.Cmd {
	return m.loadTree()
}

func (m scheduleFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case treeLoadedMsg:
		m.tree = msg.tree
		if m.tree != nil {
			m.treeNodes = m.tree.BuildTreeNodes()
		}
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

func (m scheduleFormModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c", "esc":
		return m, tea.Quit

	case "tab":
		m.currentField = (m.currentField + 1) % 5
		return m, nil

	case "shift+tab":
		m.currentField = (m.currentField + 4) % 5
		return m, nil
	}

	// Field-specific keys
	switch m.currentField {
	case fieldSchedule:
		return m.handleScheduleKey(msg)
	case fieldTarget:
		return m.handleTargetKey(msg)
	case fieldCommand:
		return m.handleCommandKey(msg)
	case fieldPreAction:
		return m.handlePreActionKey(msg)
	case fieldSave:
		return m.handleSaveKey(msg)
	}

	return m, nil
}

func (m scheduleFormModel) handleScheduleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.scheduleMode == "preset" {
		switch key {
		case "up", "k":
			if m.presetCursor > 0 {
				m.presetCursor--
			}
			return m, nil

		case "down", "j":
			if m.presetCursor < len(m.presets)-1 {
				m.presetCursor++
			}
			return m, nil

		case "enter", " ":
			m.selectedCron = m.presets[m.presetCursor].Expression
			return m, nil

		case "c":
			// Switch to custom mode
			m.scheduleMode = "custom"
			m.cronFieldFocus = 0
			return m, nil
		}
	} else {
		// Custom cron mode
		// Prioritize input first
		if len(key) == 1 && (key[0] >= '0' && key[0] <= '9' ||
			key[0] == '*' || key[0] == '/' || key[0] == '-' || key[0] == ',') {
			m.cronFields[m.cronFieldFocus] += key
			// Update selected cron
			m.selectedCron = scheduler.BuildCronExpression(
				m.cronFields[0], m.cronFields[1], m.cronFields[2],
				m.cronFields[3], m.cronFields[4])
			return m, nil
		}

		switch key {
		case "left", "h":
			m.cronFieldFocus = (m.cronFieldFocus + 4) % 5
			return m, nil

		case "right", "l":
			m.cronFieldFocus = (m.cronFieldFocus + 1) % 5
			return m, nil

		case "backspace":
			if len(m.cronFields[m.cronFieldFocus]) > 0 {
				m.cronFields[m.cronFieldFocus] = m.cronFields[m.cronFieldFocus][:len(m.cronFields[m.cronFieldFocus])-1]
				m.selectedCron = scheduler.BuildCronExpression(
					m.cronFields[0], m.cronFields[1], m.cronFields[2],
					m.cronFields[3], m.cronFields[4])
			}
			return m, nil

		case "p":
			// Switch back to preset mode
			m.scheduleMode = "preset"
			return m, nil
		}
	}

	return m, nil
}

func (m scheduleFormModel) handleTargetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if !m.treeExpanded {
		// Tree collapsed - show selected target
		if key == "enter" || key == " " {
			m.treeExpanded = true
		}
		return m, nil
	}

	// Tree expanded - allow navigation
	switch key {
	case "up", "k":
		if m.treeCursor > 0 {
			m.treeCursor--
		}
		return m, nil

	case "down", "j":
		if m.treeCursor < len(m.treeNodes)-1 {
			m.treeCursor++
		}
		return m, nil

	case " ":
		// Toggle expand/collapse
		if m.treeCursor < len(m.treeNodes) {
			node := m.treeNodes[m.treeCursor]
			if node.Type == "session" || node.Type == "window" {
				node.Expanded = !node.Expanded
				if m.tree != nil {
					m.treeNodes = buildTreeNodesWithState(m.tree, m.treeNodes)
				}
			}
		}
		return m, nil

	case "enter":
		// Select pane and collapse tree
		if m.treeCursor < len(m.treeNodes) {
			node := m.treeNodes[m.treeCursor]
			if node.Type == "pane" {
				m.selectedTarget = node.Target
				m.treeExpanded = false
			}
		}
		return m, nil

	case "esc":
		m.treeExpanded = false
		return m, nil
	}

	return m, nil
}

func (m scheduleFormModel) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Prioritize text input FIRST
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		m.commandInput += key
		return m, nil
	} else if key == "space" {
		m.commandInput += " "
		return m, nil
	}

	// Only handle special keys if not a printable character
	switch key {
	case "backspace":
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		return m, nil
	}

	return m, nil
}

func (m scheduleFormModel) handlePreActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "up", "k":
		if m.preActionCursor > 0 {
			m.preActionCursor--
		}
		return m, nil

	case "down", "j":
		if m.preActionCursor < 2 {
			m.preActionCursor++
		}
		return m, nil

	case "enter", " ":
		switch m.preActionCursor {
		case 0:
			m.preAction = scheduler.PreActionNone
		case 1:
			m.preAction = scheduler.PreActionCompact
		case 2:
			m.preAction = scheduler.PreActionNew
		}
		return m, nil
	}

	return m, nil
}

func (m scheduleFormModel) handleSaveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "left", "h":
		m.saveCursor = 0
		return m, nil

	case "right", "l":
		m.saveCursor = 1
		return m, nil

	case "enter":
		if m.saveCursor == 0 {
			// Validate and save
			if m.selectedCron == "" || m.selectedTarget == "" || m.commandInput == "" {
				// Invalid - don't save
				return m, nil
			}
			return m.saveSchedule()
		}
		// Cancel
		return m, tea.Quit
	}

	return m, nil
}

func (m scheduleFormModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// TODO: Add mouse handling for form fields
	return m, nil
}

func (m scheduleFormModel) loadTree() tea.Cmd {
	return func() tea.Msg {
		tree, err := tmux.FetchTree()
		return treeLoadedMsg{tree: tree, err: err}
	}
}

func (m scheduleFormModel) saveSchedule() (tea.Model, tea.Cmd) {
	store, err := scheduler.Load()
	if err != nil {
		return m, tea.Quit
	}

	job := scheduler.ScheduledJob{
		ID:        scheduler.GenerateID(),
		Schedule:  m.selectedCron,
		Target:    m.selectedTarget,
		Command:   m.commandInput,
		PreAction: m.preAction,
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	if err := store.Add(job); err != nil {
		return m, tea.Quit
	}

	if err := store.Save(); err != nil {
		return m, tea.Quit
	}

	m.added = true
	m.jobID = job.ID
	return m, tea.Quit
}

func (m scheduleFormModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Title
	title := schedTitleStyle.Render("Add Scheduled Command")
	sections = append(sections, title)
	sections = append(sections, "")

	// Schedule section
	sections = append(sections, m.renderScheduleSection())
	sections = append(sections, "")

	// Target section
	sections = append(sections, m.renderTargetSection())
	sections = append(sections, "")

	// Command section
	sections = append(sections, m.renderCommandSection())
	sections = append(sections, "")

	// Pre-action section
	sections = append(sections, m.renderPreActionSection())
	sections = append(sections, "")

	// Next run preview
	if m.selectedCron != "" {
		nextRun, err := scheduler.NextRun(m.selectedCron)
		if err == nil {
			preview := wizLabelStyle.Render("Next run: ") + wizValueStyle.Render(nextRun.Format("Mon Jan 2 at 3:04 PM"))
			sections = append(sections, preview)
			sections = append(sections, "")
		}
	}

	// Save/Cancel buttons
	sections = append(sections, m.renderButtons())

	// Status bar
	sections = append(sections, "")
	sections = append(sections, m.renderStatusBar())

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, content)
}

func (m scheduleFormModel) renderScheduleSection() string {
	focused := m.currentField == fieldSchedule

	label := "Schedule:"
	if focused {
		label = selectedStyle.Render(label)
	} else {
		label = wizLabelStyle.Render(label)
	}

	var content string
	if m.scheduleMode == "preset" {
		// Show selected preset or cursor
		selectedLabel := m.presets[m.presetCursor].Label
		if m.selectedCron != "" {
			// Find the preset that matches
			for _, p := range m.presets {
				if p.Expression == m.selectedCron {
					selectedLabel = p.Label
					break
				}
			}
		}

		if focused {
			// Show list
			var presetLines []string
			for i, p := range m.presets {
				line := "  " + p.Label
				if i == m.presetCursor {
					line = selectedStyle.Render("> " + p.Label)
				}
				presetLines = append(presetLines, line)
			}
			presetLines = append(presetLines, wizSubtitleStyle.Render("  (press 'c' for custom)"))
			content = lipgloss.JoinVertical(lipgloss.Left, presetLines...)
		} else {
			content = wizValueStyle.Render(selectedLabel)
		}
	} else {
		// Custom cron mode
		if focused {
			// Show field editor
			fieldNames := []string{"Min", "Hour", "Day", "Mon", "Wkdy"}
			var fields []string
			for i, val := range m.cronFields {
				var style lipgloss.Style
				if i == m.cronFieldFocus {
					style = wizCronFieldFocusStyle
				} else {
					style = wizCronFieldStyle
				}
				if val == "" {
					val = "*"
				}
				fields = append(fields, style.Render(fieldNames[i]+":"+val))
			}
			content = lipgloss.JoinHorizontal(lipgloss.Left, fields...)
			content += "\n" + wizSubtitleStyle.Render("  "+scheduler.CronToEnglish(m.selectedCron))
			content += "\n" + wizSubtitleStyle.Render("  (press 'p' for presets)")
		} else {
			content = wizValueStyle.Render(scheduler.CronToEnglish(m.selectedCron))
		}
	}

	return label + " " + content
}

func (m scheduleFormModel) renderTargetSection() string {
	focused := m.currentField == fieldTarget

	label := "Target:"
	if focused {
		label = selectedStyle.Render(label)
	} else {
		label = wizLabelStyle.Render(label)
	}

	var content string
	if m.selectedTarget != "" {
		content = wizValueStyle.Render(m.selectedTarget)
	} else {
		content = wizSubtitleStyle.Render("(not set)")
	}

	if focused && m.treeExpanded {
		// Show tree
		var treeLines []string
		for i, node := range m.treeNodes {
			indent := strings.Repeat("  ", node.Level)
			icon := ""
			var style lipgloss.Style

			switch node.Type {
			case "session":
				if node.Expanded {
					icon = "▼ "
				} else {
					icon = "▶ "
				}
				style = sessionStyle
			case "window":
				if node.Expanded {
					icon = "  ▼ "
				} else {
					icon = "  ▶ "
				}
				style = windowStyle
			case "pane":
				icon = "    > "
				style = paneStyle
			}

			line := indent + icon + node.Name
			if i == m.treeCursor {
				line = selectedStyle.Render(line)
			} else {
				line = style.Render(line)
			}
			treeLines = append(treeLines, "  "+line)
		}
		content += "\n" + lipgloss.JoinVertical(lipgloss.Left, treeLines...)
	} else if focused {
		content += wizSubtitleStyle.Render(" (press Enter to select)")
	}

	return label + " " + content
}

func (m scheduleFormModel) renderCommandSection() string {
	focused := m.currentField == fieldCommand

	label := "Command:"
	if focused {
		label = selectedStyle.Render(label)
	} else {
		label = wizLabelStyle.Render(label)
	}

	var content string
	if m.commandInput != "" {
		content = wizValueStyle.Render(m.commandInput)
		if focused {
			content += "_"
		}
	} else {
		if focused {
			content = wizSubtitleStyle.Render("(type command...)")
		} else {
			content = wizSubtitleStyle.Render("(not set)")
		}
	}

	return label + " " + content
}

func (m scheduleFormModel) renderPreActionSection() string {
	focused := m.currentField == fieldPreAction

	label := "Pre-Action:"
	if focused {
		label = selectedStyle.Render(label)
	} else {
		label = wizLabelStyle.Render(label)
	}

	options := []struct {
		label  string
		action scheduler.PreAction
	}{
		{"None", scheduler.PreActionNone},
		{"Compact first", scheduler.PreActionCompact},
		{"New session first", scheduler.PreActionNew},
	}

	var content string
	if focused {
		var optLines []string
		for i, opt := range options {
			line := "  " + opt.label
			if i == m.preActionCursor {
				line = selectedStyle.Render("> " + opt.label)
			}
			optLines = append(optLines, line)
		}
		content = "\n" + lipgloss.JoinVertical(lipgloss.Left, optLines...)
	} else {
		// Show selected pre-action
		selectedLabel := "None"
		for _, opt := range options {
			if opt.action == m.preAction {
				selectedLabel = opt.label
				break
			}
		}
		content = wizValueStyle.Render(selectedLabel)
	}

	return label + " " + content
}

func (m scheduleFormModel) renderButtons() string {
	focused := m.currentField == fieldSave

	saveLabel := "Save"
	cancelLabel := "Cancel"

	var saveStyle, cancelStyle lipgloss.Style
	if focused {
		if m.saveCursor == 0 {
			saveStyle = wizSaveBtnActiveStyle
			cancelStyle = wizCancelBtnStyle
		} else {
			saveStyle = wizSaveBtnInactiveStyle
			cancelStyle = wizCancelBtnActiveStyle
		}
	} else {
		saveStyle = wizSaveBtnStyle
		cancelStyle = wizCancelBtnStyle
	}

	return saveStyle.Render("  "+saveLabel+"  ") + "  " + cancelStyle.Render("  "+cancelLabel+"  ")
}

func (m scheduleFormModel) renderStatusBar() string {
	fieldNames := []string{"Schedule", "Target", "Command", "Pre-Action", "Save"}
	hints := "Tab to navigate │ Current field: " + selectedStyle.Render(fieldNames[m.currentField])
	return schedHintStyle.Render(hints)
}

// RunScheduleFormTUI runs the single-screen form for adding schedules
func RunScheduleFormTUI() (*ScheduleAddResult, error) {
	m := newScheduleFormModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(scheduleFormModel); ok {
		return &ScheduleAddResult{
			Added: model.added,
			JobID: model.jobID,
		}, nil
	}
	return &ScheduleAddResult{}, nil
}
