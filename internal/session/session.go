// Package session finds the Claude Code sessions on this computer: the
// transcripts in ~/.claude/projects and the metadata of the Claude desktop
// app. It does not depend on the folder where a session started.
package session

import (
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Session is one Claude Code conversation that anyresume can resume.
type Session struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Cwd        string    `json:"cwd"`
	LastActive time.Time `json:"lastActive"`
	Archived   bool      `json:"archived"`
	// Desktop is true when the Claude desktop app knows the session.
	Desktop bool `json:"desktop"`
	// Model, Effort, and PermissionMode come from the desktop app. They are
	// empty for a session that started in a terminal.
	Model          string `json:"model,omitempty"`
	Effort         string `json:"effort,omitempty"`
	PermissionMode string `json:"permissionMode,omitempty"`
}

// Discover returns every session that has messages, newest first.
func Discover(configDir, desktopDir string) ([]Session, error) {
	ts, err := ReadTranscripts(filepath.Join(configDir, "projects"))
	if err != nil {
		return nil, err
	}
	chats, err := ReadDesktopChats(desktopDir)
	if err != nil {
		return nil, err
	}
	return Merge(ts, chats), nil
}

// Merge joins the transcripts with the desktop metadata. The desktop app
// gives the title, the archived flag, the folder, and the settings of its
// chats. The result has only sessions with messages, newest first.
func Merge(ts []Transcript, chats map[string]DesktopChat) []Session {
	out := make([]Session, 0, len(ts))
	for _, t := range ts {
		if !t.HasMessages {
			continue
		}
		s := Session{ID: t.ID, Title: t.Title(), Cwd: t.Cwd, LastActive: t.Modified}
		if c, ok := chats[t.ID]; ok {
			s.Desktop = true
			if c.Title != "" {
				s.Title = c.Title
			}
			if c.Cwd != "" {
				s.Cwd = c.Cwd
			}
			s.Archived = c.IsArchived
			s.Model, s.Effort, s.PermissionMode = c.Model, c.Effort, c.PermissionMode
			if la := time.UnixMilli(c.LastActivityAt); c.LastActivityAt > 0 && la.After(s.LastActive) {
				s.LastActive = la
			}
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastActive.After(out[j].LastActive) })
	return out
}

var (
	modelRE = regexp.MustCompile(`^[A-Za-z0-9._:\[\]-]{1,100}$`)
	efforts = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true, "max": true}
)

// ResumeArgs returns the claude arguments that resume s with its own model,
// effort, and permission mode.
func (s Session) ResumeArgs() []string {
	args := []string{"--resume", s.ID}
	if modelRE.MatchString(s.Model) {
		args = append(args, "--model", s.Model)
	}
	if efforts[s.Effort] {
		args = append(args, "--effort", s.Effort)
	}
	if m := CLIPermissionMode(s.PermissionMode); m != "" {
		args = append(args, "--permission-mode", m)
	}
	return args
}

// CLIPermissionMode maps a desktop permission mode to a claude
// --permission-mode value. The desktop mode "default" is "manual" in the
// CLI. The function never returns bypassPermissions or plan, because Claude
// Code itself does not restore these modes on resume.
func CLIPermissionMode(m string) string {
	switch m {
	case "default":
		return "manual"
	case "acceptEdits", "auto", "manual", "dontAsk":
		return m
	}
	return ""
}
