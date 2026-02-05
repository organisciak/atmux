package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// testItem is a simple ListItem implementation for testing.
type testItem struct {
	id   string
	name string
}

func (t testItem) ID() string {
	return t.id
}

func (t testItem) Render(selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	name := t.name
	if len(name) > width-2 && width > 5 {
		name = name[:width-5] + "..."
	}
	if selected {
		return prefix + lipgloss.NewStyle().Bold(true).Render(name)
	}
	return prefix + name
}

func makeTestItems(count int) []ListItem {
	items := make([]ListItem, count)
	for i := 0; i < count; i++ {
		items[i] = testItem{
			id:   string(rune('a' + i)),
			name: strings.Repeat(string(rune('A'+i)), 10),
		}
	}
	return items
}

func TestNewExpandableList(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)

	if list.MaxCollapsed != DefaultMaxCollapsed {
		t.Errorf("expected MaxCollapsed=%d, got %d", DefaultMaxCollapsed, list.MaxCollapsed)
	}
	if list.MaxExpanded != DefaultMaxExpanded {
		t.Errorf("expected MaxExpanded=%d, got %d", DefaultMaxExpanded, list.MaxExpanded)
	}
	if list.Expanded {
		t.Error("expected list to be collapsed by default")
	}
	if len(list.Items) != 10 {
		t.Errorf("expected 10 items, got %d", len(list.Items))
	}
}

func TestVisibleCountCollapsed(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	if list.VisibleCount() != 5 {
		t.Errorf("expected visible count 5, got %d", list.VisibleCount())
	}
}

func TestVisibleCountExpanded(t *testing.T) {
	items := makeTestItems(25)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.MaxExpanded = 20
	list.Expanded = true

	if list.VisibleCount() != 20 {
		t.Errorf("expected visible count 20, got %d", list.VisibleCount())
	}
}

func TestVisibleCountFewItems(t *testing.T) {
	// When items <= MaxCollapsed, no expand UI needed
	items := makeTestItems(3)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	if list.VisibleCount() != 3 {
		t.Errorf("expected visible count 3, got %d", list.VisibleCount())
	}
	if list.HasFooter() {
		t.Error("expected no footer when items <= MaxCollapsed")
	}
}

func TestHasFooter(t *testing.T) {
	tests := []struct {
		name         string
		itemCount    int
		maxCollapsed int
		wantFooter   bool
	}{
		{"items less than max", 3, 5, false},
		{"items equal to max", 5, 5, false},
		{"items more than max", 10, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := makeTestItems(tt.itemCount)
			list := NewExpandableList(items)
			list.MaxCollapsed = tt.maxCollapsed

			if list.HasFooter() != tt.wantFooter {
				t.Errorf("HasFooter()=%v, want %v", list.HasFooter(), tt.wantFooter)
			}
		})
	}
}

func TestHiddenCount(t *testing.T) {
	items := makeTestItems(12)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.MaxExpanded = 10

	// Collapsed: 12 - 5 = 7 hidden
	if list.HiddenCount() != 7 {
		t.Errorf("expected 7 hidden when collapsed, got %d", list.HiddenCount())
	}

	// Expanded: 12 - 10 = 2 hidden
	list.Expanded = true
	if list.HiddenCount() != 2 {
		t.Errorf("expected 2 hidden when expanded, got %d", list.HiddenCount())
	}
}

func TestTotalSelectableCount(t *testing.T) {
	// With footer (items > MaxCollapsed)
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// 5 visible items + 1 footer
	if list.TotalSelectableCount() != 6 {
		t.Errorf("expected 6 selectable (5 items + 1 footer), got %d", list.TotalSelectableCount())
	}

	// Without footer (items <= MaxCollapsed)
	items2 := makeTestItems(3)
	list2 := NewExpandableList(items2)
	list2.MaxCollapsed = 5

	if list2.TotalSelectableCount() != 3 {
		t.Errorf("expected 3 selectable (no footer), got %d", list2.TotalSelectableCount())
	}
}

func TestMoveSelection(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Start at 0
	if list.SelectedIndex != 0 {
		t.Errorf("expected initial selection 0, got %d", list.SelectedIndex)
	}

	// Move down
	list.MoveSelection(1)
	if list.SelectedIndex != 1 {
		t.Errorf("expected selection 1 after move down, got %d", list.SelectedIndex)
	}

	// Move to footer (index 5)
	list.MoveSelection(4)
	if list.SelectedIndex != 5 {
		t.Errorf("expected selection 5 (footer), got %d", list.SelectedIndex)
	}

	// Can't move past footer
	list.MoveSelection(1)
	if list.SelectedIndex != 5 {
		t.Errorf("expected selection to stay at 5, got %d", list.SelectedIndex)
	}

	// Move up
	list.MoveSelection(-1)
	if list.SelectedIndex != 4 {
		t.Errorf("expected selection 4, got %d", list.SelectedIndex)
	}

	// Can't move before 0
	list.SelectedIndex = 0
	list.MoveSelection(-1)
	if list.SelectedIndex != 0 {
		t.Errorf("expected selection to stay at 0, got %d", list.SelectedIndex)
	}
}

func TestIsFooterSelected(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Not footer selected initially
	if list.IsFooterSelected() {
		t.Error("expected footer not selected initially")
	}

	// Select footer
	list.SelectedIndex = 5
	if !list.IsFooterSelected() {
		t.Error("expected footer to be selected at index 5")
	}

	// No footer for small lists
	items2 := makeTestItems(3)
	list2 := NewExpandableList(items2)
	list2.SelectedIndex = 3
	if list2.IsFooterSelected() {
		t.Error("expected no footer selection for small list")
	}
}

func TestSelectedItem(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Get first item
	item := list.SelectedItem()
	if item == nil {
		t.Fatal("expected non-nil item")
	}
	if item.ID() != "a" {
		t.Errorf("expected item ID 'a', got %q", item.ID())
	}

	// Get last visible item
	list.SelectedIndex = 4
	item = list.SelectedItem()
	if item == nil {
		t.Fatal("expected non-nil item")
	}
	if item.ID() != "e" {
		t.Errorf("expected item ID 'e', got %q", item.ID())
	}

	// Footer selected returns nil
	list.SelectedIndex = 5
	item = list.SelectedItem()
	if item != nil {
		t.Error("expected nil item when footer selected")
	}
}

func TestToggleExpanded(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	expandCalled := false
	list.OnExpand = func() {
		expandCalled = true
	}

	// Initially collapsed
	if list.Expanded {
		t.Error("expected list to be collapsed initially")
	}

	// Toggle to expanded
	list.ToggleExpanded()
	if !list.Expanded {
		t.Error("expected list to be expanded after toggle")
	}
	if !expandCalled {
		t.Error("expected OnExpand to be called")
	}

	// Toggle back to collapsed
	expandCalled = false
	list.ToggleExpanded()
	if list.Expanded {
		t.Error("expected list to be collapsed after second toggle")
	}
	if !expandCalled {
		t.Error("expected OnExpand to be called on collapse")
	}
}

func TestToggleExpandedClampsSelection(t *testing.T) {
	items := makeTestItems(25)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.MaxExpanded = 20
	list.Expanded = true

	// Select item beyond collapsed view
	list.SelectedIndex = 15

	// Collapse
	list.ToggleExpanded()

	// Selection should clamp to last position (5 items + 1 footer = 6, so max index is 5)
	if list.SelectedIndex != 5 {
		t.Errorf("expected selection clamped to 5, got %d", list.SelectedIndex)
	}
}

func TestKeyboardNavigation(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Down arrow
	downMsg := tea.KeyMsg{Type: tea.KeyDown}
	list, _ = list.Update(downMsg)
	if list.SelectedIndex != 1 {
		t.Errorf("expected selection 1 after down, got %d", list.SelectedIndex)
	}

	// Up arrow
	upMsg := tea.KeyMsg{Type: tea.KeyUp}
	list, _ = list.Update(upMsg)
	if list.SelectedIndex != 0 {
		t.Errorf("expected selection 0 after up, got %d", list.SelectedIndex)
	}

	// j key (vim down)
	jMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	list, _ = list.Update(jMsg)
	if list.SelectedIndex != 1 {
		t.Errorf("expected selection 1 after j, got %d", list.SelectedIndex)
	}

	// k key (vim up)
	kMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	list, _ = list.Update(kMsg)
	if list.SelectedIndex != 0 {
		t.Errorf("expected selection 0 after k, got %d", list.SelectedIndex)
	}
}

func TestEnterOnFooterExpands(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Navigate to footer
	list.SelectedIndex = 5

	// Press Enter
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	list, _ = list.Update(enterMsg)

	if !list.Expanded {
		t.Error("expected list to expand on Enter when footer selected")
	}
}

func TestEnterOnItemCallsOnSelect(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	selectedItem := ListItem(nil)
	list.OnSelect = func(item ListItem) {
		selectedItem = item
	}

	// Select first item
	list.SelectedIndex = 0

	// Press Enter
	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	list, _ = list.Update(enterMsg)

	if selectedItem == nil {
		t.Fatal("expected OnSelect to be called")
	}
	if selectedItem.ID() != "a" {
		t.Errorf("expected selected item ID 'a', got %q", selectedItem.ID())
	}
}

func TestSpaceKeyToggles(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Navigate to footer
	list.SelectedIndex = 5

	// Press Space
	spaceMsg := tea.KeyMsg{Type: tea.KeySpace}
	list, _ = list.Update(spaceMsg)

	if !list.Expanded {
		t.Error("expected list to expand on Space when footer selected")
	}
}

func TestViewRendersItems(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	view := list.View(40)

	// Should contain first 5 items
	if !strings.Contains(view, "AAAAAAAAAA") {
		t.Error("expected view to contain first item")
	}
	if !strings.Contains(view, "EEEEEEEEEE") {
		t.Error("expected view to contain 5th item (E)")
	}

	// Should NOT contain 6th item (collapsed)
	if strings.Contains(view, "FFFFFFFFFF") {
		t.Error("expected view to NOT contain 6th item when collapsed")
	}

	// Should contain "Show more" footer
	if !strings.Contains(view, "Show more") {
		t.Error("expected view to contain 'Show more' footer")
	}
	if !strings.Contains(view, "(5)") {
		t.Error("expected view to show hidden count (5)")
	}
}

func TestViewRendersExpandedItems(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.Expanded = true

	view := list.View(40)

	// Should contain all items
	if !strings.Contains(view, "AAAAAAAAAA") {
		t.Error("expected view to contain first item")
	}
	if !strings.Contains(view, "JJJJJJJJJJ") {
		t.Error("expected view to contain last item (J)")
	}

	// Should contain "Show less" footer
	if !strings.Contains(view, "Show less") {
		t.Error("expected view to contain 'Show less' footer")
	}
}

func TestViewNoFooterForSmallList(t *testing.T) {
	items := makeTestItems(3)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	view := list.View(40)

	// Should contain all items
	if !strings.Contains(view, "AAAAAAAAAA") {
		t.Error("expected view to contain first item")
	}
	if !strings.Contains(view, "CCCCCCCCCC") {
		t.Error("expected view to contain last item (C)")
	}

	// Should NOT contain footer
	if strings.Contains(view, "Show more") || strings.Contains(view, "Show less") {
		t.Error("expected no footer for small list")
	}
}

func TestSelectByIndex(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Valid selection
	if !list.SelectByIndex(3) {
		t.Error("expected SelectByIndex(3) to succeed")
	}
	if list.SelectedIndex != 3 {
		t.Errorf("expected selection 3, got %d", list.SelectedIndex)
	}

	// Footer selection
	if !list.SelectByIndex(5) {
		t.Error("expected SelectByIndex(5) to succeed for footer")
	}
	if list.SelectedIndex != 5 {
		t.Errorf("expected selection 5, got %d", list.SelectedIndex)
	}

	// Invalid selection (too high)
	if list.SelectByIndex(10) {
		t.Error("expected SelectByIndex(10) to fail")
	}

	// Invalid selection (negative)
	if list.SelectByIndex(-1) {
		t.Error("expected SelectByIndex(-1) to fail")
	}
}

func TestReset(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.Expanded = true
	list.SelectedIndex = 8

	list.Reset()

	if list.Expanded {
		t.Error("expected list to be collapsed after reset")
	}
	if list.SelectedIndex != 0 {
		t.Errorf("expected selection 0 after reset, got %d", list.SelectedIndex)
	}
}

func TestSetItems(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5
	list.SelectedIndex = 4

	// Set fewer items
	newItems := makeTestItems(3)
	list.SetItems(newItems)

	if len(list.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(list.Items))
	}

	// Selection should clamp
	if list.SelectedIndex != 2 {
		t.Errorf("expected selection clamped to 2, got %d", list.SelectedIndex)
	}
}

func TestVisibleItems(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	visible := list.VisibleItems()
	if len(visible) != 5 {
		t.Errorf("expected 5 visible items, got %d", len(visible))
	}

	// Verify they're the first 5
	for i, item := range visible {
		expectedID := string(rune('a' + i))
		if item.ID() != expectedID {
			t.Errorf("expected item %d ID %q, got %q", i, expectedID, item.ID())
		}
	}
}

func TestRenderFooterLine(t *testing.T) {
	items := makeTestItems(10)
	list := NewExpandableList(items)
	list.MaxCollapsed = 5

	// Collapsed footer
	footer := list.RenderFooterLine(false)
	if !strings.Contains(footer, "Show more") {
		t.Error("expected 'Show more' in collapsed footer")
	}
	if !strings.Contains(footer, "(5)") {
		t.Error("expected hidden count in footer")
	}

	// Selected collapsed footer
	selectedFooter := list.RenderFooterLine(true)
	if !strings.Contains(selectedFooter, "Show more") {
		t.Error("expected 'Show more' in selected footer")
	}

	// Expanded footer
	list.Expanded = true
	expandedFooter := list.RenderFooterLine(false)
	if !strings.Contains(expandedFooter, "Show less") {
		t.Error("expected 'Show less' in expanded footer")
	}

	// No footer for small list
	smallList := NewExpandableList(makeTestItems(3))
	smallList.MaxCollapsed = 5
	if smallList.RenderFooterLine(false) != "" {
		t.Error("expected empty footer for small list")
	}
}

func TestEmptyList(t *testing.T) {
	list := NewExpandableList(nil)

	if list.VisibleCount() != 0 {
		t.Errorf("expected 0 visible items, got %d", list.VisibleCount())
	}
	if list.TotalSelectableCount() != 0 {
		t.Errorf("expected 0 selectable, got %d", list.TotalSelectableCount())
	}
	if list.HasFooter() {
		t.Error("expected no footer for empty list")
	}
	if list.SelectedItem() != nil {
		t.Error("expected nil selected item for empty list")
	}

	// Navigation should be safe
	list.MoveSelection(1)
	if list.SelectedIndex != 0 {
		t.Errorf("expected selection to stay at 0, got %d", list.SelectedIndex)
	}

	// View should not panic
	view := list.View(40)
	if view != "" {
		t.Logf("empty list view: %q", view)
	}
}
