//go:build !windows

package app

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// clearLineKey is the key that clears the input line of bash, zsh, and
// fish.
const clearLineKey = "ctrl+u"

// runClaude replaces this process with claude, started in dir.
func runClaude(dir string, args []string) error {
	path, err := exec.LookPath("claude")
	if err != nil {
		return errors.New("claude is not on PATH")
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(path, append([]string{"claude"}, args...), os.Environ())
}

// quoteExe returns path in single quotes for a POSIX shell.
func quoteExe(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// availableCommit is not known on this system. Import does not stop for
// memory there.
func availableCommit() (uint64, bool) {
	return 0, false
}
