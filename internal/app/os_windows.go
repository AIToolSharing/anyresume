//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// clearLineKey is the key that clears the input line of cmd.exe and of
// PowerShell.
const clearLineKey = "esc"

// runAgent runs the program argv[0] with its arguments in dir and waits for
// it.
func runAgent(dir string, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return fmt.Errorf("%s is not on PATH", argv[0])
	}
	cmd := exec.Command(path, argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The agent handles Ctrl+C itself. anyresume ignores it and waits until
	// the agent exits.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return fmt.Errorf("%s exited with code %d", argv[0], exit.ExitCode())
	}
	return err
}

// quoteExe returns a form of path that cmd.exe and PowerShell both run as a
// command: the 8.3 short path, which has no spaces. When the volume has no
// short names, it returns the path in double quotes for cmd.exe.
func quoteExe(path string) string {
	if p, err := shortPath(path); err == nil && !strings.ContainsRune(p, ' ') {
		return p
	}
	if strings.ContainsRune(path, ' ') {
		return `"` + path + `"`
	}
	return path
}

func shortPath(path string) (string, error) {
	long, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.GetShortPathName(long, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", err
	}
	if n == 0 || int(n) > len(buf) {
		return "", errors.New("no short path")
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// memoryStatusEx is the MEMORYSTATUSEX structure of the Windows API.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// availableCommit returns the free memory commit in bytes: the memory that
// Windows can still give to processes without a larger page file.
func availableCommit() (uint64, bool) {
	m := memoryStatusEx{}
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return 0, false
	}
	return m.AvailPageFile, true
}
