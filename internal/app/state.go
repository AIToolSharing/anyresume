package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// state records the herdr tab that "anyresume import" made for each
// session. herdr keeps tab and pane IDs when it restores a session, so the
// record stays valid after a herdr restart.
type state struct {
	Version int                    `json:"version"`
	Socket  string                 `json:"socket,omitempty"`
	Tabs    map[string]importedTab `json:"tabs"`
}

type importedTab struct {
	Tab  string `json:"tab"`
	Pane string `json:"pane"`
}

// statePath returns the record file of the herdr session with the socket
// path. Each herdr session has its own tab IDs, and the plugin startup hook
// runs in each session, so each session needs its own record.
func statePath(configDir, socket string) string {
	sum := sha256.Sum256([]byte(socket))
	return filepath.Join(configDir, "anyresume", "herdr-tabs-"+hex.EncodeToString(sum[:6])+".json")
}

// legacyStatePath is the one record file of anyresume 0.1.2 and older.
func legacyStatePath(configDir string) string {
	return filepath.Join(configDir, "anyresume", "herdr-tabs.json")
}

// migrateLegacyState moves the record of anyresume 0.1.2 and older to path.
// It does nothing when path exists or when there is no old record.
// anyresume 0.1.2 and older had only one record, for the default herdr
// session, so the caller migrates only in the default session.
func migrateLegacyState(legacy, path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := os.Stat(legacy); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	st, err := loadState(legacy)
	if err != nil {
		return err
	}
	if err := st.save(path); err != nil {
		return err
	}
	return os.Rename(legacy, legacy+".migrated")
}

func loadState(path string) (state, error) {
	st := state{Version: 1, Tabs: map[string]importedTab{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, err
	}
	if st.Tabs == nil {
		st.Tabs = map[string]importedTab{}
	}
	return st, nil
}

// save writes the state to a temporary file and renames it, so that a crash
// never leaves a half-written file.
func (st state) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
