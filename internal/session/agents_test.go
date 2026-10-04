package session

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDiscoverOthers checks that each store gives its sessions with the
// resume command of its agent, and that a Codex subagent thread is left out.
func TestDiscoverOthers(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "03", "rollout-a.jsonl"),
		`{"type":"session_meta","payload":{"id":"c1","session_id":"c1","cwd":"/src/x"}}`+"\n{}\n")
	write(t, filepath.Join(home, ".codex", "sessions", "2026", "10", "03", "rollout-b.jsonl"),
		`{"type":"session_meta","payload":{"id":"c2","session_id":"c1","cwd":"/src/x"}}`+"\n")
	write(t, filepath.Join(home, ".copilot", "session-state", "p1", "workspace.yaml"),
		"id: p1\r\ncwd: /src/y\r\nname: 'fix it'\r\n")
	write(t, filepath.Join(home, ".copilot", "session-state", "p1", "events.jsonl"), "{}\n")
	write(t, filepath.Join(home, ".copilot", "session-state", "p2", "workspace.yaml"), "id: p2\n")
	write(t, filepath.Join(home, ".gemini", "antigravity", "conversations", "g1.db"), "")

	got := map[string]Session{}
	for _, s := range DiscoverOthers(home) {
		got[s.ID] = s
	}
	if len(got) != 3 {
		t.Fatalf("DiscoverOthers = %+v, want c1, p1, g1", got)
	}
	want := map[string][]string{
		"c1": {"codex", "resume", "c1"},
		"p1": {"copilot", "--resume=p1"},
		"g1": {"agy", "--conversation", "g1"},
	}
	for id, argv := range want {
		if !slices.Equal(got[id].Argv(), argv) {
			t.Errorf("%s: Argv = %q, want %q", id, got[id].Argv(), argv)
		}
	}
	if got["c1"].Cwd != "/src/x" || got["p1"].Cwd != "/src/y" || got["p1"].Title != "fix it" {
		t.Errorf("metadata: %+v", got)
	}
}

func TestArgvClaude(t *testing.T) {
	if a := (Session{ID: "u1"}).Argv(); !slices.Equal(a, []string{"claude", "--resume", "u1"}) {
		t.Errorf("Argv = %q", a)
	}
}
