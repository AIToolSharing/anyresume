package herdr

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tests run this test binary again as a fake herdr. The fake writes its
// arguments to the log file and answers like herdr 0.9.1.
const fakeEnv = "ANYRESUME_FAKE_HERDR_LOG"

func TestMain(m *testing.M) {
	if log := os.Getenv(fakeEnv); log != "" {
		os.Exit(fakeHerdr(log, os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeHerdr(log string, args []string) int {
	f, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintln(f, strings.Join(args, "\x1f"))
		f.Close()
	}
	switch strings.Join(args[:min(2, len(args))], " ") {
	case "api snapshot":
		fmt.Print(`{"id":"cli:api:snapshot","result":{"snapshot":{"workspaces":[{"workspace_id":"w1","label":"chats 09-26","tab_count":1}],` +
			`"tabs":[{"tab_id":"w1:t1","workspace_id":"w1","label":"Fix the login"}],` +
			`"panes":[{"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1","agent":"claude","agent_session":{"agent":"claude","kind":"id","value":"s-1"}},` +
			`{"pane_id":"w1:p2","tab_id":"w1:t1","workspace_id":"w1"}]},"type":"api_snapshot"}}`)
	case "pane process-info":
		fmt.Print(`{"result":{"process_info":{"shell_pid":10,"foreground_processes":[{"pid":10,"argv":["cmd.exe"]}]},"type":"pane_process_info"}}`)
	case "tab create":
		fmt.Print(`{"result":{"tab":{"tab_id":"w1:t2"},"root_pane":{"pane_id":"w1:p3"},"type":"tab_created"}}`)
	case "workspace create":
		fmt.Print(`{"result":{"workspace":{"workspace_id":"w2"},"tab":{"tab_id":"w2:t1"},"root_pane":{"pane_id":"w2:p1"}}}`)
	case "agent start":
		fmt.Fprint(os.Stderr, `{"error":{"code":"agent_not_ready","message":"agent is blocked"},"id":"cli:agent:start"}`)
		return 1
	case "pane read":
		fmt.Print("C:\\work>\r\n")
	case "tab focus", "pane send-text", "pane send-keys", "pane run":
		fmt.Print(`{"result":{"type":"ok"}}`)
	default:
		fmt.Fprint(os.Stderr, "unknown command")
		return 2
	}
	return 0
}

// newFake returns a client for the fake herdr and the file with its calls.
func newFake(t *testing.T) (*Client, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "calls.log")
	t.Setenv(fakeEnv, log)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &Client{bin: exe}, log
}

func calls(t *testing.T, log string) [][]string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		out = append(out, strings.Split(line, "\x1f"))
	}
	return out
}

func TestSnapshot(t *testing.T) {
	c, _ := newFake(t)
	snap, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Workspaces) != 1 || snap.Workspaces[0].Label != "chats 09-26" {
		t.Fatalf("workspaces = %+v", snap.Workspaces)
	}
	if len(snap.Tabs) != 1 || snap.Tabs[0].Label != "Fix the login" {
		t.Fatalf("tabs = %+v", snap.Tabs)
	}
	if len(snap.Panes) != 2 || snap.Panes[0].AgentSession == nil || snap.Panes[0].AgentSession.Value != "s-1" || snap.Panes[1].AgentSession != nil {
		t.Fatalf("panes = %+v", snap.Panes)
	}
	if !snap.HasPane("w1:p2") || snap.HasPane("w9:p9") {
		t.Fatal("HasPane is wrong")
	}
}

func TestProcessInfoIdle(t *testing.T) {
	c, _ := newFake(t)
	info, err := c.ProcessInfo("w1:p2")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Idle() {
		t.Fatalf("ProcessInfo = %+v, want an idle shell", info)
	}
	busy := ProcessInfo{ShellPID: 10, Foreground: []Process{{PID: 10}, {PID: 11, Argv: []string{"claude"}}}}
	if busy.Idle() {
		t.Fatal("Idle() = true with claude in the foreground")
	}
}

func TestCreate(t *testing.T) {
	c, log := newFake(t)
	tab, err := c.CreateTab("w1", `C:\work dir`, "Fix the login", false)
	if err != nil || tab != (Created{Workspace: "w1", Tab: "w1:t2", Pane: "w1:p3"}) {
		t.Fatalf("CreateTab = %+v, %v", tab, err)
	}
	ws, err := c.CreateWorkspace(`C:\work dir`, "claude chats", true)
	if err != nil || ws != (Created{Workspace: "w2", Tab: "w2:t1", Pane: "w2:p1"}) {
		t.Fatalf("CreateWorkspace = %+v, %v", ws, err)
	}
	got := calls(t, log)
	want := []string{"tab", "create", "--workspace", "w1", "--cwd", `C:\work dir`, "--label", "Fix the login", "--no-focus"}
	if !slices.Equal(got[0], want) {
		t.Fatalf("tab create argv = %q, want %q", got[0], want)
	}
	if got[1][len(got[1])-1] != "--focus" {
		t.Fatalf("workspace create argv = %q, want --focus at the end", got[1])
	}
}

// TestStartAgentError checks the argv that starts Claude Code and the
// decoding of a herdr error.
func TestStartAgentError(t *testing.T) {
	c, log := newFake(t)
	err := c.StartAgent("fix-the-login-2d5b", "w1:p3", 180000, []string{"--resume", "2d5b", "--model", "opus"})
	var he *Error
	if !errors.As(err, &he) || he.Code != "agent_not_ready" || he.Message != "agent is blocked" {
		t.Fatalf("StartAgent error = %#v, want the herdr error agent_not_ready", err)
	}
	want := []string{"agent", "start", "fix-the-login-2d5b", "--kind", "claude", "--pane", "w1:p3", "--timeout", "180000", "--", "--resume", "2d5b", "--model", "opus"}
	if got := calls(t, log)[0]; !slices.Equal(got, want) {
		t.Fatalf("agent start argv = %q, want %q", got, want)
	}
}

func TestReadAndTyping(t *testing.T) {
	c, log := newFake(t)
	text, err := c.Read("w1:p2", 3)
	if err != nil || text != "C:\\work>\r\n" {
		t.Fatalf("Read = %q, %v", text, err)
	}
	if err := c.SendText("w1:p2", `C:\A~1\anyresume.exe resume s-1`); err != nil {
		t.Fatal(err)
	}
	if err := c.SendKeys("w1:p2", "esc"); err != nil {
		t.Fatal(err)
	}
	got := calls(t, log)
	if !slices.Equal(got[0], []string{"pane", "read", "w1:p2", "--source", "recent-unwrapped", "--lines", "3"}) {
		t.Fatalf("pane read argv = %q", got[0])
	}
	if !slices.Equal(got[1], []string{"pane", "send-text", "w1:p2", `C:\A~1\anyresume.exe resume s-1`}) {
		t.Fatalf("send-text argv = %q", got[1])
	}
}

func TestUnknownCommandError(t *testing.T) {
	c, _ := newFake(t)
	err := c.call(nil, "no", "such")
	var he *Error
	if !errors.As(err, &he) || he.Code != "" || he.Message != "unknown command" {
		t.Fatalf("error = %#v, want a plain herdr error", err)
	}
}
