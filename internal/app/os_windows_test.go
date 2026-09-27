//go:build windows

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestQuoteExeHasNoSpaces checks that the command for a path with spaces
// names the same file and works in cmd.exe and PowerShell: a short path
// without spaces, or the path in double quotes when the volume has no short
// names.
func TestQuoteExeHasNoSpaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a b")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "any resume.exe")
	if err := os.WriteFile(exe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	q := quoteExe(exe)
	if strings.ContainsRune(q, ' ') {
		if q != `"`+exe+`"` {
			t.Fatalf("quoteExe(%q) = %q, want the short path or the quoted path", exe, q)
		}
		return
	}
	a, errA := os.Stat(q)
	b, errB := os.Stat(exe)
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Fatalf("quoteExe(%q) = %q, which is not the same file (%v, %v)", exe, q, errA, errB)
	}
}

func TestQuoteExeKeepsAPathWithoutSpaces(t *testing.T) {
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	q := quoteExe(exe)
	if strings.ContainsAny(q, ` "`) {
		t.Fatalf("quoteExe(%q) = %q", exe, q)
	}
	a, errA := os.Stat(q)
	b, errB := os.Stat(exe)
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Fatalf("quoteExe(%q) = %q, which is not the same file", exe, q)
	}
}
