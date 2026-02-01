package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/porganisciak/agent-tmux/scheduler"
	"github.com/porganisciak/agent-tmux/tmux"
)

// ScheduleAddResult contains the outcome of the add wizard
type ScheduleAddResult struct {
	Added bool
	JobID string
}

// RunScheduleAddTUI runs the schedule add wizard
func RunScheduleAddTUI() (*ScheduleAddResult, error) {
	m := newScheduleAddModel()
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}
	if model, ok := finalModel.(scheduleAddModel); ok {
		return &ScheduleAddResult{
			Added: model.added,
			JobID: model.jobID,
		}, nil
	}
	return &ScheduleAddResult{}, nil
}

const (
	stepPreset    = 0
	stepCustom    = 1
	stepTarget    = 2
	stepCommand   = 3
	stepPreAction = 4
	stepConfirm   = 5
)

type scheduleAddModel struct {
	width  int
	height int
	step   int
	cursor int

	// Step 1: Preset selection
	presets       []scheduler.CronPreset
	selectedCron  string
	useCustomCron bool

	// Step 1b: Custom cron
	cronFields     [5]string // [minute, hour, day, month, weekday]
	cronFieldFocus int

	// Step 2: Target selection
	tree           *tmux.Tree
	treeNodes      []*tmux.TreeNode
	selectedTarget string

	// Step 3: Command input
	commandInput string

	// Step 4: Pre-action
	preAction scheduler.PreAction

	// Result
	added bool
	jobID string
}

func newScheduleAddModel() scheduleAddModel {
	presets := scheduler.CommonPresets()
	// Add "Custom schedule..." option
	return scheduleAddModel{
		step:       stepPreset,
		presets:    presets,
		cronFields: [5]string{"0", "9", "*", "*", "*"},
		preAction:  scheduler.PreActionNone,
	}
}

type treeLoadedMsg struct {
	tree *tmux.Tree
	err  error
}

func (m scheduleAddModel) Init() tea.Cmd {
	return nil
}

func (m scheduleAddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m scheduleAddModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case stepPreset:
		return m.handlePresetKey(msg)
	case stepCustom:
		return m.handleCustomCronKey(msg)
	case stepTarget:
		return m.handleTargetKey(msg)
	case stepCommand:
		return m.handleCommandKey(msg)
	case stepPreAction:
		return m.handlePreActionKey(msg)
	case stepConfirm:
		return m.handleConfirmKey(msg)
	}
	return m, nil
}

func (m scheduleAddModel) handlePresetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxCursor := len(m.presets) // presets + Custom option

	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < maxCursor {
			m.cursor++
		}
		return m, nil

	case "enter":
		if m.cursor < len(m.presets) {
			// Selected a preset
			m.selectedCron = m.presets[m.cursor].Expression
			m.useCustomCron = false
			m.step = stepTarget
			m.cursor = 0
			return m, m.loadTree()
		} else {
			// Custom schedule
			m.useCustomCron = true
			m.step = stepCustom
			m.cursor = 0
			m.cronFieldFocus = 0
			return m, nil
		}
	}
	return m, nil
}

func (m scheduleAddModel) handleCustomCronKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Prioritize cron character input FIRST
	if len(key) == 1 && (key[0] >= '0' && key[0] <= '9' ||
		key[0] == '*' || key[0] == '/' || key[0] == '-' || key[0] == ',') {
		m.cronFields[m.cronFieldFocus] += key
		return m, nil
	}

	// Only handle special keys if not a valid cron character
	switch key {
	case "esc":
		m.step = stepPreset
		m.cursor = len(m.presets) // Go back to "Custom" option
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case "tab", "right":
		m.cronFieldFocus = (m.cronFieldFocus + 1) % 5
		return m, nil

	case "shift+tab", "left":
		m.cronFieldFocus = (m.cronFieldFocus + 4) % 5
		return m, nil

	case "enter":
		// Build and validate cron expression
		expr := scheduler.BuildCronExpression(
			m.cronFields[0], m.cronFields[1], m.cronFields[2],
			m.cronFields[3], m.cronFields[4])
		if _, err := scheduler.ParseSchedule(expr); err == nil {
			m.selectedCron = expr
			m.step = stepTarget
			m.cursor = 0
			return m, m.loadTree()
		}
		// Invalid - stay on this step
		return m, nil

	case "backspace":
		if len(m.cronFields[m.cronFieldFocus]) > 0 {
			m.cronFields[m.cronFieldFocus] = m.cronFields[m.cronFieldFocus][:len(m.cronFields[m.cronFieldFocus])-1]
		}
		return m, nil
	}

	return m, nil
}

func (m scheduleAddModel) handleTargetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		if m.useCustomCron {
			m.step = stepCustom
		} else {
			m.step = stepPreset
		}
		m.cursor = 0
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.treeNodes)-1 {
			m.cursor++
		}
		return m, nil

	case " ":
		// Toggle expand/collapse for sessions and windows
		if m.cursor < len(m.treeNodes) {
			node := m.treeNodes[m.cursor]
			if node.Type == "session" || node.Type == "window" {
				node.Expanded = !node.Expanded
				// Rebuild tree nodes
				if m.tree != nil {
					m.treeNodes = buildTreeNodesWithState(m.tree, m.treeNodes)
				}
			}
		}
		return m, nil

	case "enter":
		// Select pane
		if m.cursor < len(m.treeNodes) {
			node := m.treeNodes[m.cursor]
			if node.Type == "pane" {
				m.selectedTarget = node.Target
				m.step = stepCommand
				m.cursor = 0
				return m, nil
			}
		}
		return m, nil
	}
	return m, nil
}

func (m scheduleAddModel) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Prioritize text input - capture printable characters FIRST
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		m.commandInput += key
		return m, nil
	} else if key == "space" {
		m.commandInput += " "
		return m, nil
	}

	// Only handle special keys if not a printable character
	switch key {
	case "esc", "backspace":
		if m.commandInput == "" {
			m.step = stepTarget
			m.cursor = 0
			return m, nil
		}
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		return m, nil

	case "ctrl+c", "q":
		if m.commandInput == "" {
			return m, tea.Quit
		}
		return m, nil

	case "enter":
		if m.commandInput != "" {
			m.step = stepPreAction
			m.cursor = 0
			return m, nil
		}
		return m, nil
	}

	return m, nil
}

func (m scheduleAddModel) handlePreActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.step = stepCommand
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < 2 {
			m.cursor++
		}
		return m, nil

	case "enter":
		switch m.cursor {
		case 0:
			m.preAction = scheduler.PreActionNone
		case 1:
			m.preAction = scheduler.PreActionCompact
		case 2:
			m.preAction = scheduler.PreActionNew
		}
		m.step = stepConfirm
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m scheduleAddModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.step = stepPreAction
		m.cursor = 0
		return m, nil

	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k", "left", "h":
		m.cursor = 0
		return m, nil

	case "down", "j", "right", "l", "tab":
		m.cursor = 1
		return m, nil

	case "enter":
		if m.cursor == 0 {
			// Save
			return m.saveSchedule()
		}
		// Cancel
		return m, tea.Quit
	}
	return m, nil
}

func (m scheduleAddModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	switch m.step {
	case stepPreset:
		return m.handlePresetMouse(msg)
	case stepTarget:
		return m.handleTargetMouse(msg)
	case stepPreAction:
		return m.handlePreActionMouse(msg)
	case stepConfirm:
		return m.handleConfirmMouse(msg)
	}
	return m, nil
}

func (m scheduleAddModel) handlePresetMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Calculate click region for preset options
	// Title takes ~5 lines, each preset option is 1 line
	startY := m.height/2 - 10 // Approximate start of options
	clickedIdx := msg.Y - startY - 3

	if clickedIdx >= 0 && clickedIdx < len(m.presets) {
		m.cursor = clickedIdx
		// Double-click to select
		m.selectedCron = m.presets[m.cursor].Expression
		m.useCustomCron = false
		m.step = stepTarget
		m.cursor = 0
		return m, m.loadTree()
	} else if clickedIdx == len(m.presets)+1 {
		// Clicked "Custom schedule..."
		m.useCustomCron = true
		m.step = stepCustom
		m.cursor = 0
		m.cronFieldFocus = 0
		return m, nil
	}

	return m, nil
}

func (m scheduleAddModel) handleTargetMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Calculate click region for tree nodes
	startY := m.height/2 - 10
	clickedIdx := msg.Y - startY - 3

	if clickedIdx >= 0 && clickedIdx < len(m.treeNodes) {
		node := m.treeNodes[clickedIdx]
		m.cursor = clickedIdx

		// If session or window, toggle expand
		if node.Type == "session" || node.Type == "window" {
			node.Expanded = !node.Expanded
			if m.tree != nil {
				m.treeNodes = buildTreeNodesWithState(m.tree, m.treeNodes)
			}
		} else if node.Type == "pane" {
			// Select pane
			m.selectedTarget = node.Target
			m.step = stepCommand
			m.cursor = 0
		}
	}

	return m, nil
}

func (m scheduleAddModel) handlePreActionMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Calculate click region for pre-action options
	startY := m.height/2 - 5
	clickedIdx := msg.Y - startY - 3

	if clickedIdx >= 0 && clickedIdx <= 2 {
		m.cursor = clickedIdx
		// Select and continue
		switch m.cursor {
		case 0:
			m.preAction = scheduler.PreActionNone
		case 1:
			m.preAction = scheduler.PreActionCompact
		case 2:
			m.preAction = scheduler.PreActionNew
		}
		m.step = stepConfirm
		m.cursor = 0
		return m, nil
	}

	return m, nil
}

func (m scheduleAddModel) handleConfirmMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Calculate button regions
	// Buttons are at the bottom of the centered box
	buttonY := m.height/2 + 8 // Approximate Y position of buttons

	if msg.Y == buttonY {
		// Check X position for Save vs Cancel
		centerX := m.width / 2
		if msg.X >= centerX-15 && msg.X < centerX {
			// Clicked Save
			return m.saveSchedule()
		} else if msg.X >= centerX+3 && msg.X < centerX+20 {
			// Clicked Cancel
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m scheduleAddModel) loadTree() tea.Cmd {
	return func() tea.Msg {
		tree, err := tmux.FetchTree()
		return treeLoadedMsg{tree: tree, err: err}
	}
}

func (m scheduleAddModel) saveSchedule() (tea.Model, tea.Cmd) {
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

func (m scheduleAddModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	switch m.step {
	case stepPreset:
		return m.viewPreset()
	case stepCustom:
		return m.viewCustomCron()
	case stepTarget:
		return m.viewTarget()
	case stepCommand:
		return m.viewCommand()
	case stepPreAction:
		return m.viewPreAction()
	case stepConfirm:
		return m.viewConfirm()
	}
	return ""
}

func (m scheduleAddModel) viewPreset() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Add Scheduled Command")+"                    "+wizSubtitleStyle.Render("Step 1 of 5"))
	lines = append(lines, "")
	lines = append(lines, "How often should this run?")
	lines = append(lines, "")

	for i, preset := range m.presets {
		line := fmt.Sprintf("%s  %s", preset.Label, wizSubtitleStyle.Render("("+preset.Expression+")"))
		if i == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}

	lines = append(lines, wizSeparatorStyle.Render("  ─────────────────────────────────"))

	customLine := "Custom schedule..."
	if m.cursor == len(m.presets) {
		customLine = selectedStyle.Render("> " + customLine)
	} else {
		customLine = "  " + customLine
	}
	lines = append(lines, customLine)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("↑↓ select │ Enter continue │ Esc back │ q quit")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

func (m scheduleAddModel) viewCustomCron() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Custom Schedule")+"                         "+wizSubtitleStyle.Render("Step 1 of 5"))
	lines = append(lines, "")

	// Field headers
	fieldNames := []string{"Minute", "Hour", "Day", "Month", "Weekday"}
	fieldRanges := []string{"(0-59)", "(0-23)", "(1-31)", "(1-12)", "(0-6)"}

	headerLine := "  "
	rangeLine := "  "
	for i, name := range fieldNames {
		var style lipgloss.Style
		if i == m.cronFieldFocus {
			style = wizCronHeaderFocusStyle
		} else {
			style = wizCronHeaderStyle
		}
		headerLine += style.Render(name)
	}
	for i, r := range fieldRanges {
		var style lipgloss.Style
		if i == m.cronFieldFocus {
			style = wizCronRangeFocusStyle
		} else {
			style = wizCronRangeStyle
		}
		rangeLine += style.Render(r)
	}

	lines = append(lines, headerLine)
	lines = append(lines, rangeLine)

	// Field values
	valueLine := "  "
	for i, val := range m.cronFields {
		var style lipgloss.Style
		if i == m.cronFieldFocus {
			style = wizCronFieldFocusStyle
		} else {
			style = wizCronFieldStyle
		}
		if val == "" {
			val = "_"
		}
		valueLine += style.Render("[ " + val + " ]")
	}
	lines = append(lines, valueLine)
	lines = append(lines, "")

	// Preview
	expr := scheduler.BuildCronExpression(
		m.cronFields[0], m.cronFields[1], m.cronFields[2],
		m.cronFields[3], m.cronFields[4])
	preview := scheduler.CronToEnglish(expr)
	_, err := scheduler.ParseSchedule(expr)
	var previewStyle lipgloss.Style
	if err != nil {
		previewStyle = wizPreviewErrStyle
		preview = "Invalid expression"
	} else {
		previewStyle = wizPreviewOKStyle
	}

	lines = append(lines, "Preview: "+previewStyle.Render(preview))
	lines = append(lines, wizSubtitleStyle.Render("Cron:    "+expr))
	lines = append(lines, "")

	// Quick reference
	lines = append(lines, wizRefStyle.Render("Quick Reference:"))
	lines = append(lines, wizRefStyle.Render("  *     = every    */15  = every 15    0,30 = 0 and 30"))
	lines = append(lines, wizRefStyle.Render("  1-5   = 1 thru 5                      Mon-Fri = weekdays"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("Tab next field │ Enter continue │ Esc back")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

func (m scheduleAddModel) viewTarget() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Select Target Pane")+"                    "+wizSubtitleStyle.Render("Step 2 of 5"))
	lines = append(lines, "")
	lines = append(lines, "Which pane should receive the command?")
	lines = append(lines, "")

	if m.tree == nil || len(m.treeNodes) == 0 {
		lines = append(lines, wizSubtitleStyle.Render("Loading tmux sessions..."))
	} else {
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
			if i == m.cursor {
				line = selectedStyle.Render(line)
			} else {
				line = style.Render(line)
			}
			lines = append(lines, line)
		}
	}

	if m.selectedTarget != "" {
		lines = append(lines, "")
		lines = append(lines, wizPreviewOKStyle.Render("Selected: "+m.selectedTarget))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("↑↓ navigate │ Space expand │ Enter select │ Esc back")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

func (m scheduleAddModel) viewCommand() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Command to Send")+"                       "+wizSubtitleStyle.Render("Step 3 of 5"))
	lines = append(lines, "")
	lines = append(lines, "What command should be sent to the pane?")
	lines = append(lines, "")

	// Input field
	inputContent := m.commandInput
	if inputContent == "" {
		inputContent = wizSubtitleStyle.Render("Type a command...")
	}
	lines = append(lines, wizInputStyle.Render(inputContent+"_"))
	lines = append(lines, "")

	// Common commands
	lines = append(lines, wizRefStyle.Render("Common commands:"))
	lines = append(lines, wizRefStyle.Render("  /status   - Check agent status"))
	lines = append(lines, wizRefStyle.Render("  /compact  - Compact context"))
	lines = append(lines, wizRefStyle.Render("  /new      - Start fresh session"))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("Enter continue │ Esc back")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

func (m scheduleAddModel) viewPreAction() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Before Running Command")+"                "+wizSubtitleStyle.Render("Step 4 of 5"))
	lines = append(lines, "")
	lines = append(lines, "Should anything happen before your command?")
	lines = append(lines, "")

	options := []struct {
		label string
		desc  string
	}{
		{"None - just send the command", ""},
		{"Compact first - send /compact, wait 2s, then command", ""},
		{"New session first - send /new, wait 2s, then command", ""},
	}

	for i, opt := range options {
		line := opt.label
		if i == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, wizRefStyle.Render("Tip: \"Compact first\" is useful for daily status checks"))
	lines = append(lines, wizRefStyle.Render("     to ensure the agent has fresh context."))

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("↑↓ select │ Enter continue │ Esc back")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

func (m scheduleAddModel) viewConfirm() string {
	var lines []string
	lines = append(lines, schedTitleStyle.Render("Review & Save")+"                           "+wizSubtitleStyle.Render("Step 5 of 5"))
	lines = append(lines, "")

	// Schedule
	lines = append(lines, wizLabelStyle.Render("Schedule:")+"    "+wizValueStyle.Render(scheduler.CronToEnglish(m.selectedCron)))
	lines = append(lines, wizLabelStyle.Render("             ")+wizSubtitleStyle.Render("("+m.selectedCron+")"))
	lines = append(lines, "")

	// Target
	lines = append(lines, wizLabelStyle.Render("Target:")+"      "+wizValueStyle.Render(m.selectedTarget))
	lines = append(lines, "")

	// Pre-action
	preActionDesc := "None"
	switch m.preAction {
	case scheduler.PreActionCompact:
		preActionDesc = "Send /compact, wait 2 seconds"
	case scheduler.PreActionNew:
		preActionDesc = "Send /new, wait 2 seconds"
	}
	lines = append(lines, wizLabelStyle.Render("Pre-action:")+"  "+wizValueStyle.Render(preActionDesc))

	// Command
	lines = append(lines, wizLabelStyle.Render("Command:")+"     "+wizValueStyle.Render(m.commandInput))
	lines = append(lines, "")

	// Next run
	nextRun, _ := scheduler.NextRun(m.selectedCron)
	lines = append(lines, wizLabelStyle.Render("Next run:")+"    "+wizValueStyle.Render(nextRun.Format("Mon Jan 2 at 3:04 PM")))
	lines = append(lines, "")

	// Buttons
	saveBtn := "   Save & Exit   "
	cancelBtn := "     Cancel      "

	var saveBtnStyle, cancelBtnStyle lipgloss.Style
	if m.cursor == 0 {
		saveBtnStyle = wizSaveBtnActiveStyle
		cancelBtnStyle = wizCancelBtnStyle
	} else {
		saveBtnStyle = wizSaveBtnInactiveStyle
		cancelBtnStyle = wizCancelBtnActiveStyle
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Top,
		saveBtnStyle.Render(saveBtn),
		"   ",
		cancelBtnStyle.Render(cancelBtn))
	lines = append(lines, buttons)

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	box := wizBoxStyle.Render(content)

	hints := schedHintStyle.Render("←→ select button │ Enter confirm │ Esc back")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, "", hints))
}

// buildTreeNodesWithState rebuilds tree nodes while preserving expanded state
func buildTreeNodesWithState(tree *tmux.Tree, oldNodes []*tmux.TreeNode) []*tmux.TreeNode {
	// Build a map of old expanded states
	expandedMap := make(map[string]bool)
	for _, node := range oldNodes {
		expandedMap[node.Target] = node.Expanded
	}

	var nodes []*tmux.TreeNode

	for _, sess := range tree.Sessions {
		sessTarget := sess.Name
		sessExpanded := true
		if exp, ok := expandedMap[sessTarget]; ok {
			sessExpanded = exp
		}

		sessNode := &tmux.TreeNode{
			Type:     "session",
			Name:     sess.Name,
			Target:   sessTarget,
			Expanded: sessExpanded,
			Level:    0,
			Attached: sess.Attached,
		}
		nodes = append(nodes, sessNode)

		if sessNode.Expanded {
			for _, win := range sess.Windows {
				winTarget := sess.Name + ":" + strconv.Itoa(win.Index)
				winExpanded := true
				if exp, ok := expandedMap[winTarget]; ok {
					winExpanded = exp
				}

				winNode := &tmux.TreeNode{
					Type:     "window",
					Name:     win.Name,
					Target:   winTarget,
					Expanded: winExpanded,
					Level:    1,
					Active:   win.Active,
				}
				sessNode.Children = append(sessNode.Children, winNode)
				nodes = append(nodes, winNode)

				if winNode.Expanded {
					for _, pane := range win.Panes {
						paneNode := &tmux.TreeNode{
							Type:   "pane",
							Name:   pane.Title,
							Target: pane.Target,
							Level:  2,
							Active: pane.Active,
						}
						if pane.Title == "" {
							paneNode.Name = pane.Command
						}
						if paneNode.Name == "" {
							paneNode.Name = "pane " + strconv.Itoa(pane.Index)
						}
						winNode.Children = append(winNode.Children, paneNode)
						nodes = append(nodes, paneNode)
					}
				}
			}
		}
	}

	return nodes
}
