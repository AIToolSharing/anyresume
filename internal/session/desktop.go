package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DesktopChat is the metadata that the Claude desktop app keeps for one of
// its Code chats. CliSessionID is the ID of the Claude Code transcript.
type DesktopChat struct {
	CliSessionID   string `json:"cliSessionId"`
	Title          string `json:"title"`
	Cwd            string `json:"cwd"`
	IsArchived     bool   `json:"isArchived"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permissionMode"`
	LastActivityAt int64  `json:"lastActivityAt"`
}

// ReadDesktopChats reads the local_*.json files under dir, keyed by
// CliSessionID. When two files name the same session, the newer one wins.
// A missing dir gives an empty map.
func ReadDesktopChats(dir string) (map[string]DesktopChat, error) {
	chats := map[string]DesktopChat{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() || !strings.HasPrefix(name, "local_") || !strings.HasSuffix(name, ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var c DesktopChat
		if json.Unmarshal(b, &c) != nil || c.CliSessionID == "" {
			return nil
		}
		if old, ok := chats[c.CliSessionID]; !ok || c.LastActivityAt > old.LastActivityAt {
			chats[c.CliSessionID] = c
		}
		return nil
	})
	return chats, err
}
