package app

import (
	"os"
	"path/filepath"
	"testing"

	"pgregory.net/rapid"
)

// TestStatePathIsPerSocket is the regression test for 0.1.2: one record
// file served every herdr session, and the plugin startup hook of a named
// session could type resume commands into panes that had the same IDs as
// the imported tabs of the default session.
func TestStatePathIsPerSocket(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a := rapid.String().Draw(rt, "socketA")
		b := rapid.String().Draw(rt, "socketB")
		pa, pb := statePath("cfg", a), statePath("cfg", b)
		if (a == b) != (pa == pb) {
			rt.Fatalf("statePath(%q) = %q and statePath(%q) = %q", a, pa, b, pb)
		}
		if pa == legacyStatePath("cfg") {
			rt.Fatalf("statePath(%q) is the old single record", a)
		}
	})
}

func TestMigrateLegacyState(t *testing.T) {
	dir := t.TempDir()
	legacy := legacyStatePath(dir)
	path := statePath(dir, `C:\herdr\herdr.sock`)
	old := state{Version: 1, Tabs: map[string]importedTab{"s1": {Tab: "w5:t3", Pane: "w5:p3"}}}
	if err := old.save(legacy); err != nil {
		t.Fatal(err)
	}

	if err := migrateLegacyState(legacy, path); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(path)
	if err != nil || got.Tabs["s1"] != old.Tabs["s1"] {
		t.Fatalf("after migration, loadState = %+v, %v; want the old record", got, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("the old record still exists: %v", err)
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Fatalf("the old record was not kept as .migrated: %v", err)
	}

	// A second migration must not replace the new record.
	other := state{Version: 1, Tabs: map[string]importedTab{"s2": {Tab: "w1:t1", Pane: "w1:p1"}}}
	if err := other.save(legacy); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyState(legacy, path); err != nil {
		t.Fatal(err)
	}
	got, _ = loadState(path)
	if _, ok := got.Tabs["s2"]; ok || len(got.Tabs) != 1 {
		t.Fatalf("migration replaced an existing record: %+v", got.Tabs)
	}
}

func TestMigrateWithoutLegacyState(t *testing.T) {
	dir := t.TempDir()
	path := statePath(dir, "sock")
	if err := migrateLegacyState(legacyStatePath(dir), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("migration made a record from nothing: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dir, "anyresume") {
		t.Fatalf("statePath = %q, want a file in %q", path, filepath.Join(dir, "anyresume"))
	}
}
