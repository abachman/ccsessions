package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type EntryKind string

const (
	EntryHumanPrompt   EntryKind = "human_prompt"
	EntryAssistantText EntryKind = "assistant_text"
	EntryToolCall      EntryKind = "tool_call"
	EntryToolResult    EntryKind = "tool_result"
	EntryThinking      EntryKind = "thinking_marker"
	EntryProgress      EntryKind = "progress_status"
	EntryMeta          EntryKind = "meta_event"
)

type Session struct {
	ID            string
	Path          string
	ProjectPath   string
	Branch        string
	Version       string
	CWD           string
	StartedAt     time.Time
	UpdatedAt     time.Time
	MessageCount  int
	UserPrompts   int
	AssistantMsgs int
	Summary       string
	SearchText    string
	Transcript    []Entry
}

type Entry struct {
	Timestamp    time.Time
	Type         string
	Subtype      string
	Role         string
	Kind         EntryKind
	Title        string
	Content      string
	ToolID       string
	ParentToolID string
	IsError      bool
}

type rawRecord struct {
	Type             string          `json:"type"`
	Subtype          string          `json:"subtype"`
	SessionID        string          `json:"sessionId"`
	Timestamp        string          `json:"timestamp"`
	CWD              string          `json:"cwd"`
	GitBranch        string          `json:"gitBranch"`
	Version          string          `json:"version"`
	Message          json.RawMessage `json:"message"`
	Content          json.RawMessage `json:"content"`
	Data             json.RawMessage `json:"data"`
	Summary          string          `json:"summary"`
	LastPrompt       string          `json:"lastPrompt"`
	CustomTitle      string          `json:"customTitle"`
	AgentName        string          `json:"agentName"`
	Operation        string          `json:"operation"`
	PRURL            string          `json:"prUrl"`
	PRRepository     string          `json:"prRepository"`
	PRNumber         int             `json:"prNumber"`
	DurationMS       int             `json:"durationMs"`
	MessageID        string          `json:"messageId"`
	IsSnapshotUpdate bool            `json:"isSnapshotUpdate"`
	Snapshot         *rawSnapshot    `json:"snapshot"`
}

type rawSnapshot struct {
	Timestamp string `json:"timestamp"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type rawContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	Signature string          `json:"signature"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type rawProgress struct {
	Type               string          `json:"type"`
	Output             string          `json:"output"`
	FullOutput         string          `json:"fullOutput"`
	ElapsedTimeSeconds int             `json:"elapsedTimeSeconds"`
	TotalLines         int             `json:"totalLines"`
	HookEvent          string          `json:"hookEvent"`
	HookName           string          `json:"hookName"`
	Command            string          `json:"command"`
	Status             string          `json:"status"`
	ServerName         string          `json:"serverName"`
	ToolName           string          `json:"toolName"`
	Query              string          `json:"query"`
	ResultCount        int             `json:"resultCount"`
	TaskDescription    string          `json:"taskDescription"`
	TaskType           string          `json:"taskType"`
	Prompt             string          `json:"prompt"`
	AgentID            string          `json:"agentId"`
	Message            json.RawMessage `json:"message"`
}

func DiscoverForCurrentDir() ([]Session, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	root, err := projectHistoryDir(cwd)
	if err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(filepath.Join(root, "*.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("glob session files: %w", err)
	}

	sessions := make([]Session, 0, len(matches))
	for _, match := range matches {
		session, err := ParseSessionFile(match)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

func projectHistoryDir(cwd string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	sanitized := strings.ReplaceAll(filepath.Clean(cwd), string(filepath.Separator), "-")
	return filepath.Join(home, ".claude", "projects", sanitized), nil
}

func ParseSessionFile(path string) (session Session, err error) {
	file, err := os.Open(path)
	if err != nil {
		return Session{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			closeErr = fmt.Errorf("close %s: %w", path, closeErr)
			if err != nil {
				err = errors.Join(err, closeErr)
				return
			}
			err = closeErr
		}
	}()

	session = Session{
		ID:          strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
		Path:        path,
		ProjectPath: filepath.Dir(path),
	}

	var searchParts []string
	var firstUser string

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	for scanner.Scan() {
		var record rawRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return Session{}, fmt.Errorf("parse %s: %w", path, err)
		}

		if record.SessionID != "" {
			session.ID = record.SessionID
		}
		if record.CWD != "" {
			session.CWD = record.CWD
		}
		if record.GitBranch != "" {
			session.Branch = record.GitBranch
		}
		if record.Version != "" {
			session.Version = record.Version
		}

		ts := parseTime(record.Timestamp)
		if ts.IsZero() && record.Snapshot != nil {
			ts = parseTime(record.Snapshot.Timestamp)
		}
		if !ts.IsZero() {
			if session.StartedAt.IsZero() || ts.Before(session.StartedAt) {
				session.StartedAt = ts
			}
			if ts.After(session.UpdatedAt) {
				session.UpdatedAt = ts
			}
		}

		entries := normalizeRecord(record, ts)
		for _, entry := range entries {
			if entry.Kind == EntryHumanPrompt {
				session.UserPrompts++
				if firstUser == "" && !strings.Contains(entry.Content, "<local-command-caveat>") {
					firstUser = oneLine(entry.Content)
				}
			}
			if entry.Kind == EntryAssistantText || entry.Kind == EntryToolCall || entry.Kind == EntryThinking {
				session.AssistantMsgs++
			}
			if isCountedEntry(entry) {
				session.MessageCount++
			}
			if text := entrySearchText(entry); text != "" {
				searchParts = append(searchParts, text)
			}
		}
		session.Transcript = append(session.Transcript, entries...)
	}

	if err := scanner.Err(); err != nil {
		return Session{}, fmt.Errorf("scan %s: %w", path, err)
	}

	session.Summary = firstUser
	if session.Summary == "" {
		session.Summary = fallbackSummary(session.Transcript)
	}
	if session.Summary == "" {
		session.Summary = "(no user prompt found)"
	}
	session.SearchText = strings.ToLower(strings.Join(searchParts, "\n"))

	return session, nil
}

func normalizeRecord(record rawRecord, ts time.Time) []Entry {
	switch record.Type {
	case "user", "assistant":
		return normalizeMessageRecord(record, ts)
	case "progress":
		entry := normalizeProgressRecord(record, ts)
		if entry == nil {
			return nil
		}
		return []Entry{*entry}
	default:
		entry := normalizeMetaRecord(record, ts)
		if entry == nil {
			return nil
		}
		return []Entry{*entry}
	}
}

func normalizeMessageRecord(record rawRecord, ts time.Time) []Entry {
	if len(record.Message) == 0 {
		return nil
	}

	var msg rawMessage
	if json.Unmarshal(record.Message, &msg) != nil {
		return nil
	}

	if len(msg.Content) == 0 {
		return nil
	}

	var text string
	if json.Unmarshal(msg.Content, &text) == nil {
		entry := Entry{
			Timestamp: ts,
			Type:      record.Type,
			Role:      msg.Role,
			Kind:      kindForTextRole(msg.Role),
			Content:   text,
		}
		return []Entry{entry}
	}

	var blocks []rawContentBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}

	entries := make([]Entry, 0, len(blocks))
	for _, block := range blocks {
		entry := normalizeContentBlock(record, msg.Role, ts, block)
		if entry == nil {
			continue
		}
		entries = append(entries, *entry)
	}
	return entries
}

func normalizeContentBlock(record rawRecord, role string, ts time.Time, block rawContentBlock) *Entry {
	entry := &Entry{
		Timestamp: ts,
		Type:      record.Type,
		Subtype:   block.Type,
		Role:      role,
	}

	switch block.Type {
	case "text":
		entry.Kind = kindForTextRole(role)
		entry.Content = block.Text
	case "thinking":
		entry.Kind = EntryThinking
		entry.Title = "Thinking"
		if strings.TrimSpace(block.Thinking) != "" {
			entry.Content = block.Thinking
		}
	case "tool_use":
		entry.Kind = EntryToolCall
		entry.Title = block.Name
		entry.ToolID = block.ID
		entry.Content = summarizeToolInput(block.Name, block.Input)
	case "server_tool_use":
		entry.Kind = EntryToolCall
		entry.Title = "Server: " + block.Name
		entry.ToolID = block.ID
		entry.Content = summarizeToolInput(block.Name, block.Input)
	case "tool_result":
		entry.Kind = EntryToolResult
		entry.ParentToolID = block.ToolUseID
		entry.IsError = block.IsError
		entry.Content = flattenFlexibleContent(block.Content)
	default:
		return nil
	}

	if strings.TrimSpace(entry.Content) == "" && entry.Kind != EntryThinking {
		return nil
	}
	return entry
}

func normalizeProgressRecord(record rawRecord, ts time.Time) *Entry {
	if len(record.Data) == 0 {
		return nil
	}

	var progress rawProgress
	if json.Unmarshal(record.Data, &progress) != nil {
		return nil
	}

	entry := &Entry{
		Timestamp: ts,
		Type:      record.Type,
		Subtype:   progress.Type,
		Kind:      EntryProgress,
	}

	switch progress.Type {
	case "hook_progress":
		entry.Title = progress.HookName
		entry.Content = strings.TrimSpace(strings.Join([]string{progress.HookEvent, progress.Command}, "  "))
	case "bash_progress":
		entry.Title = "Bash Progress"
		entry.Content = firstNonEmpty(progress.Output, progress.FullOutput)
	case "agent_progress":
		entry.Title = "Agent Progress"
		entry.Content = firstNonEmpty(progress.Prompt, extractProgressMessageText(progress.Message))
		if progress.AgentID != "" {
			entry.Title = "Agent " + progress.AgentID
		}
	case "query_update":
		entry.Title = "Query Update"
		entry.Content = progress.Query
	case "search_results_received":
		entry.Title = "Search Results"
		if progress.Query != "" {
			entry.Content = fmt.Sprintf("%d results for %s", progress.ResultCount, progress.Query)
		} else {
			entry.Content = fmt.Sprintf("%d results received", progress.ResultCount)
		}
	case "waiting_for_task":
		entry.Title = "Waiting"
		entry.Content = strings.TrimSpace(strings.Join([]string{progress.TaskDescription, progress.TaskType}, "  "))
	case "mcp_progress":
		entry.Title = "MCP " + firstNonEmpty(progress.Status, "progress")
		entry.Content = strings.TrimSpace(strings.Join([]string{progress.ServerName, progress.ToolName}, "  "))
	default:
		return nil
	}

	if strings.TrimSpace(entry.Content) == "" {
		entry.Content = entry.Title
	}
	return entry
}

func normalizeMetaRecord(record rawRecord, ts time.Time) *Entry {
	entry := &Entry{
		Timestamp: ts,
		Type:      record.Type,
		Subtype:   record.Subtype,
		Kind:      EntryMeta,
	}

	switch record.Type {
	case "summary":
		entry.Title = "Summary"
		entry.Content = record.Summary
	case "last-prompt":
		entry.Title = "Last Prompt"
		entry.Content = record.LastPrompt
	case "custom-title":
		entry.Title = "Custom Title"
		entry.Content = record.CustomTitle
	case "agent-name":
		entry.Title = "Agent Name"
		entry.Content = record.AgentName
	case "queue-operation":
		entry.Title = "Queue " + firstNonEmpty(record.Operation, "operation")
		entry.Content = flattenFlexibleContent(record.Content)
	case "pr-link":
		entry.Title = "Pull Request"
		switch {
		case record.PRRepository != "" && record.PRNumber > 0:
			entry.Content = fmt.Sprintf("%s #%d", record.PRRepository, record.PRNumber)
		case record.PRURL != "":
			entry.Content = record.PRURL
		}
	case "system":
		entry.Title = firstNonEmpty(record.Subtype, "system")
		if record.DurationMS > 0 {
			entry.Content = fmt.Sprintf("%d ms", record.DurationMS)
		}
	case "file-history-snapshot":
		entry.Title = "File Snapshot"
		if record.IsSnapshotUpdate {
			entry.Content = "snapshot updated"
		} else {
			entry.Content = "snapshot created"
		}
	default:
		return nil
	}

	if strings.TrimSpace(entry.Content) == "" {
		return nil
	}
	return entry
}

func kindForTextRole(role string) EntryKind {
	if role == "assistant" {
		return EntryAssistantText
	}
	return EntryHumanPrompt
}

func summarizeToolInput(name string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		switch name {
		case "Bash":
			return joinSummaryParts(object["description"], object["command"])
		case "Read":
			return stringify(object["file_path"])
		case "Edit":
			return joinSummaryParts(object["file_path"], object["old_string"])
		case "Write":
			return joinSummaryParts(object["file_path"], previewString(object["content"], 120))
		case "Glob":
			return joinSummaryParts(object["path"], object["pattern"])
		case "Grep":
			return joinSummaryParts(object["pattern"], object["path"])
		case "WebSearch":
			return stringify(object["query"])
		case "WebFetch":
			return stringify(object["url"])
		}

		if description := stringify(object["description"]); description != "" {
			return description
		}
		if command := stringify(object["command"]); command != "" {
			return command
		}

		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			value := stringify(object[key])
			if value == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
			if len(parts) == 3 {
				break
			}
		}
		return strings.Join(parts, "  ")
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	return string(raw)
}

func flattenFlexibleContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	var blocks []rawContentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			switch block.Type {
			case "text":
				if block.Text != "" {
					parts = append(parts, block.Text)
				}
			case "tool_result":
				value := flattenFlexibleContent(block.Content)
				if value != "" {
					parts = append(parts, value)
				}
			}
		}
		return strings.Join(parts, "\n")
	}

	var object map[string]any
	if json.Unmarshal(raw, &object) == nil {
		parts := make([]string, 0, len(object))
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := stringify(object[key])
			if value == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
		return strings.Join(parts, "  ")
	}

	return ""
}

func extractProgressMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var container struct {
		Message rawMessage `json:"message"`
	}
	if json.Unmarshal(raw, &container) == nil {
		return flattenFlexibleContent(container.Message.Content)
	}
	return ""
}

func isCountedEntry(entry Entry) bool {
	switch entry.Kind {
	case EntryHumanPrompt, EntryAssistantText, EntryToolCall, EntryToolResult:
		return true
	default:
		return false
	}
}

func entrySearchText(entry Entry) string {
	switch entry.Kind {
	case EntryHumanPrompt, EntryAssistantText, EntryToolCall, EntryToolResult, EntryProgress:
		return strings.TrimSpace(strings.Join([]string{entry.Title, entry.Content}, "\n"))
	default:
		return ""
	}
}

func fallbackSummary(entries []Entry) string {
	for _, entry := range entries {
		switch entry.Kind {
		case EntryAssistantText, EntryToolCall, EntryProgress:
			if text := oneLine(firstNonEmpty(entry.Content, entry.Title)); text != "" {
				return text
			}
		}
	}
	return ""
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func oneLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 120 {
		return string(runes[:117]) + "..."
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringify(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func previewString(value any, limit int) string {
	text := stringify(value)
	return oneLine(truncateRunes(text, limit))
}

func truncateRunes(value string, limit int) string {
	if limit <= 3 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-3]) + "..."
}

func joinSummaryParts(values ...any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(stringify(value))
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "  ")
}
