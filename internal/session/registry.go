package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Holder is a live process that has a session open.
type Holder struct {
	PID int
	// Entrypoint is "cli" for a terminal and "claude-desktop" for the
	// desktop app.
	Entrypoint string
}

type registryEntry struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	Entrypoint string `json:"entrypoint"`
}

// Holders reads the live-session registry of Claude Code,
// <config dir>/sessions/<pid>.json, and returns the live holders keyed by
// session ID. Claude Code does not stop a resume when the desktop app holds
// the session, so anyresume checks this registry itself.
func Holders(configDir string) (map[string]Holder, error) {
	return readHolders(filepath.Join(configDir, "sessions"), processAlive)
}

func readHolders(dir string, alive func(pid int) bool) (map[string]Holder, error) {
	holders := map[string]Holder{}
	files, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return holders, nil
	}
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var e registryEntry
		if json.Unmarshal(b, &e) != nil || e.SessionID == "" || e.PID <= 0 {
			continue
		}
		if alive(e.PID) {
			holders[e.SessionID] = Holder{PID: e.PID, Entrypoint: e.Entrypoint}
		}
	}
	return holders, nil
}
