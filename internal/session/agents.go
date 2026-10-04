package session

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverOthers returns the Codex, Copilot CLI, and Antigravity sessions in
// the home folder. A store that is missing or unreadable gives no sessions.
func DiscoverOthers(home string) []Session {
	var out []Session
	out = append(out, codexSessions(filepath.Join(home, ".codex", "sessions"))...)
	out = append(out, copilotSessions(filepath.Join(home, ".copilot", "session-state"))...)
	out = append(out, antigravitySessions(filepath.Join(home, ".gemini", "antigravity", "conversations"))...)
	return out
}

// codexSessions reads the first record of each rollout file. A rollout of
// a subagent thread has a session_id other than its own id; codex resume
// continues the parent, so it is left out.
func codexSessions(dir string) []Session {
	var out []Session
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		line, _ := bufio.NewReader(f).ReadBytes('\n')
		var rec struct {
			Type    string `json:"type"`
			Payload struct {
				ID        string `json:"id"`
				SessionID string `json:"session_id"`
				Cwd       string `json:"cwd"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &rec) != nil || rec.Type != "session_meta" || rec.Payload.ID == "" {
			return nil
		}
		if p := rec.Payload; p.SessionID != "" && p.SessionID != p.ID {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, Session{
			ID: rec.Payload.ID, Title: "codex " + filepath.Base(rec.Payload.Cwd),
			Cwd: rec.Payload.Cwd, LastActive: info.ModTime(), Agent: Codex,
		})
		return nil
	})
	return out
}

// copilotSessions reads workspace.yaml in each session folder. A session
// without events.jsonl has no messages.
func copilotSessions(dir string) []Session {
	entries, _ := os.ReadDir(dir)
	var out []Session
	for _, e := range entries {
		events, err := os.Stat(filepath.Join(dir, e.Name(), "events.jsonl"))
		if !e.IsDir() || err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name(), "workspace.yaml"))
		if err != nil {
			continue
		}
		s := Session{LastActive: events.ModTime(), Agent: Copilot}
		// ponytail: reads only one-line "key: value" fields, a YAML parser
		// if Copilot starts to write block scalars for them.
		for _, l := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(strings.TrimRight(l, "\r"), ": ")
			if !ok {
				continue
			}
			v = strings.Trim(v, `'"`)
			switch k {
			case "id":
				s.ID = v
			case "cwd":
				s.Cwd = v
			case "name":
				s.Title = v
			}
		}
		if s.ID == "" {
			continue
		}
		if s.Title == "" {
			s.Title = "copilot " + filepath.Base(s.Cwd)
		}
		out = append(out, s)
	}
	return out
}

// antigravitySessions lists the conversation databases. The ID is the file
// name. The folder is not read from the database, so Antigravity resumes in
// the home folder.
func antigravitySessions(dir string) []Session {
	entries, _ := os.ReadDir(dir)
	var out []Session
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".db")
		info, err := e.Info()
		if !ok || e.IsDir() || err != nil {
			continue
		}
		out = append(out, Session{ID: id, Title: "antigravity " + id[:min(8, len(id))], LastActive: info.ModTime(), Agent: Antigravity})
	}
	return out
}
