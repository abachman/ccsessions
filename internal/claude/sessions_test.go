package claude

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testdataPath returns the absolute path to a file in testdata/.
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

// --- projectHistoryDir ---

func TestProjectHistoryDir_DefaultClaudeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := projectHistoryDir("/tmp/my-project", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(home, ".claude", "projects", "-tmp-my-project")
	if got != want {
		t.Errorf("projectHistoryDir() = %q, want %q", got, want)
	}
}

func TestProjectHistoryDir_CustomClaudeDir(t *testing.T) {
	got, err := projectHistoryDir("/tmp/my-project", "/tmp/.claude-personal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join("/tmp/.claude-personal", "projects", "-tmp-my-project")
	if got != want {
		t.Errorf("projectHistoryDir() = %q, want %q", got, want)
	}
}

func TestProjectHistoryDir_TildeClaudeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := projectHistoryDir("/tmp/my-project", "~/.claude-personal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := filepath.Join(home, ".claude-personal", "projects", "-tmp-my-project")
	if got != want {
		t.Errorf("projectHistoryDir() = %q, want %q", got, want)
	}
}

// --- ParseSessionFile ---

func TestParseSessionFile_Simple(t *testing.T) {
	session, err := ParseSessionFile(testdataPath("simple.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID != "abc123" {
		t.Errorf("ID = %q, want %q", session.ID, "abc123")
	}
	if session.UserPrompts != 1 {
		t.Errorf("UserPrompts = %d, want 1", session.UserPrompts)
	}
	if session.AssistantMsgs != 1 {
		t.Errorf("AssistantMsgs = %d, want 1", session.AssistantMsgs)
	}
	if session.Summary != "hello world" {
		t.Errorf("Summary = %q, want %q", session.Summary, "hello world")
	}
}

func TestParseSessionFile_ToolUse(t *testing.T) {
	session, err := ParseSessionFile(testdataPath("tool_use.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolEntry *Entry
	for i := range session.Transcript {
		if session.Transcript[i].Kind == EntryToolCall {
			toolEntry = &session.Transcript[i]
			break
		}
	}
	if toolEntry == nil {
		t.Fatal("no tool_call entry found in transcript")
	}
	if toolEntry.Title != "Read" {
		t.Errorf("tool Title = %q, want %q", toolEntry.Title, "Read")
	}
	if toolEntry.Content != "/foo/bar.go" {
		t.Errorf("tool Content = %q, want %q", toolEntry.Content, "/foo/bar.go")
	}
}

func TestParseSessionFile_Empty(t *testing.T) {
	session, err := ParseSessionFile(testdataPath("empty.jsonl"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.UserPrompts != 0 {
		t.Errorf("UserPrompts = %d, want 0", session.UserPrompts)
	}
	if session.AssistantMsgs != 0 {
		t.Errorf("AssistantMsgs = %d, want 0", session.AssistantMsgs)
	}
	if session.Summary != "(no user prompt found)" {
		t.Errorf("Summary = %q, want %q", session.Summary, "(no user prompt found)")
	}
}

// --- oneLine ---

func TestOneLine(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiline collapses to single line",
			input: "line one\nline two\nline three",
			want:  "line one line two line three",
		},
		{
			name:  "long string truncated with ellipsis",
			input: strings.Repeat("a", 130),
			want:  strings.Repeat("a", 117) + "...",
		},
		{
			name:  "short string unchanged",
			input: "short",
			want:  "short",
		},
		{
			name:  "exactly 120 runes unchanged",
			input: strings.Repeat("x", 120),
			want:  strings.Repeat("x", 120),
		},
		{
			name:  "121 runes truncated",
			input: strings.Repeat("x", 121),
			want:  strings.Repeat("x", 117) + "...",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := oneLine(tc.input)
			if got != tc.want {
				t.Errorf("oneLine(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- truncateRunes ---

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		limit int
		want  string
	}{
		{
			name:  "shorter than limit unchanged",
			input: "hello",
			limit: 10,
			want:  "hello",
		},
		{
			name:  "longer than limit truncated with ellipsis",
			input: "hello world",
			limit: 8,
			want:  "hello...",
		},
		{
			name:  "limit <= 3 returns value unchanged",
			input: "hello",
			limit: 3,
			want:  "hello",
		},
		{
			name:  "limit 1 returns value unchanged",
			input: "hello",
			limit: 1,
			want:  "hello",
		},
		{
			name:  "exactly at limit unchanged",
			input: "hello",
			limit: 5,
			want:  "hello",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.input, tc.limit)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.input, tc.limit, got, tc.want)
			}
		})
	}
}

// --- summarizeToolInput ---

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return b
}

func TestSummarizeToolInput(t *testing.T) {
	cases := []struct {
		name         string
		toolName     string
		input        map[string]any
		wantContains string // substring that must appear in result
		wantExact    string // exact match (if set, checked instead of wantContains)
	}{
		{
			name:      "Bash with command and description",
			toolName:  "Bash",
			input:     map[string]any{"command": "ls -la", "description": "list files"},
			wantExact: "list files  ls -la",
		},
		{
			name:      "Read with file_path",
			toolName:  "Read",
			input:     map[string]any{"file_path": "/some/path.go"},
			wantExact: "/some/path.go",
		},
		{
			name:      "WebSearch with query",
			toolName:  "WebSearch",
			input:     map[string]any{"query": "golang testing"},
			wantExact: "golang testing",
		},
		{
			name:         "unknown tool falls back to key=value pairs",
			toolName:     "UnknownTool",
			input:        map[string]any{"alpha": "val1", "beta": "val2"},
			wantContains: "alpha=val1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := mustMarshal(t, tc.input)
			got := summarizeToolInput(tc.toolName, raw)
			if tc.wantExact != "" {
				if got != tc.wantExact {
					t.Errorf("summarizeToolInput(%q) = %q, want %q", tc.toolName, got, tc.wantExact)
				}
			} else if !strings.Contains(got, tc.wantContains) {
				t.Errorf("summarizeToolInput(%q) = %q, want it to contain %q", tc.toolName, got, tc.wantContains)
			}
		})
	}
}

// --- normalizeRecord ---

func makeTS() time.Time {
	ts, _ := time.Parse(time.RFC3339, "2024-01-01T10:00:00Z")
	return ts
}

func TestNormalizeRecord_UserStringContent(t *testing.T) {
	record := rawRecord{
		Type:    "user",
		Message: json.RawMessage(`{"role":"user","content":"hello"}`),
	}
	entries := normalizeRecord(record, makeTS())
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Kind != EntryHumanPrompt {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, EntryHumanPrompt)
	}
	if entries[0].Content != "hello" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "hello")
	}
}

func TestNormalizeRecord_AssistantTextBlock(t *testing.T) {
	record := rawRecord{
		Type:    "assistant",
		Message: json.RawMessage(`{"role":"assistant","content":[{"type":"text","text":"good morning"}]}`),
	}
	entries := normalizeRecord(record, makeTS())
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Kind != EntryAssistantText {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, EntryAssistantText)
	}
	if entries[0].Content != "good morning" {
		t.Errorf("Content = %q, want %q", entries[0].Content, "good morning")
	}
}

func TestNormalizeRecord_ProgressBashProgress(t *testing.T) {
	record := rawRecord{
		Type: "progress",
		Data: json.RawMessage(`{"type":"bash_progress","output":"running tests..."}`),
	}
	entries := normalizeRecord(record, makeTS())
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Kind != EntryProgress {
		t.Errorf("Kind = %q, want %q", entries[0].Kind, EntryProgress)
	}
	if entries[0].Content != "running tests..." {
		t.Errorf("Content = %q, want %q", entries[0].Content, "running tests...")
	}
}

func TestNormalizeRecord_UnknownType(t *testing.T) {
	record := rawRecord{
		Type: "totally_unknown_xyz",
	}
	entries := normalizeRecord(record, makeTS())
	if len(entries) != 0 {
		t.Errorf("got %d entries for unknown type, want 0", len(entries))
	}
}
