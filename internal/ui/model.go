package ui

import (
	"fmt"
	"strings"
	"time"

	"cview/internal/claude"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	appStyle = lipgloss.NewStyle().
			Padding(1, 2)
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	listStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	detailStyle = listStyle
)

type focusTarget int

const (
	focusSearch focusTarget = iota
	focusList
	focusDetails
)

type Model struct {
	search        textinput.Model
	list          viewport.Model
	details       viewport.Model
	sessions      []claude.Session
	filtered      []claude.Session
	selected      int
	width         int
	height        int
	err           error
	focus         focusTarget
	projectFolder string
}

func NewModel() (Model, error) {
	search := textinput.New()
	search.Placeholder = "Search session history"
	search.Prompt = "Search: "
	search.Focus()

	sessions, err := claude.DiscoverForCurrentDir()
	if err != nil {
		return Model{}, err
	}

	model := Model{
		search:   search,
		sessions: sessions,
		filtered: sessions,
		focus:    focusSearch,
	}
	model.projectFolder = currentProjectDir(sessions)
	model.list = viewport.New(0, 0)
	model.details = viewport.New(0, 0)
	model.syncList()
	model.syncDetails(true)
	return model, nil
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.cycleFocus(1)
			return m, nil
		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil
		}

		switch m.focus {
		case focusList:
			switch msg.String() {
			case "up", "k":
				m.move(-1)
				return m, nil
			case "down", "j":
				m.move(1)
				return m, nil
			}
		case focusDetails:
			var vpCmd tea.Cmd
			m.details, vpCmd = m.details.Update(msg)
			return m, vpCmd
		}
	}

	if m.focus == focusSearch {
		var cmd tea.Cmd
		prev := m.search.Value()
		m.search, cmd = m.search.Update(msg)
		if m.search.Value() != prev {
			m.applyFilter()
		}
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.err != nil {
		return appStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	header := []string{
		titleStyle.Render("Claude Session Viewer"),
		mutedStyle.Render(fmt.Sprintf("%d sessions", len(m.filtered))),
	}
	if m.projectFolder != "" {
		header = append(header, mutedStyle.Render(m.projectFolder))
	}

	leftWidth, rightWidth, panelHeight := m.panelDimensions()
	list := m.panelStyle(focusList).Width(leftWidth).Height(panelHeight).Render(m.list.View())
	detail := m.panelStyle(focusDetails).Width(rightWidth).Height(panelHeight).Render(m.details.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, list, detail)

	return appStyle.Render(strings.Join([]string{
		strings.Join(header, "  "),
		m.search.View(),
		body,
		mutedStyle.Render("Controls: Tab cycles focus, j/k or arrows move or scroll in the focused pane, q quits"),
	}, "\n\n"))
}

func (m *Model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if query == "" {
		m.filtered = m.sessions
		m.selected = 0
		m.syncList()
		m.syncDetails(true)
		return
	}

	filtered := make([]claude.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		if strings.Contains(strings.ToLower(session.Summary), query) || strings.Contains(session.SearchText, query) {
			filtered = append(filtered, session)
		}
	}
	m.filtered = filtered
	m.selected = 0
	m.syncList()
	m.syncDetails(true)
}

func (m *Model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.filtered) {
		m.selected = len(m.filtered) - 1
	}
	m.syncList()
	m.syncDetails(true)
}

func (m *Model) syncList() {
	m.list.SetContent(m.renderListContent(max(1, m.list.Width)))
	m.ensureListSelectionVisible()
}

func (m *Model) syncDetails(resetScroll bool) {
	if len(m.filtered) == 0 {
		m.details.SetContent("No sessions matched the current filter.")
		if resetScroll {
			m.details.GotoTop()
		}
		return
	}

	selected := m.filtered[m.selected]
	lines := []string{
		titleStyle.Render(selected.Summary),
		wrapLabelValue("Session", selected.ID, max(20, m.details.Width)),
		wrapLabelValue("Updated", formatTime(selected.UpdatedAt), max(20, m.details.Width)),
	}
	if !selected.StartedAt.IsZero() {
		lines = append(lines, wrapLabelValue("Started", formatTime(selected.StartedAt), max(20, m.details.Width)))
	}
	if selected.Branch != "" {
		lines = append(lines, wrapLabelValue("Branch", selected.Branch, max(20, m.details.Width)))
	}
	if selected.CWD != "" {
		lines = append(lines, wrapLabelValue("CWD", selected.CWD, max(20, m.details.Width)))
	}
	lines = append(lines,
		fmt.Sprintf("Messages: %d total, %d user, %d assistant", selected.MessageCount, selected.UserPrompts, selected.AssistantMsgs),
		wrapLabelValue("File", selected.Path, max(20, m.details.Width)),
		"",
		titleStyle.Render("Full Session Log"),
	)

	for _, entry := range selected.Transcript {
		label := entry.Role
		if label == "" {
			label = entry.Type
		}
		header := fmt.Sprintf("[%s] %s", strings.ToUpper(label), formatTime(entry.Timestamp))
		lines = append(lines, header)
		lines = append(lines, wrapText(entry.Content, max(20, m.details.Width)))
		lines = append(lines, "")
	}

	m.details.SetContent(strings.Join(lines, "\n"))
	if resetScroll {
		m.details.GotoTop()
	}
}

func (m *Model) resize() {
	leftWidth, rightWidth, panelHeight := m.panelDimensions()
	horizontalFrame, verticalFrame := listStyle.GetFrameSize()
	m.list.Width = max(1, leftWidth-horizontalFrame)
	m.list.Height = max(1, panelHeight-verticalFrame)
	m.details.Width = max(1, rightWidth-horizontalFrame)
	m.details.Height = max(1, panelHeight-verticalFrame)
	m.syncList()
	m.syncDetails(false)
}

func (m Model) panelDimensions() (leftWidth, rightWidth, panelHeight int) {
	leftWidth = max(32, m.width/3)
	rightWidth = max(40, m.width-leftWidth-8)
	panelHeight = max(8, m.height-10)
	return leftWidth, rightWidth, panelHeight
}

func (m *Model) cycleFocus(delta int) {
	targets := []focusTarget{focusSearch, focusList, focusDetails}
	index := 0
	for i, target := range targets {
		if target == m.focus {
			index = i
			break
		}
	}
	index = (index + delta + len(targets)) % len(targets)
	m.focus = targets[index]
	if m.focus == focusSearch {
		m.search.Focus()
		return
	}
	m.search.Blur()
}

func (m Model) panelStyle(target focusTarget) lipgloss.Style {
	style := listStyle
	if target == focusDetails {
		style = detailStyle
	}
	if m.focus == target {
		return style.BorderForeground(lipgloss.Color("69"))
	}
	return style
}

func (m Model) renderListContent(width int) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Width(width).Render("No sessions found.")
	}

	lines := make([]string, 0, len(m.filtered))
	for i, session := range m.filtered {
		item := fmt.Sprintf("%s\n%s", truncate(session.Summary, width), mutedStyle.Render(sessionMeta(session, width)))
		if i == m.selected {
			lines = append(lines, selectedStyle.Width(width).Render(item))
			continue
		}
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(item))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) ensureListSelectionVisible() {
	if len(m.filtered) == 0 || m.list.Height <= 0 {
		m.list.GotoTop()
		return
	}

	itemHeight := 2
	top := m.selected * itemHeight
	bottom := top + itemHeight
	visibleTop := m.list.YOffset
	visibleBottom := visibleTop + m.list.Height

	if top < visibleTop {
		m.list.SetYOffset(top)
		return
	}
	if bottom > visibleBottom {
		m.list.SetYOffset(bottom - m.list.Height)
	}
}

func sessionMeta(session claude.Session, width int) string {
	bits := []string{formatTime(session.UpdatedAt)}
	if session.Branch != "" {
		bits = append(bits, session.Branch)
	}
	return truncate(strings.Join(bits, "  "), width)
}

func currentProjectDir(sessions []claude.Session) string {
	if len(sessions) == 0 {
		return ""
	}
	return sessions[0].ProjectPath
}

func formatTime(ts time.Time) string {
	if ts.IsZero() {
		return "unknown"
	}
	return ts.Local().Format("2006-01-02 15:04")
}

func truncate(value string, width int) string {
	if width <= 3 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-3]) + "..."
}

func wrapLabelValue(label, value string, width int) string {
	return wrapText(fmt.Sprintf("%s: %s", label, value), width)
}

func wrapText(value string, width int) string {
	if width <= 0 {
		return value
	}

	sourceLines := strings.Split(strings.TrimRight(value, "\n"), "\n")
	wrapped := make([]string, 0, len(sourceLines))
	for _, line := range sourceLines {
		if strings.TrimSpace(line) == "" {
			wrapped = append(wrapped, "")
			continue
		}

		words := strings.Fields(line)
		if len(words) == 0 {
			wrapped = append(wrapped, "")
			continue
		}

		current := words[0]
		for _, word := range words[1:] {
			if len([]rune(current))+1+len([]rune(word)) > width {
				wrapped = append(wrapped, current)
				current = word
				continue
			}
			current += " " + word
		}
		wrapped = append(wrapped, current)
	}

	return strings.Join(wrapped, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
