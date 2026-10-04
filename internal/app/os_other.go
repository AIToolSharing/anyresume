//go:build !windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// clearLineKey is the key that clears the input line of bash, zsh, and
// fish.
const clearLineKey = "ctrl+u"

// runAgent replaces this process with the program argv[0], started in dir.
func runAgent(dir string, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("%s is not on PATH", argv[0])
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
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
