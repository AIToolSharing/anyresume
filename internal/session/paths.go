package session

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the Claude Code config folder. CLAUDE_CONFIG_DIR
// overrides the default ~/.claude.
func ConfigDir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// DesktopDir returns the folder where the Claude desktop app keeps the
// metadata of its Code chats: %APPDATA%\Claude\claude-code-sessions on
// Windows and ~/Library/Application Support/Claude/claude-code-sessions on
// macOS. The folder does not exist when the desktop app is not installed.
func DesktopDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "Claude", "claude-code-sessions"), nil
}
