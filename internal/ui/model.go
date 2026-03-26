package ui

import (
	"fmt"
	"strings"
	"time"

	"ccsessions/internal/claude"

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
	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("111"))
	mutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
	promptBlockStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("236")).
				Padding(0, 1)
	assistantBlockStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("223")).
			Background(lipgloss.Color("237")).
			Padding(0, 1)
	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)
	toolErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("124")).
			Padding(0, 1)
	progressStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("109"))
	metaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)
	listStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	detailStyle      = listStyle
	searchMatchStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("58")).
				Foreground(lipgloss.Color("230"))
	searchMatchActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Background(lipgloss.Color("220")).
				Foreground(lipgloss.Color("16"))
)

type focusTarget int

const (
	focusSearch focusTarget = iota
	focusList
	focusDetails
)

type detailLineKind int

const (
	detailLinePlain detailLineKind = iota
	detailLineTitle
	detailLineSectionTitle
	detailLineMuted
	detailLinePromptHeader
	detailLinePromptBody
	detailLineAssistantHeader
	detailLineAssistantBody
	detailLineToolCallHeader
	detailLineToolCallBody
	detailLineToolResultHeader
	detailLineToolResultBody
	detailLineToolErrorBody
	detailLineProgress
	detailLineMeta
)

type detailLine struct {
	text string
	kind detailLineKind
}

type detailMatch struct {
	line  int
	start int
	end   int
}

type Model struct {
	search              textinput.Model
	detailSearch        textinput.Model
	list                viewport.Model
	details             viewport.Model
	sessions            []claude.Session
	filtered            []claude.Session
	selected            int
	width               int
	height              int
	err                 error
	focus               focusTarget
	projectFolder       string
	debug               bool
	discovery           claude.DiscoveryInfo
	detailLines         []detailLine
	detailMatches       []detailMatch
	detailSearchActive  bool
	detailSearchEditing bool
	activeDetailMatch   int
}

func NewModel(claudeDir string, debug bool) (Model, error) {
	search := textinput.New()
	search.Placeholder = "Search session history"
	search.Prompt = "Search: "
	search.Focus()

	sessions, discovery, err := claude.DiscoverForCurrentDir(claudeDir)
	if err != nil {
		return Model{}, err
	}

	model := Model{
		search:       search,
		detailSearch: newDetailSearchInput(),
		sessions:     sessions,
		filtered:     sessions,
		focus:        focusSearch,
		debug:        debug,
		discovery:    discovery,
	}
	model.projectFolder = currentProjectDir(sessions)
	model.list = viewport.New(0, 0)
	model.details = viewport.New(0, 0)
	model.syncList()
	model.syncDetails(true)
	return model, nil
}

func newDetailSearchInput() textinput.Model {
	search := textinput.New()
	search.Prompt = "/"
	search.Placeholder = "search"
	search.CharLimit = 0
	return search
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
			if handled, cmd := m.updateDetailSearch(msg); handled {
				return m, cmd
			}

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

func (m *Model) updateDetailSearch(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.activateDetailSearch()
		return true, nil
	case "esc":
		if m.detailSearchEditing || m.detailSearchActive {
			m.clearDetailSearch()
			return true, nil
		}
	case "enter":
		if m.detailSearchEditing {
			m.detailSearchEditing = false
			m.detailSearch.Blur()
			m.detailSearchActive = strings.TrimSpace(m.detailSearch.Value()) != ""
			m.refreshDetailViewport(false)
			return true, nil
		}
	case "up":
		if m.detailSearchActive {
			m.moveDetailMatch(-1)
			return true, nil
		}
	case "down":
		if m.detailSearchActive {
			m.moveDetailMatch(1)
			return true, nil
		}
	}

	if !m.detailSearchEditing {
		return false, nil
	}

	var cmd tea.Cmd
	prev := m.detailSearch.Value()
	m.detailSearch, cmd = m.detailSearch.Update(msg)
	if m.detailSearch.Value() != prev {
		m.detailSearchActive = strings.TrimSpace(m.detailSearch.Value()) != ""
		m.recomputeDetailMatches(true)
	}
	return true, cmd
}

func (m *Model) activateDetailSearch() {
	m.detailSearchEditing = true
	m.detailSearchActive = strings.TrimSpace(m.detailSearch.Value()) != ""
	m.detailSearch.Focus()
	m.refreshDetailViewport(false)
}

func (m *Model) clearDetailSearch() {
	m.detailSearch.SetValue("")
	m.detailSearchActive = false
	m.detailSearchEditing = false
	m.detailSearch.Blur()
	m.detailMatches = nil
	m.activeDetailMatch = 0
	m.refreshDetailViewport(false)
}

func (m *Model) moveDetailMatch(delta int) {
	if len(m.detailMatches) == 0 {
		return
	}
	m.activeDetailMatch = (m.activeDetailMatch + delta + len(m.detailMatches)) % len(m.detailMatches)
	m.scrollToActiveMatch()
	m.refreshDetailViewport(false)
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
	if m.debug {
		header = append(header, mutedStyle.Render(m.debugSummary()))
	}

	leftWidth, rightWidth, panelHeight := m.panelDimensions()
	list := m.panelStyle(focusList).Width(leftWidth).Height(panelHeight).Render(m.list.View())
	detail := m.panelStyle(focusDetails).Width(rightWidth).Height(panelHeight).Render(m.renderDetailPanel())

	body := lipgloss.JoinHorizontal(lipgloss.Top, list, detail)

	return appStyle.Render(strings.Join([]string{
		strings.Join(header, "  "),
		m.search.View(),
		body,
		mutedStyle.Render("Controls: Tab cycles focus, j/k or arrows move or scroll, / searches session log, Enter keeps search, Esc clears, q quits"),
	}, "\n\n"))
}

func (m Model) debugSummary() string {
	projectState := "missing"
	if m.discovery.ProjectFound {
		projectState = "found"
	}

	return fmt.Sprintf(
		"debug claude_dir=%s project_dir=%s project=%s sessions=%d",
		m.discovery.ClaudeDir,
		m.discovery.ProjectDir,
		projectState,
		m.discovery.SessionCount,
	)
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
	m.detailLines = m.buildDetailLines()
	m.recomputeDetailMatches(resetScroll)
}

func (m *Model) buildDetailLines() []detailLine {
	width := max(20, m.details.Width)

	if len(m.filtered) == 0 {
		return []detailLine{{text: "No sessions matched the current filter.", kind: detailLineMuted}}
	}

	selected := m.filtered[m.selected]
	lines := []detailLine{
		{text: selected.Summary, kind: detailLineTitle},
	}

	lines = append(lines, wrapDetailLabelValue("Session", selected.ID, width, detailLinePlain)...)
	lines = append(lines, wrapDetailLabelValue("Updated", formatTime(selected.UpdatedAt), width, detailLinePlain)...)
	if !selected.StartedAt.IsZero() {
		lines = append(lines, wrapDetailLabelValue("Started", formatTime(selected.StartedAt), width, detailLinePlain)...)
	}
	if selected.Branch != "" {
		lines = append(lines, wrapDetailLabelValue("Branch", selected.Branch, width, detailLinePlain)...)
	}
	if selected.CWD != "" {
		lines = append(lines, wrapDetailLabelValue("CWD", selected.CWD, width, detailLinePlain)...)
	}

	lines = append(lines,
		detailLine{text: fmt.Sprintf("Messages: %d total, %d user, %d assistant", selected.MessageCount, selected.UserPrompts, selected.AssistantMsgs), kind: detailLinePlain},
	)
	lines = append(lines, wrapDetailLabelValue("File", selected.Path, width, detailLinePlain)...)
	lines = append(lines,
		detailLine{text: "", kind: detailLinePlain},
		detailLine{text: "Full Session Log", kind: detailLineSectionTitle},
	)

	for _, entry := range selected.Transcript {
		lines = append(lines, m.renderEntryLines(entry)...)
		lines = append(lines, detailLine{text: "", kind: detailLinePlain})
	}

	return lines
}

func (m *Model) refreshDetailViewport(resetScroll bool) {
	m.details.SetContent(m.renderDetailViewportContent())
	if resetScroll {
		m.details.GotoTop()
		return
	}
	if m.detailSearchActive && len(m.detailMatches) > 0 {
		m.scrollToActiveMatch()
		return
	}
	m.clampDetailOffset()
}

func (m *Model) recomputeDetailMatches(resetSelection bool) {
	query := strings.TrimSpace(m.detailSearch.Value())
	if query == "" {
		m.detailMatches = nil
		m.activeDetailMatch = 0
		m.refreshDetailViewport(resetSelection)
		return
	}

	m.detailMatches = findDetailMatches(m.detailLines, query)
	if resetSelection || m.activeDetailMatch >= len(m.detailMatches) {
		m.activeDetailMatch = 0
	}
	m.refreshDetailViewport(resetSelection && len(m.detailMatches) == 0)
}

func (m *Model) resize() {
	leftWidth, rightWidth, panelHeight := m.panelDimensions()
	horizontalFrame, verticalFrame := listStyle.GetFrameSize()
	m.list.Width = max(1, leftWidth-horizontalFrame)
	m.list.Height = max(1, panelHeight-verticalFrame)
	m.details.Width = max(1, rightWidth-horizontalFrame)
	m.details.Height = max(1, panelHeight-verticalFrame-1)
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
	if m.focus != focusDetails {
		m.detailSearchEditing = false
		m.detailSearch.Blur()
	}
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

func (m Model) renderDetailPanel() string {
	return strings.Join([]string{
		m.details.View(),
		m.renderDetailFooter(),
	}, "\n")
}

func (m Model) renderDetailFooter() string {
	width := max(1, m.details.Width)
	if !m.detailSearchActive && !m.detailSearchEditing {
		return mutedStyle.Width(width).Render("Press / to search this session log")
	}

	left := m.detailSearch.View()
	right := fmt.Sprintf("%d of %d", m.activeDetailMatch+1, len(m.detailMatches))
	if len(m.detailMatches) == 0 {
		right = "0 of 0"
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	if leftWidth+rightWidth >= width {
		return lipgloss.NewStyle().Width(width).Render(left)
	}

	padding := strings.Repeat(" ", width-leftWidth-rightWidth)
	return lipgloss.NewStyle().Width(width).Render(left + padding + mutedStyle.Render(right))
}

func (m Model) renderDetailViewportContent() string {
	if len(m.detailLines) == 0 {
		return ""
	}

	matchesByLine := groupMatchesByLine(m.detailMatches)
	lines := make([]string, 0, len(m.detailLines))
	for index, line := range m.detailLines {
		lines = append(lines, renderDetailLine(line, matchesByLine[index], m.activeDetailMatch))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) scrollToActiveMatch() {
	if len(m.detailMatches) == 0 {
		return
	}

	line := m.detailMatches[m.activeDetailMatch].line
	maxOffset := max(0, len(m.detailLines)-m.details.Height)
	if line > maxOffset {
		line = maxOffset
	}
	m.details.SetYOffset(line)
}

func (m *Model) clampDetailOffset() {
	maxOffset := max(0, len(m.detailLines)-m.details.Height)
	if m.details.YOffset > maxOffset {
		m.details.SetYOffset(maxOffset)
	}
	if m.details.YOffset < 0 {
		m.details.SetYOffset(0)
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

func wrapDetailLabelValue(label, value string, width int, kind detailLineKind) []detailLine {
	return wrapDetailText(fmt.Sprintf("%s: %s", label, value), width, kind)
}

func wrapDetailText(value string, width int, kind detailLineKind) []detailLine {
	lines := wrapTextLines(value, width)
	out := make([]detailLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, detailLine{text: line, kind: kind})
	}
	return out
}

func wrapTextLines(value string, width int) []string {
	if width <= 0 {
		return []string{value}
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

	if len(wrapped) == 0 {
		return []string{""}
	}
	return wrapped
}

func (m Model) renderEntryLines(entry claude.Entry) []detailLine {
	width := max(20, m.details.Width)
	switch entry.Kind {
	case claude.EntryHumanPrompt:
		return renderPromptLines(entry, width)
	case claude.EntryAssistantText:
		return renderAssistantLines(entry, width)
	case claude.EntryToolCall:
		return renderToolCallLines(entry, width)
	case claude.EntryToolResult:
		return renderToolResultLines(entry, width)
	case claude.EntryThinking:
		return renderThinkingLines(entry, width)
	case claude.EntryProgress:
		return renderProgressLines(entry, width)
	case claude.EntryMeta:
		return renderMetaLines(entry, width)
	default:
		return wrapDetailText(entry.Content, width, detailLinePlain)
	}
}

func renderPromptLines(entry claude.Entry, width int) []detailLine {
	lines := []detailLine{{
		text: fmt.Sprintf("Prompt  %s", formatTime(entry.Timestamp)),
		kind: detailLinePromptHeader,
	}}
	lines = append(lines, wrapDetailText(entry.Content, max(10, width-4), detailLinePromptBody)...)
	return lines
}

func renderAssistantLines(entry claude.Entry, width int) []detailLine {
	lines := []detailLine{{
		text: fmt.Sprintf("Assistant  %s", formatTime(entry.Timestamp)),
		kind: detailLineAssistantHeader,
	}}
	lines = append(lines, wrapDetailText(entry.Content, width, detailLineAssistantBody)...)
	return lines
}

func renderToolCallLines(entry claude.Entry, width int) []detailLine {
	title := firstNonEmpty(entry.Title, "Tool Call")
	lines := []detailLine{{
		text: fmt.Sprintf("%s  %s", title, formatTime(entry.Timestamp)),
		kind: detailLineToolCallHeader,
	}}
	lines = append(lines, wrapDetailText(entry.Content, max(10, width-2), detailLineToolCallBody)...)
	return lines
}

func renderToolResultLines(entry claude.Entry, width int) []detailLine {
	headerLabel := "Tool Result"
	bodyKind := detailLineToolResultBody
	if entry.IsError {
		headerLabel = "Tool Error"
		bodyKind = detailLineToolErrorBody
	}

	lines := []detailLine{{
		text: fmt.Sprintf("%s  %s", headerLabel, formatTime(entry.Timestamp)),
		kind: detailLineToolResultHeader,
	}}
	lines = append(lines, wrapDetailText(entry.Content, max(10, width-2), bodyKind)...)
	return lines
}

func renderThinkingLines(entry claude.Entry, width int) []detailLine {
	label := firstNonEmpty(entry.Title, "Thinking")
	return []detailLine{{
		text: truncate(label+"  "+formatTime(entry.Timestamp), width),
		kind: detailLineProgress,
	}}
}

func renderProgressLines(entry claude.Entry, width int) []detailLine {
	label := firstNonEmpty(entry.Title, "Progress")
	text := label
	if strings.TrimSpace(entry.Content) != "" && entry.Content != label {
		text += "  " + oneLineForUI(entry.Content)
	}
	return wrapDetailText(text+"  "+formatTime(entry.Timestamp), width, detailLineProgress)
}

func renderMetaLines(entry claude.Entry, width int) []detailLine {
	label := firstNonEmpty(entry.Title, "Meta")
	text := label
	if strings.TrimSpace(entry.Content) != "" {
		text += ": " + oneLineForUI(entry.Content)
	}
	if !entry.Timestamp.IsZero() {
		text += "  " + formatTime(entry.Timestamp)
	}
	return wrapDetailText(text, width, detailLineMeta)
}

func findDetailMatches(lines []detailLine, query string) []detailMatch {
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return nil
	}

	matches := make([]detailMatch, 0)
	for lineIndex, line := range lines {
		lineRunes := []rune(line.text)
		for start := 0; start+len(queryRunes) <= len(lineRunes); start++ {
			if !strings.EqualFold(string(lineRunes[start:start+len(queryRunes)]), query) {
				continue
			}
			matches = append(matches, detailMatch{
				line:  lineIndex,
				start: start,
				end:   start + len(queryRunes),
			})
			start += len(queryRunes) - 1
		}
	}

	return matches
}

func groupMatchesByLine(matches []detailMatch) map[int][]detailMatch {
	if len(matches) == 0 {
		return nil
	}

	grouped := make(map[int][]detailMatch, len(matches))
	for index, match := range matches {
		grouped[match.line] = append(grouped[match.line], detailMatch{
			line:  index,
			start: match.start,
			end:   match.end,
		})
	}
	return grouped
}

func renderDetailLine(line detailLine, matches []detailMatch, activeMatch int) string {
	base := detailLineStyle(line.kind)
	if len(matches) == 0 {
		return base.Render(line.text)
	}

	var parts []string
	runes := []rune(line.text)
	cursor := 0
	for _, match := range matches {
		if match.start > cursor {
			parts = append(parts, base.Render(string(runes[cursor:match.start])))
		}

		segment := string(runes[match.start:match.end])
		style := searchMatchStyle
		if match.line == activeMatch {
			style = searchMatchActiveStyle
		}
		parts = append(parts, style.Render(segment))
		cursor = match.end
	}

	if cursor < len(runes) {
		parts = append(parts, base.Render(string(runes[cursor:])))
	}

	return strings.Join(parts, "")
}

func detailLineStyle(kind detailLineKind) lipgloss.Style {
	switch kind {
	case detailLineTitle:
		return titleStyle
	case detailLineSectionTitle:
		return sectionTitleStyle
	case detailLineMuted, detailLinePromptHeader, detailLineAssistantHeader, detailLineToolCallHeader, detailLineToolResultHeader:
		return mutedStyle
	case detailLinePromptBody:
		return promptBlockStyle
	case detailLineAssistantBody:
		return assistantBlockStyle
	case detailLineToolCallBody:
		return toolCallStyle
	case detailLineToolResultBody:
		return toolResultStyle
	case detailLineToolErrorBody:
		return toolErrorStyle
	case detailLineProgress:
		return progressStyle
	case detailLineMeta:
		return metaStyle
	default:
		return lipgloss.NewStyle()
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func oneLineForUI(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
