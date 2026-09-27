package app

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AIToolSharing/anyresume/internal/herdr"
	"github.com/AIToolSharing/anyresume/internal/session"
)

// openedWorkspace is the herdr workspace for the sessions that "pick" and
// "open" start. All folders share it.
const openedWorkspace = "claude chats"

// tabLabelMax is the longest tab label in runes.
const tabLabelMax = 40

// minFreeCommit is the least free memory commit that "import" keeps. Each
// idle tab costs one shell process.
const minFreeCommit = 1 << 30

// promptRE matches the end of the visible text of a pane whose shell waits
// at an empty prompt: cmd.exe ends with ">", PowerShell with "> ", sh and
// bash with "$ ", zsh with "% ", and a root shell with "# ".
var promptRE = regexp.MustCompile(`[>$#%]\s*$`)

// promptPattern is promptRE for herdr, which uses Rust regular expressions.
const promptPattern = `[>$#%]\s*$`

// importWorkspace returns the herdr workspace for an imported session: one
// workspace for each day of last activity.
func importWorkspace(s session.Session) string {
	return "chats " + s.LastActive.Local().Format("01-02")
}

// tabLabel returns the tab label for a title: one line, at most tabLabelMax
// runes.
func tabLabel(title string) string {
	r := []rune(cleanTitle(title))
	if len(r) > tabLabelMax {
		r = r[:tabLabelMax]
	}
	return strings.TrimSpace(string(r))
}

// agentName returns a herdr agent name for s. herdr needs names that match
// [a-z][a-z0-9_-]{0,31}. The name is a slug of the title and the first four
// characters of the session ID, so two sessions with one title get two
// names.
func agentName(s session.Session) string {
	slug := slugify(s.Title)
	if slug == "" || slug[0] < 'a' || slug[0] > 'z' {
		slug = strings.TrimRight("chat-"+slug, "-")
	}
	if len(slug) > 26 {
		slug = strings.TrimRight(slug[:26], "-")
	}
	id := slugify(s.ID)
	if len(id) > 4 {
		id = id[:4]
	}
	if id == "" {
		id = "x"
	}
	return slug + "-" + id
}

// slugify keeps the ASCII letters and digits of s in lower case and puts
// one "-" for each run of other characters between them.
func slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// findRunning returns the pane that runs the session. herdr knows the
// session of a pane from its Claude Code integration. A pane that resumed
// a copy of the session with --fork-session holds the ID in its arguments.
func (e *env) findRunning(snap herdr.Snapshot, id string) (string, bool) {
	for _, p := range snap.Panes {
		if p.AgentSession != nil && p.AgentSession.Value == id {
			return p.ID, true
		}
	}
	for _, p := range snap.Panes {
		if p.Agent != "claude" {
			continue
		}
		info, err := e.herdr.ProcessInfo(p.ID)
		if err != nil {
			continue
		}
		for _, f := range info.Foreground {
			if slices.Contains(f.Argv, id) {
				return p.ID, true
			}
		}
	}
	return "", false
}

// openInHerdr focuses the pane that runs s, or resumes s in its imported
// tab, or opens s in a new tab of the workspace "claude chats".
func (e *env) openInHerdr(s session.Session) error {
	snap, err := e.herdr.Snapshot()
	if err != nil {
		return err
	}
	if pane, ok := e.findRunning(snap, s.ID); ok {
		return e.herdr.FocusPane(pane)
	}
	if h, ok := e.holders()[s.ID]; ok {
		return heldError(s, h)
	}
	st, err := loadState(e.statePath)
	if err != nil {
		return err
	}
	if t, ok := st.Tabs[s.ID]; ok && snap.HasPane(t.Pane) {
		if info, err := e.herdr.ProcessInfo(t.Pane); err == nil && info.Idle() {
			cmd, err := resumeCommand(s.ID)
			if err != nil {
				return err
			}
			if err := e.herdr.FocusTab(t.Tab); err != nil {
				return err
			}
			// Remove a command that import typed before, then run it again.
			if err := e.herdr.SendKeys(t.Pane, clearLineKey); err != nil {
				return err
			}
			return e.herdr.Run(t.Pane, cmd)
		}
	}
	c, err := e.placeTab(&snap, openedWorkspace, s, true)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stderr, "Opening %q. A long session can take a minute.\n", cleanTitle(s.Title))
	err = e.herdr.StartAgent(agentName(s), c.Pane, 180000, s.ResumeArgs())
	var he *herdr.Error
	if errors.As(err, &he) && he.Code == "agent_not_ready" {
		fmt.Fprintln(e.stderr, "Claude Code asks a question in the new tab. Answer it there.")
		return nil
	}
	return err
}

// placeTab makes a tab for s in the workspace with the label. When the
// workspace does not exist, it makes the workspace and uses its first tab.
func (e *env) placeTab(snap *herdr.Snapshot, label string, s session.Session, focus bool) (herdr.Created, error) {
	dir := sessionDir(s)
	title := tabLabel(s.Title)
	for _, w := range snap.Workspaces {
		if w.Label == label {
			return e.herdr.CreateTab(w.ID, dir, title, focus)
		}
	}
	c, err := e.herdr.CreateWorkspace(dir, label, focus)
	if err != nil {
		return c, err
	}
	snap.Workspaces = append(snap.Workspaces, herdr.Workspace{ID: c.Workspace, Label: label})
	return c, e.herdr.RenameTab(c.Tab, title)
}

type stepKind int

const (
	stepRetype stepKind = iota
	stepCreate
)

type importStep struct {
	kind      stepKind
	session   session.Session
	tab       importedTab
	workspace string
}

// planImport decides what "import" does for each session. A session that
// runs in a herdr pane needs nothing. A session with an imported tab gets
// its resume command typed again. Every other session gets a new tab,
// unless refresh is set.
func planImport(sessions []session.Session, st state, snap herdr.Snapshot, refresh bool) []importStep {
	running := map[string]bool{}
	for _, p := range snap.Panes {
		if p.AgentSession != nil {
			running[p.AgentSession.Value] = true
		}
	}
	var steps []importStep
	for _, s := range sessions {
		if running[s.ID] {
			continue
		}
		if t, ok := st.Tabs[s.ID]; ok && snap.HasPane(t.Pane) {
			steps = append(steps, importStep{kind: stepRetype, session: s, tab: t})
			continue
		}
		if !refresh {
			steps = append(steps, importStep{kind: stepCreate, session: s, workspace: importWorkspace(s)})
		}
	}
	return steps
}

// importTabs gives each session a herdr tab in its own folder. The shell of
// the tab has the resume command on its prompt: press Enter to continue the
// session. With --refresh, it only types the command again, for example
// after a herdr restart, which restores the tabs with empty prompts.
func (e *env) importTabs(args []string) error {
	fs := newFlags("import")
	refresh := fs.Bool("refresh", false, "type the resume command again in imported tabs; make no new tabs")
	limit := fs.Int("limit", 0, "make at most this many new tabs (0: no limit)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if e.herdr == nil {
		return errors.New("import needs herdr: run it in a herdr pane or as the plugin action")
	}
	sessions, err := e.sessions()
	if err != nil {
		return err
	}
	st, err := loadState(e.statePath)
	if err != nil {
		return err
	}
	snap, err := e.herdr.Snapshot()
	if err != nil {
		return err
	}

	var made, pending []importStep
	stop := ""
	for _, step := range planImport(sessions, st, snap, *refresh) {
		if step.kind == stepRetype {
			pending = append(pending, step)
			continue
		}
		if *limit > 0 && len(made) >= *limit {
			stop = fmt.Sprintf("stopped at the limit of %d new tabs", *limit)
			break
		}
		if free, known := availableCommit(); known && free < minFreeCommit {
			stop = fmt.Sprintf("stopped because only %d MB of memory commit is free", free>>20)
			break
		}
		c, err := e.placeTab(&snap, step.workspace, step.session, false)
		if err != nil {
			return err
		}
		step.tab = importedTab{Tab: c.Tab, Pane: c.Pane}
		st.Tabs[step.session.ID] = step.tab
		if err := st.save(e.statePath); err != nil {
			return err
		}
		made = append(made, step)
	}

	typed := 0
	for _, step := range made {
		// A new shell needs a moment before it shows its prompt.
		if e.herdr.WaitOutput(step.tab.Pane, promptPattern, 20000) != nil {
			continue
		}
		r, err := e.retype(step)
		if err != nil {
			return err
		}
		if r == typedCommand {
			typed++
		}
	}
	// After a herdr restart, the restored shells start at the same time.
	// Try a pane without a prompt up to three times.
	for round := 0; round < 3 && len(pending) > 0; round++ {
		if round > 0 {
			time.Sleep(2 * time.Second)
		}
		var later []importStep
		for _, step := range pending {
			r, err := e.retype(step)
			if err != nil {
				return err
			}
			switch r {
			case typedCommand:
				typed++
			case notReady:
				later = append(later, step)
			}
		}
		pending = later
	}

	fmt.Fprintf(e.stdout, "Made %d new tabs. Typed the resume command in %d tabs.\n", len(made), typed)
	if stop != "" {
		fmt.Fprintf(e.stdout, "Import %s. Run it again to continue.\n", stop)
	}
	return nil
}

type retypeResult int

const (
	// typedCommand: the shell waited at an empty prompt, and retype typed
	// the resume command.
	typedCommand retypeResult = iota
	// notReady: the pane shows no text yet. Its shell is still starting.
	notReady
	// skipped: a program runs in the pane, or the prompt has text on it.
	skipped
)

// retype types the resume command into an imported tab when its shell waits
// at an empty prompt.
func (e *env) retype(step importStep) (retypeResult, error) {
	info, err := e.herdr.ProcessInfo(step.tab.Pane)
	if err != nil || !info.Idle() {
		return skipped, nil
	}
	text, err := e.herdr.Read(step.tab.Pane, 3)
	if err != nil {
		return skipped, nil
	}
	text = strings.TrimRight(text, " \t\r\n")
	if text == "" {
		return notReady, nil
	}
	if !promptRE.MatchString(text) {
		return skipped, nil
	}
	cmd, err := resumeCommand(step.session.ID)
	if err != nil {
		return skipped, err
	}
	return typedCommand, e.herdr.SendText(step.tab.Pane, cmd)
}
