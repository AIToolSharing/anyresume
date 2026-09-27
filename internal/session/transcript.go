package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Transcript holds what anyresume reads from one Claude Code transcript,
// ~/.claude/projects/<folder>/<session id>.jsonl.
type Transcript struct {
	ID       string
	Path     string
	Cwd      string
	Modified time.Time
	// HasMessages is true when the transcript has a user or assistant
	// message. A transcript without messages has nothing to resume.
	HasMessages bool

	custom, ai, agent, summary, lastPrompt string
}

// Title returns the best title of the session: the title that the user or
// the desktop app gave it, then the title that Claude Code generated, then
// the agent name, the summary, and the last prompt.
func (t *Transcript) Title() string {
	for _, s := range []string{t.custom, t.ai, t.agent, t.summary, t.lastPrompt} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// chunkSize is the read buffer size. A line longer than chunkSize holds
// message content. The reader does not decode such a line. It only looks
// for "cwd" in it.
const chunkSize = 64 << 10

// cwdOverlap is the number of bytes that the reader keeps from the end of
// one chunk of a long line, so that it finds a "cwd" value that crosses a
// chunk border.
const cwdOverlap = 8 << 10

var (
	uuidRE      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	cwdKey      = []byte(`"cwd":"`)
	userType    = []byte(`"type":"user"`)
	assistType  = []byte(`"type":"assistant"`)
	titleTypes  = []string{"custom-title", "ai-title", "agent-name", "summary", "last-prompt"}
	titlePrefix [][]byte
)

func init() {
	for _, t := range titleTypes {
		titlePrefix = append(titlePrefix, []byte(`{"type":"`+t+`"`))
	}
}

type record struct {
	Type        string `json:"type"`
	CustomTitle string `json:"customTitle"`
	AITitle     string `json:"aiTitle"`
	AgentName   string `json:"agentName"`
	Summary     string `json:"summary"`
	LastPrompt  string `json:"lastPrompt"`
}

// ReadTranscript reads one transcript file.
func ReadTranscript(path string) (Transcript, error) {
	f, err := os.Open(path)
	if err != nil {
		return Transcript{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Transcript{}, err
	}
	t := Transcript{
		ID:       strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Path:     path,
		Modified: info.ModTime(),
	}
	if err := t.scan(f); err != nil {
		return Transcript{}, err
	}
	return t, nil
}

// scan reads the transcript line by line. It keeps the last value of each
// title record and the first "cwd" value.
func (t *Transcript) scan(r io.Reader) error {
	br := bufio.NewReaderSize(r, chunkSize)
	var tail []byte
	long := false
	for {
		chunk, err := br.ReadSlice('\n')
		if len(chunk) > 0 {
			if !long && err != bufio.ErrBufferFull {
				t.line(bytes.TrimRight(chunk, "\r\n"))
			} else {
				if !long {
					t.messageMark(chunk)
				}
				if t.Cwd == "" {
					buf := append(tail, chunk...)
					if cwd, ok := findCwd(buf); ok {
						t.Cwd = cwd
					}
					tail = append(tail[:0], lastBytes(chunk, cwdOverlap)...)
				}
			}
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			long = true
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		default:
			long = false
			tail = tail[:0]
		}
	}
}

// line reads one complete line.
func (t *Transcript) line(b []byte) {
	for _, p := range titlePrefix {
		if !bytes.HasPrefix(b, p) {
			continue
		}
		var rec record
		if json.Unmarshal(b, &rec) != nil {
			return
		}
		switch rec.Type {
		case "custom-title":
			keep(&t.custom, rec.CustomTitle)
		case "ai-title":
			keep(&t.ai, rec.AITitle)
		case "agent-name":
			keep(&t.agent, rec.AgentName)
		case "summary":
			keep(&t.summary, rec.Summary)
		case "last-prompt":
			keep(&t.lastPrompt, rec.LastPrompt)
		}
		return
	}
	t.messageMark(b)
	if t.Cwd == "" {
		if cwd, ok := findCwd(b); ok {
			t.Cwd = cwd
		}
	}
}

// messageMark sets HasMessages when b is the start of a user or assistant
// message. The "type" field comes before the message content, so the first
// chunk of a long line holds it.
func (t *Transcript) messageMark(b []byte) {
	if !t.HasMessages && (bytes.Contains(b, userType) || bytes.Contains(b, assistType)) {
		t.HasMessages = true
	}
}

func keep(dst *string, v string) {
	if strings.TrimSpace(v) != "" {
		*dst = v
	}
}

func lastBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	return b[len(b)-n:]
}

// findCwd returns the value of the first "cwd" member in b. Claude Code
// writes compact JSON, so the key is `"cwd":"`. An escaped key inside a JSON
// string reads `\"cwd\":\"`, which does not match.
func findCwd(b []byte) (string, bool) {
	i := bytes.Index(b, cwdKey)
	if i < 0 {
		return "", false
	}
	start := i + len(cwdKey) - 1
	for j := start + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			var s string
			if json.Unmarshal(b[start:j+1], &s) != nil {
				return "", false
			}
			return s, true
		}
	}
	return "", false
}

// ReadTranscripts reads every session transcript under projectsDir. It skips
// the transcripts of subagents and files that it cannot read.
func ReadTranscripts(projectsDir string) ([]Transcript, error) {
	dirs, err := os.ReadDir(projectsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(projectsDir, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			name := f.Name()
			if f.IsDir() || !strings.HasSuffix(name, ".jsonl") || !uuidRE.MatchString(strings.TrimSuffix(name, ".jsonl")) {
				continue
			}
			paths = append(paths, filepath.Join(projectsDir, d.Name(), name))
		}
	}

	out := make([]Transcript, len(paths))
	ok := make([]bool, len(paths))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, p := range paths {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			t, err := ReadTranscript(p)
			out[i], ok[i] = t, err == nil
		}()
	}
	wg.Wait()

	read := out[:0]
	for i, t := range out {
		if ok[i] {
			read = append(read, t)
		}
	}
	return read, nil
}
