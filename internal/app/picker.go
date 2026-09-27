package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unicode"

	"github.com/AIToolSharing/anyresume/internal/session"
)

// cleanTitle turns a title into one line: it replaces control characters,
// such as tabs and line breaks, and joins the words with single spaces.
func cleanTitle(t string) string {
	t = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, t)
	t = strings.Join(strings.Fields(t), " ")
	if t == "" {
		return "(untitled)"
	}
	return t
}

// displayRow is the visible part of a list row: the date of the last
// activity, a mark for a session that a live process holds, the title, and
// the archived flag.
func displayRow(s session.Session, live bool) string {
	mark := "  "
	if live {
		mark = "● "
	}
	title := cleanTitle(s.Title)
	if s.Archived {
		title += " (archived)"
	}
	return s.LastActive.Local().Format("01-02 15:04") + "  " + mark + title
}

// fzfRow is one row of fzf input: the visible part, a tab, and the index of
// the session. fzf shows only the first field.
func fzfRow(i int, s session.Session, live bool) string {
	return displayRow(s, live) + "\t" + strconv.Itoa(i)
}

// rowIndex returns the index that fzfRow put at the end of a row.
func rowIndex(row string) (int, error) {
	row = strings.TrimRight(row, "\r\n")
	i := strings.LastIndexByte(row, '\t')
	if i < 0 {
		return 0, fmt.Errorf("fzf returned a row without an index: %q", row)
	}
	return strconv.Atoi(row[i+1:])
}

// pick shows the sessions in fzf. ok is false when the user closes fzf.
func pick(sessions []session.Session, held map[string]session.Holder, query string) (s session.Session, ok bool, err error) {
	if len(sessions) == 0 {
		return s, false, errors.New("found no Claude Code sessions")
	}
	var in bytes.Buffer
	for i, s := range sessions {
		_, live := held[s.ID]
		in.WriteString(fzfRow(i, s, live))
		in.WriteByte('\n')
	}
	args := []string{"--reverse", "--no-multi", "--delimiter", "\t", "--with-nth", "1",
		"--prompt", "session> ", "--exit-0",
		"--header", "Enter: resume the session. Esc: close. ● = open in a live Claude process."}
	if query != "" {
		args = append(args, "--query", query, "--select-1")
	}
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		// fzf exits with 1 when nothing matches and with 130 on Esc.
		if errors.As(err, &exit) && (exit.ExitCode() == 1 || exit.ExitCode() == 130) {
			return s, false, nil
		}
		if errors.Is(err, exec.ErrNotFound) {
			return s, false, errors.New("fzf is not on PATH. Install it: https://github.com/junegunn/fzf#installation")
		}
		return s, false, fmt.Errorf("fzf: %w", err)
	}
	i, err := rowIndex(string(out))
	if err != nil {
		return s, false, err
	}
	if i < 0 || i >= len(sessions) {
		return s, false, fmt.Errorf("fzf returned row %d of %d", i, len(sessions))
	}
	return sessions[i], true, nil
}
