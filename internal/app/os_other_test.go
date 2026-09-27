//go:build !windows

package app

import (
	"os/exec"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// TestQuoteExeRoundTripsThroughSh checks that sh reads the quoted path back
// as the same string, also with spaces, quotes, and other shell characters.
func TestQuoteExeRoundTripsThroughSh(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		p := rapid.StringN(1, 40, -1).Filter(func(s string) bool { return !strings.ContainsRune(s, 0) }).Draw(rt, "path")
		out, err := exec.Command("sh", "-c", "printf %s "+quoteExe(p)).Output()
		if err != nil {
			rt.Fatalf("sh: %v", err)
		}
		if string(out) != p {
			rt.Fatalf("sh read %q back as %q", p, out)
		}
	})
}
