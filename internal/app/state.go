package app

import (
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
	Tabs    map[string]importedTab `json:"tabs"`
}

type importedTab struct {
	Tab  string `json:"tab"`
	Pane string `json:"pane"`
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
