// Package herdr calls the herdr command-line interface. herdr is a terminal
// workspace manager for AI coding agents (https://herdr.dev). The CLI is the
// herdr plugin API: most commands answer with JSON on stdout, and errors
// come as JSON on stderr with exit status 1.
package herdr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Client runs herdr commands.
type Client struct {
	bin string
}

// New returns a client for the herdr binary in HERDR_BIN_PATH, which herdr
// sets for plugins, or for "herdr" on PATH.
func New() *Client {
	bin := os.Getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	return &Client{bin: bin}
}

// Snapshot is the live layout of the herdr session.
type Snapshot struct {
	Workspaces []Workspace `json:"workspaces"`
	Tabs       []Tab       `json:"tabs"`
	Panes      []Pane      `json:"panes"`
}

// Workspace is one herdr workspace.
type Workspace struct {
	ID    string `json:"workspace_id"`
	Label string `json:"label"`
}

// Tab is one herdr tab.
type Tab struct {
	ID          string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
}

// Pane is one herdr pane. AgentSession is set when herdr knows the session
// of the agent in the pane.
type Pane struct {
	ID           string        `json:"pane_id"`
	TabID        string        `json:"tab_id"`
	WorkspaceID  string        `json:"workspace_id"`
	Agent        string        `json:"agent"`
	AgentSession *AgentSession `json:"agent_session"`
}

// AgentSession identifies the native session of an agent.
type AgentSession struct {
	Value string `json:"value"`
}

// HasPane reports whether the snapshot has a pane with the ID.
func (s Snapshot) HasPane(id string) bool {
	for _, p := range s.Panes {
		if p.ID == id {
			return true
		}
	}
	return false
}

// Process is one foreground process of a pane.
type Process struct {
	PID  int      `json:"pid"`
	Argv []string `json:"argv"`
}

// ProcessInfo tells what runs in a pane.
type ProcessInfo struct {
	ShellPID   int       `json:"shell_pid"`
	Foreground []Process `json:"foreground_processes"`
}

// Idle reports whether only the shell runs in the pane.
func (p ProcessInfo) Idle() bool {
	for _, f := range p.Foreground {
		if f.PID != p.ShellPID {
			return false
		}
	}
	return true
}

// Error is an error that herdr reported.
type Error struct {
	Args    []string
	Code    string
	Message string
}

func (e *Error) Error() string {
	cmd := strings.Join(e.Args[:min(2, len(e.Args))], " ")
	if e.Code == "" {
		return fmt.Sprintf("herdr %s: %s", cmd, e.Message)
	}
	return fmt.Sprintf("herdr %s: %s: %s", cmd, e.Code, e.Message)
}

// run runs herdr and returns stdout.
func (c *Client) run(args ...string) ([]byte, error) {
	cmd := exec.Command(c.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		e := &Error{Args: args, Message: strings.TrimSpace(stderr.String())}
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(stderr.Bytes(), &body) == nil && body.Error.Message != "" {
			e.Code, e.Message = body.Error.Code, body.Error.Message
		}
		if e.Message == "" {
			e.Message = err.Error()
		}
		return nil, e
	}
	return stdout.Bytes(), nil
}

// call runs herdr and decodes the "result" member of its answer into out.
func (c *Client) call(out any, args ...string) error {
	b, err := c.run(args...)
	if err != nil || out == nil {
		return err
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("herdr %s: %w", args[0], err)
	}
	return json.Unmarshal(env.Result, out)
}

// Snapshot returns the live layout.
func (c *Client) Snapshot() (Snapshot, error) {
	var r struct {
		Snapshot Snapshot `json:"snapshot"`
	}
	err := c.call(&r, "api", "snapshot")
	return r.Snapshot, err
}

// ProcessInfo returns what runs in a pane.
func (c *Client) ProcessInfo(pane string) (ProcessInfo, error) {
	var r struct {
		ProcessInfo ProcessInfo `json:"process_info"`
	}
	err := c.call(&r, "pane", "process-info", "--pane", pane)
	return r.ProcessInfo, err
}

// FocusPane shows the pane that hosts an agent.
func (c *Client) FocusPane(pane string) error {
	return c.call(nil, "agent", "focus", pane)
}

// FocusTab shows a tab.
func (c *Client) FocusTab(tab string) error {
	return c.call(nil, "tab", "focus", tab)
}

// Created is the tab and the pane of a new workspace or tab.
type Created struct {
	Workspace string
	Tab       string
	Pane      string
}

type createResult struct {
	Workspace struct {
		ID string `json:"workspace_id"`
	} `json:"workspace"`
	Tab struct {
		ID string `json:"tab_id"`
	} `json:"tab"`
	RootPane struct {
		ID string `json:"pane_id"`
	} `json:"root_pane"`
}

func focusFlag(focus bool) string {
	if focus {
		return "--focus"
	}
	return "--no-focus"
}

// CreateWorkspace makes a workspace with one tab.
func (c *Client) CreateWorkspace(cwd, label string, focus bool) (Created, error) {
	var r createResult
	err := c.call(&r, "workspace", "create", "--cwd", cwd, "--label", label, focusFlag(focus))
	return Created{Workspace: r.Workspace.ID, Tab: r.Tab.ID, Pane: r.RootPane.ID}, err
}

// CreateTab makes a tab in a workspace.
func (c *Client) CreateTab(workspace, cwd, label string, focus bool) (Created, error) {
	var r createResult
	err := c.call(&r, "tab", "create", "--workspace", workspace, "--cwd", cwd, "--label", label, focusFlag(focus))
	return Created{Workspace: workspace, Tab: r.Tab.ID, Pane: r.RootPane.ID}, err
}

// RenameTab sets the label of a tab.
func (c *Client) RenameTab(tab, label string) error {
	return c.call(nil, "tab", "rename", tab, label)
}

// StartAgent starts Claude Code in an idle shell pane with the arguments,
// and waits until it is ready for input. herdr reports the code
// "agent_not_ready" when Claude Code shows a question at start, for example
// the question about trust in a folder.
func (c *Client) StartAgent(name, pane string, timeoutMS int, args []string) error {
	argv := append([]string{"agent", "start", name, "--kind", "claude", "--pane", pane,
		"--timeout", strconv.Itoa(timeoutMS), "--"}, args...)
	return c.call(nil, argv...)
}

// SendText types text into a pane without Enter.
func (c *Client) SendText(pane, text string) error {
	return c.call(nil, "pane", "send-text", pane, text)
}

// SendKeys sends logical keys, such as "esc", to a pane.
func (c *Client) SendKeys(pane string, keys ...string) error {
	return c.call(nil, append([]string{"pane", "send-keys", pane}, keys...)...)
}

// Run types a command into a pane and presses Enter.
func (c *Client) Run(pane, command string) error {
	return c.call(nil, "pane", "run", pane, command)
}

// WaitOutput waits until the visible text of a pane matches the regular
// expression.
func (c *Client) WaitOutput(pane, regex string, timeoutMS int) error {
	return c.call(nil, "pane", "wait-output", pane, "--regex", regex, "--timeout", strconv.Itoa(timeoutMS))
}

// Read returns the last lines of the recent text of a pane, with the soft
// line wraps joined, so that a long command reads as one line.
func (c *Client) Read(pane string, lines int) (string, error) {
	b, err := c.run("pane", "read", pane, "--source", "recent-unwrapped", "--lines", strconv.Itoa(lines))
	return string(b), err
}

// OpenPluginPane opens a pane that a plugin manifest declares.
func (c *Client) OpenPluginPane(plugin, entrypoint string) error {
	return c.call(nil, "plugin", "pane", "open", "--plugin", plugin, "--entrypoint", entrypoint)
}
