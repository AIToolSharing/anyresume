package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// titleRecord writes a title record in the shape that Claude Code uses: the
// "type" member first.
func titleRecord(kind, value string) []byte {
	var v any
	switch kind {
	case "custom-title":
		v = struct {
			Type        string `json:"type"`
			CustomTitle string `json:"customTitle"`
			SessionID   string `json:"sessionId"`
		}{kind, value, "s"}
	case "ai-title":
		v = struct {
			Type    string `json:"type"`
			AITitle string `json:"aiTitle"`
		}{kind, value}
	case "agent-name":
		v = struct {
			Type      string `json:"type"`
			AgentName string `json:"agentName"`
		}{kind, value}
	case "summary":
		v = struct {
			Type    string `json:"type"`
			Summary string `json:"summary"`
		}{kind, value}
	case "last-prompt":
		v = struct {
			Type       string `json:"type"`
			LastPrompt string `json:"lastPrompt"`
		}{kind, value}
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// userRecord writes a user message in the shape that Claude Code uses:
// "type" before the message content, and "cwd" after it.
func userRecord(content, cwd string) []byte {
	b, err := json.Marshal(struct {
		ParentUUID *string `json:"parentUuid"`
		Type       string  `json:"type"`
		Message    struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Cwd       string `json:"cwd"`
		SessionID string `json:"sessionId"`
	}{Type: "user", Message: struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}{"user", content}, Cwd: cwd, SessionID: "s"})
	if err != nil {
		panic(err)
	}
	return b
}

// fataler is the part of testing.T and rapid.T that scanLines uses.
type fataler interface {
	Helper()
	Fatalf(format string, args ...any)
}

func scanLines(t fataler, lines [][]byte) Transcript {
	t.Helper()
	var tr Transcript
	if err := tr.scan(bytes.NewReader(append(bytes.Join(lines, []byte("\n")), '\n'))); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return tr
}

// TestTitlePriority checks the title against a reference model: the last
// non-blank value of the most important record kind.
func TestTitlePriority(t *testing.T) {
	kinds := []string{"custom-title", "ai-title", "agent-name", "summary", "last-prompt"}
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(0, 12).Draw(rt, "n")
		last := map[string]string{}
		var lines [][]byte
		for i := 0; i < n; i++ {
			kind := rapid.SampledFrom(kinds).Draw(rt, "kind")
			value := rapid.String().Draw(rt, "value")
			lines = append(lines, titleRecord(kind, value))
			if strings.TrimSpace(value) != "" {
				last[kind] = value
			}
		}
		want := ""
		for _, k := range kinds {
			if v, ok := last[k]; ok {
				want = v
				break
			}
		}
		tr := scanLines(rt, lines)
		if got := tr.Title(); got != want {
			rt.Fatalf("Title() = %q, want %q", got, want)
		}
	})
}

// TestCwdAcrossChunks checks that the reader finds "cwd" in a long message
// line when the key or the value crosses a chunk border. The content length
// puts the key at most one record length before, or 64 bytes after, the
// first or second border.
func TestCwdAcrossChunks(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		cwd := rapid.StringN(1, 300, -1).Draw(rt, "cwd")
		empty := userRecord("", cwd)
		base := bytes.Index(empty, cwdKey)
		border := rapid.IntRange(1, 2).Draw(rt, "border") * chunkSize
		shift := rapid.IntRange(-len(empty), 64).Draw(rt, "shift")
		size := max(0, border+shift-base)
		content := strings.Repeat("x", size)
		lines := [][]byte{titleRecord("ai-title", "t"), userRecord(content, cwd)}
		tr := scanLines(rt, lines)
		if tr.Cwd != cwd {
			rt.Fatalf("Cwd = %q, want %q (content size %d)", tr.Cwd, cwd, size)
		}
		if !tr.HasMessages {
			rt.Fatalf("HasMessages = false for a user message")
		}
	})
}

func TestEscapedCwdKeyInContentDoesNotMatch(t *testing.T) {
	content := `the text "cwd":"C:\\wrong" is in the message`
	tr := scanLines(t, [][]byte{userRecord(content, `C:\right`)})
	if tr.Cwd != `C:\right` {
		t.Fatalf("Cwd = %q, want %q", tr.Cwd, `C:\right`)
	}
}

func TestTranscriptWithoutMessages(t *testing.T) {
	tr := scanLines(t, [][]byte{titleRecord("last-prompt", "hi"), []byte(`{"type":"queue-operation","operation":"enqueue"}`)})
	if tr.HasMessages {
		t.Fatal("HasMessages = true for a transcript without messages")
	}
}

func TestReadTranscriptsSkipsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "C--work")
	id := "0123abcd-0000-4000-8000-000000000001"
	write := func(path string, lines ...[]byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(bytes.Join(lines, []byte("\n")), '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(project, id+".jsonl"), titleRecord("custom-title", "Main"), userRecord("hi", `C:\work`))
	write(filepath.Join(project, "agent-1234.jsonl"), userRecord("sub", `C:\work`))
	write(filepath.Join(project, id, "subagents", "agent-1.jsonl"), userRecord("sub", `C:\work`))

	ts, err := ReadTranscripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].ID != id || ts[0].Title() != "Main" || ts[0].Cwd != `C:\work` {
		t.Fatalf("ReadTranscripts = %+v, want only the session %s", ts, id)
	}
}

func TestReadTranscriptsMissingDir(t *testing.T) {
	ts, err := ReadTranscripts(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(ts) != 0 {
		t.Fatalf("ReadTranscripts(missing) = %v, %v; want no sessions and no error", ts, err)
	}
}
