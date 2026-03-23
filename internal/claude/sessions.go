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
	Timestamp time.Time
	Type      string
	Role      string
	Content   string
}

type rawRecord struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"sessionId"`
	Timestamp string          `json:"timestamp"`
	CWD       string          `json:"cwd"`
	GitBranch string          `json:"gitBranch"`
	Version   string          `json:"version"`
	Message   json.RawMessage `json:"message"`
	Content   json.RawMessage `json:"content"`
	Data      json.RawMessage `json:"data"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
		if !ts.IsZero() {
			if session.StartedAt.IsZero() || ts.Before(session.StartedAt) {
				session.StartedAt = ts
			}
			if ts.After(session.UpdatedAt) {
				session.UpdatedAt = ts
			}
		}

		role, content := extractContent(record)
		if content == "" {
			continue
		}

		entryType := record.Type
		if entryType == "" {
			entryType = record.Subtype
		}

		session.MessageCount++
		if role == "user" {
			session.UserPrompts++
			if firstUser == "" && !strings.Contains(content, "<local-command-caveat>") {
				firstUser = oneLine(content)
			}
		}
		if role == "assistant" {
			session.AssistantMsgs++
		}

		session.Transcript = append(session.Transcript, Entry{
			Timestamp: ts,
			Type:      entryType,
			Role:      role,
			Content:   content,
		})
		searchParts = append(searchParts, content)
	}

	if err := scanner.Err(); err != nil {
		return Session{}, fmt.Errorf("scan %s: %w", path, err)
	}

	session.Summary = firstUser
	if session.Summary == "" {
		session.Summary = "(no user prompt found)"
	}
	session.SearchText = strings.ToLower(strings.Join(searchParts, "\n"))

	return session, nil
}

func extractContent(record rawRecord) (string, string) {
	if len(record.Message) > 0 {
		var msg rawMessage
		if json.Unmarshal(record.Message, &msg) == nil {
			return msg.Role, flattenContent(msg.Content)
		}
	}

	if len(record.Content) > 0 {
		var text string
		if json.Unmarshal(record.Content, &text) == nil {
			return record.Type, text
		}
	}

	return record.Type, ""
}

func flattenContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	var blocks []contentBlock
	if json.Unmarshal(raw, &blocks) == nil {
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		}
		return strings.Join(parts, "\n")
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
