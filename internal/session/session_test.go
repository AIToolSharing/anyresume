package session

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func TestMergeUsesDesktopMetadata(t *testing.T) {
	old := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ts := []Transcript{
		{ID: "a", Cwd: `C:\cli`, Modified: old, HasMessages: true, custom: "CLI title"},
		{ID: "b", Cwd: `C:\b`, Modified: old.Add(time.Hour), HasMessages: true, lastPrompt: "fix it"},
		{ID: "c", Modified: old.Add(2 * time.Hour)},
	}
	chats := map[string]DesktopChat{
		"a": {CliSessionID: "a", Title: "Desktop title", Cwd: `C:\scratch`, IsArchived: true,
			Model: "claude-opus-5-5", Effort: "max", PermissionMode: "auto",
			LastActivityAt: old.Add(3 * time.Hour).UnixMilli()},
	}
	got := Merge(ts, chats)
	if len(got) != 2 {
		t.Fatalf("Merge kept %d sessions, want 2 (a session without messages is dropped)", len(got))
	}
	a, b := got[0], got[1]
	if a.ID != "a" || b.ID != "b" {
		t.Fatalf("order = %s, %s; want a (newer desktop activity), then b", a.ID, b.ID)
	}
	if a.Title != "Desktop title" || a.Cwd != `C:\scratch` || !a.Archived || !a.Desktop || a.Model != "claude-opus-5-5" {
		t.Fatalf("desktop metadata not applied: %+v", a)
	}
	if b.Title != "fix it" || b.Desktop {
		t.Fatalf("CLI session wrong: %+v", b)
	}
}

func TestMergeSortsNewestFirst(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		n := rapid.IntRange(0, 30).Draw(rt, "n")
		ts := make([]Transcript, n)
		for i := range ts {
			ts[i] = Transcript{
				ID:          string(rune('a' + i)),
				Modified:    time.Unix(rapid.Int64Range(0, 1e9).Draw(rt, "t"), 0),
				HasMessages: true,
			}
		}
		got := Merge(ts, nil)
		if !slices.IsSortedFunc(got, func(x, y Session) int { return y.LastActive.Compare(x.LastActive) }) {
			rt.Fatalf("Merge result is not sorted newest first")
		}
	})
}

// TestResumeArgsNeverRestoreBypass checks every permission mode string:
// the arguments never ask for bypassPermissions or plan mode.
func TestResumeArgsNeverRestoreBypass(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		mode := rapid.OneOf(rapid.SampledFrom([]string{"default", "auto", "acceptEdits", "bypassPermissions", "plan", "manual", "dontAsk", ""}), rapid.String()).Draw(rt, "mode")
		args := Session{ID: "x", PermissionMode: mode}.ResumeArgs()
		for _, a := range args {
			if a == "bypassPermissions" || a == "plan" || a == "--dangerously-skip-permissions" {
				rt.Fatalf("ResumeArgs for mode %q = %v", mode, args)
			}
		}
	})
}

func TestResumeArgs(t *testing.T) {
	s := Session{ID: "id", Model: "claude-opus-5-5[1m]", Effort: "max", PermissionMode: "default"}
	want := []string{"--resume", "id", "--model", "claude-opus-5-5[1m]", "--effort", "max", "--permission-mode", "manual"}
	if got := s.ResumeArgs(); !slices.Equal(got, want) {
		t.Fatalf("ResumeArgs = %v, want %v", got, want)
	}
	bad := Session{ID: "id", Model: "opus --dangerously-skip-permissions", Effort: "huge"}
	if got := bad.ResumeArgs(); !slices.Equal(got, []string{"--resume", "id"}) {
		t.Fatalf("ResumeArgs with a bad model and effort = %v, want only --resume", got)
	}
}

func TestReadDesktopChats(t *testing.T) {
	dir := t.TempDir()
	org := filepath.Join(dir, "account", "org")
	if err := os.MkdirAll(org, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"local_1.json": `{"cliSessionId":"s1","title":"Old","lastActivityAt":1}`,
		"local_2.json": `{"cliSessionId":"s1","title":"New","lastActivityAt":2,"isArchived":true}`,
		"local_3.json": `{"title":"no session id"}`,
		"other.json":   `{"cliSessionId":"s9"}`,
		"local_4.json": `not json`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(org, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	chats, err := ReadDesktopChats(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 1 || chats["s1"].Title != "New" || !chats["s1"].IsArchived {
		t.Fatalf("ReadDesktopChats = %+v, want only s1 with the newer title", chats)
	}
	if _, err := ReadDesktopChats(filepath.Join(dir, "missing")); err != nil {
		t.Fatalf("ReadDesktopChats(missing) error = %v", err)
	}
}

func TestReadHolders(t *testing.T) {
	dir := t.TempDir()
	entries := map[string]string{
		"100.json": `{"pid":100,"sessionId":"live","entrypoint":"claude-desktop"}`,
		"200.json": `{"pid":200,"sessionId":"dead","entrypoint":"cli"}`,
		"300.json": `{"pid":0,"sessionId":"zero"}`,
		"bad.json": `{`,
	}
	for name, body := range entries {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	alive := func(pid int) bool { return pid == 100 }
	h, err := readHolders(dir, alive)
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 1 || h["live"] != (Holder{PID: 100, Entrypoint: "claude-desktop"}) {
		t.Fatalf("readHolders = %+v, want only the live desktop holder", h)
	}
}

func TestProcessAliveForThisProcess(t *testing.T) {
	if !processAlive(os.Getpid()) {
		t.Fatal("processAlive(own pid) = false")
	}
}
