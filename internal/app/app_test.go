package app

import (
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AIToolSharing/anyresume/internal/herdr"
	"github.com/AIToolSharing/anyresume/internal/session"
	"pgregory.net/rapid"
)

func randomSession(t *rapid.T) session.Session {
	return session.Session{
		ID:         rapid.StringMatching(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`).Draw(t, "id"),
		Title:      rapid.String().Draw(t, "title"),
		LastActive: time.Unix(rapid.Int64Range(0, 4e9).Draw(t, "time"), 0),
		Archived:   rapid.Bool().Draw(t, "archived"),
	}
}

// TestRowIndexRoundTrip checks that every title, also with tabs and line
// breaks, gives one fzf row and that the row gives back its index.
func TestRowIndexRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		s := randomSession(rt)
		i := rapid.IntRange(0, 1<<20).Draw(rt, "index")
		row := fzfRow(i, s, rapid.Bool().Draw(rt, "live"))
		if strings.ContainsAny(displayRow(s, false), "\t\r\n") {
			rt.Fatalf("displayRow has a tab or a line break: %q", displayRow(s, false))
		}
		got, err := rowIndex(row + "\n")
		if err != nil || got != i {
			rt.Fatalf("rowIndex(%q) = %d, %v; want %d", row, got, err, i)
		}
	})
}

var agentNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// TestAgentNameIsValid checks the herdr name rule for every title and ID.
func TestAgentNameIsValid(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		s := session.Session{ID: rapid.String().Draw(rt, "id"), Title: rapid.String().Draw(rt, "title")}
		if name := agentName(s); !agentNameRE.MatchString(name) {
			rt.Fatalf("agentName(%q, %q) = %q, not a valid herdr agent name", s.Title, s.ID, name)
		}
	})
}

func TestAgentNameExamples(t *testing.T) {
	cases := map[string]string{
		"Deploy Android APKs to Play Store": "deploy-android-apks-to-pla-2d5b",
		"四字熟語 deck card updates":            "deck-card-updates-2d5b",
		"2026 plan":                         "chat-2026-plan-2d5b",
		"":                                  "chat-2d5b",
	}
	for title, want := range cases {
		if got := agentName(session.Session{ID: "2d5b72c7-8fb9", Title: title}); got != want {
			t.Errorf("agentName(%q) = %q, want %q", title, got, want)
		}
	}
}

// TestTabLabel checks that a label is one short, non-empty line.
func TestTabLabel(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		l := tabLabel(rapid.String().Draw(rt, "title"))
		if l == "" || utf8.RuneCountInString(l) > tabLabelMax {
			rt.Fatalf("tabLabel = %q", l)
		}
		for _, r := range l {
			if unicode.IsControl(r) {
				rt.Fatalf("tabLabel %q has a control character", l)
			}
		}
	})
}

func TestPromptRE(t *testing.T) {
	for _, s := range []string{`PS C:\Users\a>`, `PS C:\Users\a> `, `C:\work>`, "user@host:~$ ", "% ", "root# ", "C:\\a>\r\n"} {
		if !promptRE.MatchString(strings.TrimRight(s, "\r\n")) {
			t.Errorf("promptRE does not match the empty prompt %q", s)
		}
	}
	for _, s := range []string{`C:\work>anyresume.exe resume 1234`, `PS C:\> claude`, "$ ls", ""} {
		if promptRE.MatchString(s) {
			t.Errorf("promptRE matches %q, which is not an empty prompt", s)
		}
	}
}

func TestPlanImport(t *testing.T) {
	day := time.Date(2026, 9, 26, 12, 0, 0, 0, time.Local)
	sessions := []session.Session{
		{ID: "running", LastActive: day},
		{ID: "imported", LastActive: day},
		{ID: "closed-tab", LastActive: day.AddDate(0, 0, -1)},
		{ID: "new", LastActive: day.AddDate(0, 0, -2)},
	}
	st := state{Tabs: map[string]importedTab{
		"imported":   {Tab: "w4:t1", Pane: "w4:p1"},
		"closed-tab": {Tab: "w4:t2", Pane: "w4:p2"},
	}}
	snap := herdr.Snapshot{Panes: []herdr.Pane{
		{ID: "w1:p1", AgentSession: &herdr.AgentSession{Value: "running"}},
		{ID: "w4:p1"},
	}}

	got := planImport(sessions, st, snap, false)
	want := []importStep{
		{kind: stepRetype, session: sessions[1], tab: importedTab{Tab: "w4:t1", Pane: "w4:p1"}},
		{kind: stepCreate, session: sessions[2], workspace: "chats 09-25"},
		{kind: stepCreate, session: sessions[3], workspace: "chats 09-24"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("planImport =\n%+v\nwant\n%+v", got, want)
	}

	refresh := planImport(sessions, st, snap, true)
	if len(refresh) != 1 || refresh[0].kind != stepRetype {
		t.Fatalf("planImport with refresh = %+v, want only the retype step", refresh)
	}
}

// TestStateRoundTrip checks that save and loadState keep every record.
func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		tabs := rapid.MapOf(rapid.String(), rapid.Custom(func(t *rapid.T) importedTab {
			return importedTab{Tab: rapid.String().Draw(t, "tab"), Pane: rapid.String().Draw(t, "pane")}
		})).Draw(rt, "tabs")
		path := filepath.Join(dir, "sub", "state.json")
		if err := (state{Version: 1, Tabs: tabs}).save(path); err != nil {
			rt.Fatalf("save: %v", err)
		}
		got, err := loadState(path)
		if err != nil {
			rt.Fatalf("loadState: %v", err)
		}
		if len(tabs) == 0 && len(got.Tabs) == 0 {
			return
		}
		if !reflect.DeepEqual(got.Tabs, tabs) {
			rt.Fatalf("loadState = %v, want %v", got.Tabs, tabs)
		}
	})
}

func TestLoadStateMissingFile(t *testing.T) {
	st, err := loadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || st.Tabs == nil || len(st.Tabs) != 0 {
		t.Fatalf("loadState(missing) = %+v, %v", st, err)
	}
}

func TestFindSession(t *testing.T) {
	sessions := []session.Session{{ID: "aaaaaaaa-1"}, {ID: "aaaaaaaa-2"}, {ID: "bbbbbbbb-1"}}
	if s, err := findSession(sessions, "bbbbbbbb"); err != nil || s.ID != "bbbbbbbb-1" {
		t.Fatalf("findSession(unique prefix) = %v, %v", s, err)
	}
	if _, err := findSession(sessions, "aaaaaaaa"); err == nil {
		t.Fatal("findSession(ambiguous prefix) gave no error")
	}
	if _, err := findSession(sessions, "bbb"); err == nil {
		t.Fatal("findSession(short prefix) gave no error")
	}
	if s, err := findSession(sessions, "aaaaaaaa-2"); err != nil || s.ID != "aaaaaaaa-2" {
		t.Fatalf("findSession(exact) = %v, %v", s, err)
	}
}
