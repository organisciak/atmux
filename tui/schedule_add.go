package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/config"
	"github.com/porganisciak/agent-tmux/tmux"
)

// WizardStep represents a step in the schedule wizard
type WizardStep int

const (
	WizardStepSchedule WizardStep = iota
	WizardStepTarget
	WizardStepCommand
	WizardStepPreAction
	WizardStepConfirm
)

// scheduleWizardModel handles the add/edit flow for scheduled jobs
type scheduleWizardModel struct {
	// Current step
	step WizardStep

	// Schedule selection
	presets       []config.CronPreset
	presetIndex   int
	customCron    textinput.Model
	usingCustom   bool
	cronFields    [5]string // For custom cron entry
	cronFieldIdx  int
	cronValid     bool
	cronError     string

	// Target selection
	tree         *tmux.Tree
	flatNodes    []*tmux.TreeNode
	targetIndex  int
	targetExpand map[string]bool

	// Command input
	commandInput textinput.Model
	nameInput    textinput.Model

	// Pre-action
	preActions      []config.PreAction
	preActionIndex  int
	preActionLabels []string

	// Confirm step
	confirmFocusIdx int // 0=save, 1=cancel

	// State
	width     int
	height    int
	done      bool
	cancelled bool
	editingID string // non-empty if editing existing job
}

func newScheduleWizardModel(existingJob *config.ScheduledJob) *scheduleWizardModel {
	customInput := textinput.New()
	customInput.Placeholder = "* * * * * (min hour day month weekday)"
	customInput.CharLimit = 50
	customInput.Width = 40

	cmdInput := textinput.New()
	cmdInput.Placeholder = "Command to send..."
	cmdInput.CharLimit = 256
	cmdInput.Width = 50

	nameInput := textinput.New()
	nameInput.Placeholder = "Optional name for this job"
	nameInput.CharLimit = 50
	nameInput.Width = 40

	preActions := []config.PreAction{
		config.PreActionNone,
		config.PreActionCompact,
		config.PreActionNewSession,
	}
	preActionLabels := []string{
		"None - Send command directly",
		"Compact first - Run /compact before sending",
		"New session - Create new session first",
	}

	m := &scheduleWizardModel{
		step:            WizardStepSchedule,
		presets:         config.GetCronPresets(),
		presetIndex:     0,
		customCron:      customInput,
		cronFields:      [5]string{"*", "*", "*", "*", "*"},
		commandInput:    cmdInput,
		nameInput:       nameInput,
		preActions:      preActions,
		preActionLabels: preActionLabels,
		targetExpand:    make(map[string]bool),
	}

	// If editing, populate fields
	if existingJob != nil {
		m.editingID = existingJob.ID
		m.commandInput.SetValue(existingJob.Command)
		m.nameInput.SetValue(existingJob.Name)

		// Find matching preset or use custom
		found := false
		for i, p := range m.presets {
			if p.Expr == existingJob.CronExpr {
				m.presetIndex = i
				found = true
				break
			}
		}
		if !found {
			m.presetIndex = len(m.presets) - 1 // Custom
			m.usingCustom = true
			m.customCron.SetValue(existingJob.CronExpr)
			fields := strings.Fields(existingJob.CronExpr)
			if len(fields) == 5 {
				for i := 0; i < 5; i++ {
					m.cronFields[i] = fields[i]
				}
			}
		}

		// Find pre-action
		for i, pa := range m.preActions {
			if pa == existingJob.PreAction {
				m.preActionIndex = i
				break
			}
		}
	}

	return m
}

func (m scheduleWizardModel) Init() tea.Cmd {
	return tea.Batch(
		fetchTreeForWizard,
		textinput.Blink,
	)
}

// fetchTreeForWizard fetches the tmux tree for target selection
func fetchTreeForWizard() tea.Msg {
	tree, err := tmux.FetchTree()
	return wizardTreeMsg{tree: tree, err: err}
}

type wizardTreeMsg struct {
	tree *tmux.Tree
	err  error
}

func (m scheduleWizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case wizardTreeMsg:
		if msg.err == nil {
			m.tree = msg.tree
			m.rebuildFlatNodes()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Update text inputs if active
	if m.step == WizardStepSchedule && m.usingCustom {
		var cmd tea.Cmd
		m.customCron, cmd = m.customCron.Update(msg)
		cmds = append(cmds, cmd)
		m.validateCron()
	}
	if m.step == WizardStepCommand {
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		cmds = append(cmds, cmd)
		m.nameInput, cmd = m.nameInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *scheduleWizardModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "ctrl+c":
		m.done = true
		m.cancelled = true
		return m, nil
	case "esc":
		if m.step == WizardStepSchedule {
			m.done = true
			m.cancelled = true
			return m, nil
		}
		// Go back a step
		if m.step > WizardStepSchedule {
			m.step--
		}
		return m, nil
	}

	// Step-specific handling
	switch m.step {
	case WizardStepSchedule:
		return m.handleScheduleStep(msg)
	case WizardStepTarget:
		return m.handleTargetStep(msg)
	case WizardStepCommand:
		return m.handleCommandStep(msg)
	case WizardStepPreAction:
		return m.handlePreActionStep(msg)
	case WizardStepConfirm:
		return m.handleConfirmStep(msg)
	}

	return m, nil
}

func (m *scheduleWizardModel) handleScheduleStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.usingCustom {
		// In custom cron entry mode
		switch msg.String() {
		case "tab":
			m.cronFieldIdx = (m.cronFieldIdx + 1) % 5
			return m, nil
		case "shift+tab":
			m.cronFieldIdx = (m.cronFieldIdx + 4) % 5
			return m, nil
		case "up":
			m.incrementCronField(1)
			return m, nil
		case "down":
			m.incrementCronField(-1)
			return m, nil
		case "backspace":
			// Go back to preset selection
			m.usingCustom = false
			m.customCron.Blur()
			return m, nil
		case "enter":
			if m.cronValid {
				m.step = WizardStepTarget
			}
			return m, nil
		default:
			// Handle typing in current field
			if len(msg.String()) == 1 {
				char := msg.String()[0]
				if (char >= '0' && char <= '9') || char == '*' || char == '/' || char == '-' || char == ',' {
					if m.cronFields[m.cronFieldIdx] == "*" {
						m.cronFields[m.cronFieldIdx] = string(char)
					} else {
						m.cronFields[m.cronFieldIdx] += string(char)
					}
					m.validateCron()
				}
			}
		}
		return m, nil
	}

	// Preset selection mode
	switch msg.String() {
	case "up", "k":
		if m.presetIndex > 0 {
			m.presetIndex--
		}
		return m, nil
	case "down", "j":
		if m.presetIndex < len(m.presets)-1 {
			m.presetIndex++
		}
		return m, nil
	case "enter":
		if m.presets[m.presetIndex].Expr == "" {
			// Custom selected
			m.usingCustom = true
			m.customCron.Focus()
			return m, textinput.Blink
		}
		m.step = WizardStepTarget
		return m, nil
	}
	return m, nil
}

func (m *scheduleWizardModel) handleTargetStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.targetIndex > 0 {
			m.targetIndex--
		}
		return m, nil
	case "down", "j":
		if m.targetIndex < len(m.flatNodes)-1 {
			m.targetIndex++
		}
		return m, nil
	case " ":
		// Toggle expand
		if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
			node := m.flatNodes[m.targetIndex]
			if node.Type == "session" || node.Type == "window" {
				key := node.Type + ":" + node.Target
				m.targetExpand[key] = !m.targetExpand[key]
				m.rebuildFlatNodes()
			}
		}
		return m, nil
	case "enter":
		if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
			node := m.flatNodes[m.targetIndex]
			if node.Type == "pane" {
				// Pane selected, move to next step
				m.step = WizardStepCommand
				m.commandInput.Focus()
				return m, textinput.Blink
			}
			// Toggle expand for non-panes
			key := node.Type + ":" + node.Target
			m.targetExpand[key] = !m.targetExpand[key]
			m.rebuildFlatNodes()
		}
		return m, nil
	}
	return m, nil
}

func (m *scheduleWizardModel) handleCommandStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Toggle between name and command input
		if m.commandInput.Focused() {
			m.commandInput.Blur()
			m.nameInput.Focus()
		} else {
			m.nameInput.Blur()
			m.commandInput.Focus()
		}
		return m, textinput.Blink
	case "enter":
		if m.commandInput.Value() != "" {
			m.commandInput.Blur()
			m.nameInput.Blur()
			m.step = WizardStepPreAction
		}
		return m, nil
	}
	// Update text input
	var cmd tea.Cmd
	if m.commandInput.Focused() {
		m.commandInput, cmd = m.commandInput.Update(msg)
	} else {
		m.nameInput, cmd = m.nameInput.Update(msg)
	}
	return m, cmd
}

func (m *scheduleWizardModel) handlePreActionStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.preActionIndex > 0 {
			m.preActionIndex--
		}
		return m, nil
	case "down", "j":
		if m.preActionIndex < len(m.preActions)-1 {
			m.preActionIndex++
		}
		return m, nil
	case "enter":
		m.step = WizardStepConfirm
		return m, nil
	}
	return m, nil
}

func (m *scheduleWizardModel) handleConfirmStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		m.confirmFocusIdx = 0
		return m, nil
	case "right", "l":
		m.confirmFocusIdx = 1
		return m, nil
	case "tab":
		m.confirmFocusIdx = (m.confirmFocusIdx + 1) % 2
		return m, nil
	case "enter":
		m.done = true
		m.cancelled = m.confirmFocusIdx == 1
		return m, nil
	case "s":
		m.done = true
		m.cancelled = false
		return m, nil
	case "c":
		m.done = true
		m.cancelled = true
		return m, nil
	}
	return m, nil
}

func (m *scheduleWizardModel) validateCron() {
	expr := strings.Join(m.cronFields[:], " ")
	if err := config.ParseCron(expr); err != nil {
		m.cronValid = false
		m.cronError = err.Error()
	} else {
		m.cronValid = true
		m.cronError = ""
	}
}

func (m *scheduleWizardModel) incrementCronField(delta int) {
	field := m.cronFields[m.cronFieldIdx]
	if field == "*" {
		if delta > 0 {
			m.cronFields[m.cronFieldIdx] = "0"
		}
		return
	}

	// Try to parse as number
	var num int
	_, err := fmt.Sscanf(field, "%d", &num)
	if err != nil {
		return
	}

	num += delta
	fieldInfo := config.CronField{}
	switch m.cronFieldIdx {
	case 0:
		fieldInfo = config.CronField{Name: "minute", Min: 0, Max: 59}
	case 1:
		fieldInfo = config.CronField{Name: "hour", Min: 0, Max: 23}
	case 2:
		fieldInfo = config.CronField{Name: "day", Min: 1, Max: 31}
	case 3:
		fieldInfo = config.CronField{Name: "month", Min: 1, Max: 12}
	case 4:
		fieldInfo = config.CronField{Name: "weekday", Min: 0, Max: 6}
	}

	if num < fieldInfo.Min {
		num = fieldInfo.Max
	} else if num > fieldInfo.Max {
		num = fieldInfo.Min
	}

	m.cronFields[m.cronFieldIdx] = fmt.Sprintf("%d", num)
	m.validateCron()
}

func (m *scheduleWizardModel) rebuildFlatNodes() {
	if m.tree == nil {
		m.flatNodes = nil
		return
	}

	var nodes []*tmux.TreeNode
	for _, sess := range m.tree.Sessions {
		sessKey := "session:" + sess.Name
		sessExpanded := m.targetExpand[sessKey]

		sessNode := &tmux.TreeNode{
			Type:     "session",
			Name:     sess.Name,
			Target:   sess.Name,
			Expanded: sessExpanded,
			Level:    0,
		}
		nodes = append(nodes, sessNode)

		if sessExpanded {
			for _, win := range sess.Windows {
				winTarget := fmt.Sprintf("%s:%d", sess.Name, win.Index)
				winKey := "window:" + winTarget
				winExpanded := m.targetExpand[winKey]

				winNode := &tmux.TreeNode{
					Type:     "window",
					Name:     win.Name,
					Target:   winTarget,
					Expanded: winExpanded,
					Level:    1,
				}
				nodes = append(nodes, winNode)

				if winExpanded {
					for _, pane := range win.Panes {
						paneNode := &tmux.TreeNode{
							Type:   "pane",
							Name:   pane.Title,
							Target: pane.Target,
							Level:  2,
						}
						if paneNode.Name == "" {
							paneNode.Name = pane.Command
						}
						if paneNode.Name == "" {
							paneNode.Name = fmt.Sprintf("pane %d", pane.Index)
						}
						nodes = append(nodes, paneNode)
					}
				}
			}
		}
	}
	m.flatNodes = nodes
}

func (m *scheduleWizardModel) buildJob() config.ScheduledJob {
	var cronExpr string
	if m.usingCustom {
		cronExpr = strings.Join(m.cronFields[:], " ")
	} else {
		cronExpr = m.presets[m.presetIndex].Expr
	}

	var target string
	if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
		target = m.flatNodes[m.targetIndex].Target
	}

	return config.ScheduledJob{
		ID:        m.editingID,
		Name:      m.nameInput.Value(),
		CronExpr:  cronExpr,
		Target:    target,
		Command:   m.commandInput.Value(),
		PreAction: m.preActions[m.preActionIndex],
		Enabled:   true,
	}
}

func (m scheduleWizardModel) View() string {
	var sections []string

	// Title
	editMode := "Add"
	if m.editingID != "" {
		editMode = "Edit"
	}
	title := schedTitleStyle.Render(fmt.Sprintf("%s Scheduled Job", editMode))
	sections = append(sections, title)

	// Step indicator
	steps := []string{"Schedule", "Target", "Command", "Pre-action", "Confirm"}
	stepIndicator := m.renderStepIndicator(steps)
	sections = append(sections, stepIndicator)
	sections = append(sections, "")

	// Current step content
	switch m.step {
	case WizardStepSchedule:
		sections = append(sections, m.renderScheduleStep()...)
	case WizardStepTarget:
		sections = append(sections, m.renderTargetStep()...)
	case WizardStepCommand:
		sections = append(sections, m.renderCommandStep()...)
	case WizardStepPreAction:
		sections = append(sections, m.renderPreActionStep()...)
	case WizardStepConfirm:
		sections = append(sections, m.renderConfirmStep()...)
	}

	// Navigation hint
	sections = append(sections, "")
	sections = append(sections, schedHintStyle.Render("[Esc] back [Enter] next"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m scheduleWizardModel) renderStepIndicator(steps []string) string {
	var parts []string
	for i, step := range steps {
		style := schedHintStyle
		if i == int(m.step) {
			style = schedTitleStyle
		}
		parts = append(parts, style.Render(step))
		if i < len(steps)-1 {
			parts = append(parts, schedHintStyle.Render(" > "))
		}
	}
	return strings.Join(parts, "")
}

func (m scheduleWizardModel) renderScheduleStep() []string {
	var lines []string

	subtitle := wizSubtitleStyle.Render("When should this command run?")
	lines = append(lines, subtitle)
	lines = append(lines, "")

	if m.usingCustom {
		// Custom cron entry
		lines = append(lines, "Enter custom cron expression:")
		lines = append(lines, "")

		// Field headers
		headers := []string{"Minute", "Hour", "Day", "Month", "Weekday"}
		var headerRow []string
		for i, h := range headers {
			style := wizCronHeaderStyle
			if i == m.cronFieldIdx {
				style = wizCronHeaderFocusStyle
			}
			headerRow = append(headerRow, style.Render(h))
		}
		lines = append(lines, strings.Join(headerRow, " "))

		// Field values
		var fieldRow []string
		for i, f := range m.cronFields {
			style := wizCronFieldStyle
			if i == m.cronFieldIdx {
				style = wizCronFieldFocusStyle
			}
			fieldRow = append(fieldRow, style.Render(f))
		}
		lines = append(lines, strings.Join(fieldRow, " "))

		// Field ranges
		ranges := []string{"0-59", "0-23", "1-31", "1-12", "0-6"}
		var rangeRow []string
		for i, r := range ranges {
			style := wizCronRangeStyle
			if i == m.cronFieldIdx {
				style = wizCronRangeFocusStyle
			}
			rangeRow = append(rangeRow, style.Render(r))
		}
		lines = append(lines, strings.Join(rangeRow, " "))

		lines = append(lines, "")

		// Preview
		expr := strings.Join(m.cronFields[:], " ")
		if m.cronValid {
			english := config.CronToEnglish(expr)
			nextRun := config.FormatNextRun(expr)
			lines = append(lines, wizPreviewOKStyle.Render("Preview: "+english))
			lines = append(lines, wizPreviewOKStyle.Render("Next run: "+nextRun))
		} else {
			lines = append(lines, wizPreviewErrStyle.Render("Error: "+m.cronError))
		}

		lines = append(lines, "")
		lines = append(lines, wizRefStyle.Render("[Tab] switch field [Up/Down] adjust [Backspace] go back"))
	} else {
		// Preset selection
		for i, preset := range m.presets {
			var row string
			if i == m.presetIndex {
				row = selectedStyle.Render("> ") + lipgloss.NewStyle().Bold(true).Render(preset.Name)
			} else {
				row = "  " + preset.Name
			}
			lines = append(lines, row)
			if i == m.presetIndex {
				lines = append(lines, "    "+wizSubtitleStyle.Render(preset.Description))
			}
		}
	}

	return lines
}

func (m scheduleWizardModel) renderTargetStep() []string {
	var lines []string

	subtitle := wizSubtitleStyle.Render("Which pane should receive the command?")
	lines = append(lines, subtitle)
	lines = append(lines, "")

	if len(m.flatNodes) == 0 {
		lines = append(lines, schedHintStyle.Render("No tmux sessions found. Start a tmux session first."))
		return lines
	}

	// Tree view
	maxDisplay := 15
	for i, node := range m.flatNodes {
		if i >= maxDisplay {
			lines = append(lines, schedHintStyle.Render(fmt.Sprintf("... and %d more", len(m.flatNodes)-maxDisplay)))
			break
		}

		indent := strings.Repeat("  ", node.Level)
		icon := getNodeIcon(node.Type, node.Expanded, node.Active)
		name := node.Name

		var row string
		if i == m.targetIndex {
			row = selectedStyle.Render("> " + indent + icon + " " + name)
		} else {
			row = "  " + indent + icon + " " + name
		}

		// Highlight panes differently
		if node.Type == "pane" {
			if i == m.targetIndex {
				row += schedTargetStyle.Render(" <- select this")
			}
		}

		lines = append(lines, row)
	}

	lines = append(lines, "")
	lines = append(lines, wizRefStyle.Render("[Space/Enter] expand [Enter on pane] select"))

	return lines
}

func (m scheduleWizardModel) renderCommandStep() []string {
	var lines []string

	subtitle := wizSubtitleStyle.Render("What command should be sent?")
	lines = append(lines, subtitle)
	lines = append(lines, "")

	// Command input
	cmdLabel := wizLabelStyle.Render("Command:")
	cmdStyle := wizInputStyle
	if m.commandInput.Focused() {
		cmdStyle = cmdStyle.BorderForeground(activeColor)
	}
	lines = append(lines, cmdLabel)
	lines = append(lines, cmdStyle.Render(m.commandInput.View()))
	lines = append(lines, "")

	// Optional name
	nameLabel := wizLabelStyle.Render("Name (optional):")
	nameStyle := wizInputStyle
	if m.nameInput.Focused() {
		nameStyle = nameStyle.BorderForeground(activeColor)
	}
	lines = append(lines, nameLabel)
	lines = append(lines, nameStyle.Render(m.nameInput.View()))

	lines = append(lines, "")
	lines = append(lines, wizRefStyle.Render("[Tab] switch field [Enter] continue"))

	return lines
}

func (m scheduleWizardModel) renderPreActionStep() []string {
	var lines []string

	subtitle := wizSubtitleStyle.Render("What should happen before sending the command?")
	lines = append(lines, subtitle)
	lines = append(lines, "")

	for i, label := range m.preActionLabels {
		var row string
		if i == m.preActionIndex {
			row = selectedStyle.Render("> ") + lipgloss.NewStyle().Bold(true).Render(label)
		} else {
			row = "  " + label
		}
		lines = append(lines, row)
	}

	return lines
}

func (m scheduleWizardModel) renderConfirmStep() []string {
	var lines []string

	subtitle := wizSubtitleStyle.Render("Review and confirm")
	lines = append(lines, subtitle)
	lines = append(lines, "")

	// Summary box
	var cronExpr string
	if m.usingCustom {
		cronExpr = strings.Join(m.cronFields[:], " ")
	} else {
		cronExpr = m.presets[m.presetIndex].Expr
	}

	var target string
	if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
		target = m.flatNodes[m.targetIndex].Target
	}

	summaryLines := []string{
		wizLabelStyle.Render("Schedule: ") + wizValueStyle.Render(config.CronToEnglish(cronExpr)),
		wizLabelStyle.Render("Next run: ") + wizValueStyle.Render(config.FormatNextRun(cronExpr)),
		wizLabelStyle.Render("Target:   ") + wizValueStyle.Render(target),
		wizLabelStyle.Render("Command:  ") + wizValueStyle.Render(m.commandInput.Value()),
		wizLabelStyle.Render("Pre-action: ") + wizValueStyle.Render(m.preActionLabels[m.preActionIndex]),
	}

	if m.nameInput.Value() != "" {
		summaryLines = append([]string{
			wizLabelStyle.Render("Name:     ") + wizValueStyle.Render(m.nameInput.Value()),
		}, summaryLines...)
	}

	summary := wizBoxStyle.Render(strings.Join(summaryLines, "\n"))
	lines = append(lines, summary)
	lines = append(lines, "")

	// Save/Cancel buttons
	saveBtn := wizSaveBtnStyle.Render(" Save ")
	cancelBtn := wizCancelBtnStyle.Render(" Cancel ")

	if m.confirmFocusIdx == 0 {
		saveBtn = wizSaveBtnActiveStyle.Render(" Save ")
	} else {
		cancelBtn = wizCancelBtnActiveStyle.Render(" Cancel ")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn)
	lines = append(lines, buttons)

	return lines
}
