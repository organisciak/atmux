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

// FormField identifies which section of the form is focused
type FormField int

const (
	FieldSchedule FormField = iota
	FieldTarget
	FieldCommand
	FieldName
	FieldPreAction
	FieldButtons
)

// scheduleWizardModel handles the add/edit flow for scheduled jobs as a
// single-screen form.  All fields are visible simultaneously; Tab/Shift-Tab
// moves focus between sections.
type scheduleWizardModel struct {
	// Focus management
	focusedField FormField

	// Schedule selection
	presets      []config.CronPreset
	presetIndex  int
	usingCustom  bool
	cronFields   [5]string
	cronFieldIdx int
	cronValid    bool
	cronError    string

	// Target selection
	tree           *tmux.Tree
	flatNodes      []*tmux.TreeNode
	targetIndex    int
	targetExpand   map[string]bool
	selectedTarget string // stored target string for display when unfocused

	// Command input
	commandInput textinput.Model
	nameInput    textinput.Model

	// Pre-action
	preActions      []config.PreAction
	preActionIndex  int
	preActionLabels []string

	// Buttons
	buttonFocusIdx int // 0=save, 1=cancel

	// State
	width     int
	height    int
	done      bool
	cancelled bool
	editingID string // non-empty if editing existing job
}

func newScheduleWizardModel(existingJob *config.ScheduledJob) *scheduleWizardModel {
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
		focusedField:    FieldSchedule,
		presets:         config.GetCronPresets(),
		presetIndex:     0,
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

		// Store the target for display
		m.selectedTarget = existingJob.Target
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
			// If we have a pre-selected target (editing), find it in the tree
			if m.selectedTarget != "" {
				m.selectTargetByString(m.selectedTarget)
			}
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Update text inputs if they are focused
	if m.focusedField == FieldCommand && m.commandInput.Focused() {
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		cmds = append(cmds, cmd)
	}
	if m.focusedField == FieldName && m.nameInput.Focused() {
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// selectTargetByString expands the tree to reveal and select a target pane
func (m *scheduleWizardModel) selectTargetByString(target string) {
	if m.tree == nil || target == "" {
		return
	}
	// Expand all sessions and windows to find the target
	for _, sess := range m.tree.Sessions {
		sessKey := "session:" + sess.Name
		for _, win := range sess.Windows {
			winTarget := fmt.Sprintf("%s:%d", sess.Name, win.Index)
			for _, pane := range win.Panes {
				if pane.Target == target {
					m.targetExpand[sessKey] = true
					winKey := "window:" + winTarget
					m.targetExpand[winKey] = true
					m.rebuildFlatNodes()
					// Find the pane in flatNodes
					for i, node := range m.flatNodes {
						if node.Type == "pane" && node.Target == target {
							m.targetIndex = i
							return
						}
					}
					return
				}
			}
		}
	}
}

func (m *scheduleWizardModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c":
		m.done = true
		m.cancelled = true
		return *m, nil
	case "esc":
		// If in a text input, blur it and stay in current field
		if m.focusedField == FieldCommand && m.commandInput.Focused() {
			m.commandInput.Blur()
			return *m, nil
		}
		if m.focusedField == FieldName && m.nameInput.Focused() {
			m.nameInput.Blur()
			return *m, nil
		}
		// Otherwise cancel
		m.done = true
		m.cancelled = true
		return *m, nil
	}

	// Tab / Shift-Tab for section navigation (except when in custom cron mode
	// where tab cycles cron fields)
	if key == "tab" && !(m.focusedField == FieldSchedule && m.usingCustom) {
		m.blurInputs()
		m.focusedField++
		if m.focusedField > FieldButtons {
			m.focusedField = FieldSchedule
		}
		m.onFieldFocus()
		return *m, m.focusCmd()
	}
	if key == "shift+tab" && !(m.focusedField == FieldSchedule && m.usingCustom) {
		m.blurInputs()
		if m.focusedField == FieldSchedule {
			m.focusedField = FieldButtons
		} else {
			m.focusedField--
		}
		m.onFieldFocus()
		return *m, m.focusCmd()
	}

	// Delegate to section-specific handlers
	switch m.focusedField {
	case FieldSchedule:
		return m.handleScheduleField(msg)
	case FieldTarget:
		return m.handleTargetField(msg)
	case FieldCommand:
		return m.handleCommandField(msg)
	case FieldName:
		return m.handleNameField(msg)
	case FieldPreAction:
		return m.handlePreActionField(msg)
	case FieldButtons:
		return m.handleButtonsField(msg)
	}

	return *m, nil
}

// blurInputs blurs all text inputs
func (m *scheduleWizardModel) blurInputs() {
	m.commandInput.Blur()
	m.nameInput.Blur()
}

// onFieldFocus is called when a field gains focus
func (m *scheduleWizardModel) onFieldFocus() {
	switch m.focusedField {
	case FieldCommand:
		m.commandInput.Focus()
	case FieldName:
		m.nameInput.Focus()
	case FieldTarget:
		// Update selectedTarget from current tree selection
		m.updateSelectedTarget()
	}
}

// focusCmd returns the appropriate tea.Cmd for the newly focused field
func (m *scheduleWizardModel) focusCmd() tea.Cmd {
	if m.focusedField == FieldCommand || m.focusedField == FieldName {
		return textinput.Blink
	}
	return nil
}

// updateSelectedTarget stores the currently selected target string
func (m *scheduleWizardModel) updateSelectedTarget() {
	if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
		node := m.flatNodes[m.targetIndex]
		if node.Type == "pane" {
			m.selectedTarget = node.Target
		}
	}
}

// --- Schedule field ---

func (m *scheduleWizardModel) handleScheduleField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.usingCustom {
		switch key {
		case "tab":
			m.cronFieldIdx = (m.cronFieldIdx + 1) % 5
			return *m, nil
		case "shift+tab":
			m.cronFieldIdx = (m.cronFieldIdx + 4) % 5
			return *m, nil
		case "up":
			m.incrementCronField(1)
			return *m, nil
		case "down":
			m.incrementCronField(-1)
			return *m, nil
		case "backspace":
			// If the field has content beyond a single char, trim the last char
			f := m.cronFields[m.cronFieldIdx]
			if len(f) > 1 {
				m.cronFields[m.cronFieldIdx] = f[:len(f)-1]
				m.validateCron()
			} else {
				// Reset to wildcard and go back to preset mode
				m.cronFields[m.cronFieldIdx] = "*"
				m.usingCustom = false
			}
			return *m, nil
		case "enter":
			// Enter in custom cron mode moves to next section if valid
			if m.cronValid {
				m.blurInputs()
				m.focusedField = FieldTarget
				m.onFieldFocus()
			}
			return *m, nil
		default:
			if len(key) == 1 {
				char := key[0]
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
		return *m, nil
	}

	// Preset selection mode
	switch key {
	case "up", "k":
		if m.presetIndex > 0 {
			m.presetIndex--
		}
		return *m, nil
	case "down", "j":
		if m.presetIndex < len(m.presets)-1 {
			m.presetIndex++
		}
		return *m, nil
	case "enter":
		if m.presets[m.presetIndex].Expr == "" {
			// Custom selected
			m.usingCustom = true
			return *m, nil
		}
		// Selecting a preset just selects it; doesn't advance
		return *m, nil
	}
	return *m, nil
}

// --- Target field ---

func (m *scheduleWizardModel) handleTargetField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "up", "k":
		if m.targetIndex > 0 {
			m.targetIndex--
		}
		return *m, nil
	case "down", "j":
		if m.targetIndex < len(m.flatNodes)-1 {
			m.targetIndex++
		}
		return *m, nil
	case " ":
		// Toggle expand
		if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
			node := m.flatNodes[m.targetIndex]
			if node.Type == "session" || node.Type == "window" {
				nodeKey := node.Type + ":" + node.Target
				m.targetExpand[nodeKey] = !m.targetExpand[nodeKey]
				m.rebuildFlatNodes()
			}
		}
		return *m, nil
	case "enter":
		if m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
			node := m.flatNodes[m.targetIndex]
			if node.Type == "pane" {
				// Select pane and store it
				m.selectedTarget = node.Target
				return *m, nil
			}
			// Toggle expand for non-panes
			nodeKey := node.Type + ":" + node.Target
			m.targetExpand[nodeKey] = !m.targetExpand[nodeKey]
			m.rebuildFlatNodes()
		}
		return *m, nil
	}
	return *m, nil
}

// --- Command field ---

func (m *scheduleWizardModel) handleCommandField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "enter" {
		// Move to next field instead of submitting
		m.blurInputs()
		m.focusedField = FieldName
		m.onFieldFocus()
		return *m, textinput.Blink
	}
	// Pass through to text input
	var cmd tea.Cmd
	m.commandInput, cmd = m.commandInput.Update(msg)
	return *m, cmd
}

// --- Name field ---

func (m *scheduleWizardModel) handleNameField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "enter" {
		// Move to next field
		m.blurInputs()
		m.focusedField = FieldPreAction
		m.onFieldFocus()
		return *m, nil
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return *m, cmd
}

// --- Pre-action field ---

func (m *scheduleWizardModel) handlePreActionField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "up", "k":
		if m.preActionIndex > 0 {
			m.preActionIndex--
		}
		return *m, nil
	case "down", "j":
		if m.preActionIndex < len(m.preActions)-1 {
			m.preActionIndex++
		}
		return *m, nil
	case "enter":
		// Move to buttons
		m.focusedField = FieldButtons
		return *m, nil
	}
	return *m, nil
}

// --- Buttons field ---

func (m *scheduleWizardModel) handleButtonsField(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "left", "h":
		m.buttonFocusIdx = 0
		return *m, nil
	case "right", "l":
		m.buttonFocusIdx = 1
		return *m, nil
	case "enter":
		m.done = true
		m.cancelled = m.buttonFocusIdx == 1
		return *m, nil
	case "s":
		m.done = true
		m.cancelled = false
		return *m, nil
	case "c":
		m.done = true
		m.cancelled = true
		return *m, nil
	}
	return *m, nil
}

// --- Shared helpers (unchanged from original) ---

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

	target := m.selectedTarget
	if target == "" && m.targetIndex >= 0 && m.targetIndex < len(m.flatNodes) {
		node := m.flatNodes[m.targetIndex]
		if node.Type == "pane" {
			target = node.Target
		}
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

// ── View ────────────────────────────────────────────────────────────────

// Styles local to the form rendering
var (
	formSectionFocusedBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(primaryColor).
				Padding(0, 1)

	formSectionUnfocusedStyle = lipgloss.NewStyle().
					PaddingLeft(2)

	formSectionLabelFocused = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor)

	formSectionLabelUnfocused = lipgloss.NewStyle().
					Foreground(dimColor)

	formSummaryValue = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
)

func (m scheduleWizardModel) View() string {
	var sections []string

	// Title
	editMode := "Add"
	if m.editingID != "" {
		editMode = "Edit"
	}
	title := schedTitleStyle.Render(fmt.Sprintf("%s Scheduled Job", editMode))
	sections = append(sections, title)
	sections = append(sections, "")

	// Render each section
	sections = append(sections, m.viewScheduleSection())
	sections = append(sections, m.viewTargetSection())
	sections = append(sections, m.viewCommandSection())
	sections = append(sections, m.viewNameSection())
	sections = append(sections, m.viewPreActionSection())
	sections = append(sections, "")
	sections = append(sections, m.viewButtons())

	// Navigation hint
	sections = append(sections, "")
	hint := "[Tab] next section [Shift+Tab] prev [Esc] cancel"
	sections = append(sections, schedHintStyle.Render(hint))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// --- Schedule section ---

func (m scheduleWizardModel) viewScheduleSection() string {
	focused := m.focusedField == FieldSchedule

	if !focused {
		// Compact summary line
		label := formSectionLabelUnfocused.Render("Schedule: ")
		value := formSummaryValue.Render(m.scheduleDisplayValue())
		return formSectionUnfocusedStyle.Render(label + value)
	}

	// Expanded view
	var lines []string
	header := formSectionLabelFocused.Render("Schedule")

	if m.usingCustom {
		lines = append(lines, header)
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
		lines = append(lines, wizRefStyle.Render("[Tab] switch cron field [Up/Down] adjust [Backspace] back to presets"))
	} else {
		lines = append(lines, header)
		lines = append(lines, "")

		for i, preset := range m.presets {
			var row string
			if i == m.presetIndex {
				row = selectedStyle.Render("> ") + lipgloss.NewStyle().Bold(true).Render(preset.Name)
			} else {
				row = "  " + preset.Name
			}
			lines = append(lines, row)
			if i == m.presetIndex && preset.Description != "" {
				lines = append(lines, "    "+wizSubtitleStyle.Render(preset.Description))
			}
		}
	}

	content := strings.Join(lines, "\n")
	return formSectionFocusedBorder.Render(content)
}

func (m scheduleWizardModel) scheduleDisplayValue() string {
	if m.usingCustom {
		expr := strings.Join(m.cronFields[:], " ")
		if m.cronValid {
			return config.CronToEnglish(expr)
		}
		return expr + " (invalid)"
	}
	return m.presets[m.presetIndex].Name
}

// --- Target section ---

func (m scheduleWizardModel) viewTargetSection() string {
	focused := m.focusedField == FieldTarget

	if !focused {
		label := formSectionLabelUnfocused.Render("Target: ")
		target := m.selectedTarget
		if target == "" {
			target = "(none selected)"
		}
		value := formSummaryValue.Render(target)
		return formSectionUnfocusedStyle.Render(label + value)
	}

	// Expanded tree view
	var lines []string
	header := formSectionLabelFocused.Render("Target Pane")
	lines = append(lines, header)
	lines = append(lines, "")

	if len(m.flatNodes) == 0 {
		if m.tree == nil {
			lines = append(lines, schedHintStyle.Render("Loading tmux sessions..."))
		} else {
			lines = append(lines, schedHintStyle.Render("No tmux sessions found. Start a tmux session first."))
		}
	} else {
		maxDisplay := 12
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
				if node.Type == "pane" {
					row += schedTargetStyle.Render(" <- select")
				}
			} else {
				row = "  " + indent + icon + " " + name
			}

			lines = append(lines, row)
		}
		lines = append(lines, "")
		lines = append(lines, wizRefStyle.Render("[Space/Enter] expand [Enter on pane] select"))
	}

	content := strings.Join(lines, "\n")
	return formSectionFocusedBorder.Render(content)
}

// --- Command section ---

func (m scheduleWizardModel) viewCommandSection() string {
	focused := m.focusedField == FieldCommand

	if !focused {
		label := formSectionLabelUnfocused.Render("Command: ")
		cmd := m.commandInput.Value()
		if cmd == "" {
			cmd = "(empty)"
		}
		value := formSummaryValue.Render(cmd)
		return formSectionUnfocusedStyle.Render(label + value)
	}

	var lines []string
	header := formSectionLabelFocused.Render("Command")
	lines = append(lines, header)

	cmdStyle := wizInputStyle
	if m.commandInput.Focused() {
		cmdStyle = cmdStyle.BorderForeground(activeColor)
	}
	lines = append(lines, cmdStyle.Render(m.commandInput.View()))

	content := strings.Join(lines, "\n")
	return formSectionFocusedBorder.Render(content)
}

// --- Name section ---

func (m scheduleWizardModel) viewNameSection() string {
	focused := m.focusedField == FieldName

	if !focused {
		label := formSectionLabelUnfocused.Render("Name: ")
		name := m.nameInput.Value()
		if name == "" {
			name = "(optional)"
		}
		value := formSummaryValue.Render(name)
		return formSectionUnfocusedStyle.Render(label + value)
	}

	var lines []string
	header := formSectionLabelFocused.Render("Name (optional)")
	lines = append(lines, header)

	nameStyle := wizInputStyle
	if m.nameInput.Focused() {
		nameStyle = nameStyle.BorderForeground(activeColor)
	}
	lines = append(lines, nameStyle.Render(m.nameInput.View()))

	content := strings.Join(lines, "\n")
	return formSectionFocusedBorder.Render(content)
}

// --- Pre-action section ---

func (m scheduleWizardModel) viewPreActionSection() string {
	focused := m.focusedField == FieldPreAction

	if !focused {
		label := formSectionLabelUnfocused.Render("Pre-Action: ")
		value := formSummaryValue.Render(m.preActionLabels[m.preActionIndex])
		return formSectionUnfocusedStyle.Render(label + value)
	}

	var lines []string
	header := formSectionLabelFocused.Render("Pre-Action")
	lines = append(lines, header)
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

	content := strings.Join(lines, "\n")
	return formSectionFocusedBorder.Render(content)
}

// --- Buttons ---

func (m scheduleWizardModel) viewButtons() string {
	focused := m.focusedField == FieldButtons

	var saveBtn, cancelBtn string
	if focused {
		if m.buttonFocusIdx == 0 {
			saveBtn = wizSaveBtnActiveStyle.Render(" Save ")
			cancelBtn = wizCancelBtnStyle.Render(" Cancel ")
		} else {
			saveBtn = wizSaveBtnInactiveStyle.Render(" Save ")
			cancelBtn = wizCancelBtnActiveStyle.Render(" Cancel ")
		}
	} else {
		saveBtn = wizSaveBtnInactiveStyle.Render(" Save ")
		cancelBtn = wizCancelBtnStyle.Render(" Cancel ")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, "          ", saveBtn, "  ", cancelBtn)
	return buttons
}
