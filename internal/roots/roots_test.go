package roots

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T, instance []string, allowUser bool) (*Store, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	store := NewStore(instance, allowUser)
	store.homeDir = func(string) string { return home }
	return store, home
}

func TestListCombinesInstanceAndPersonalRoots(t *testing.T) {
	store, home := newTestStore(t, []string{"/", "~"}, true)

	entries := store.List("ian")
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want the instance root and the home root", entries)
	}
	if entries[0].Path != "/" || entries[0].Label != "/" || entries[0].Source != SourceInstance {
		t.Errorf("first entry = %+v, want the filesystem root", entries[0])
	}
	if entries[1].Path != home || entries[1].Label != "~" || entries[1].Source != SourceInstance {
		t.Errorf("second entry = %+v, want the home directory", entries[1])
	}
}

func TestEmptyInstanceRootsFallBackToFilesystemRoot(t *testing.T) {
	store, _ := newTestStore(t, nil, true)
	entries := store.List("ian")
	if len(entries) != 1 || entries[0].Path != "/" {
		t.Fatalf("entries = %+v, want the filesystem root", entries)
	}
}

func TestAddAndRemovePersonalRoot(t *testing.T) {
	store, home := newTestStore(t, []string{"/"}, true)

	target := filepath.Join(home, "projects")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.Add("ian", target); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	entries := store.List("ian")
	found := false
	for _, entry := range entries {
		if entry.Path == target {
			found = true
			if entry.Source != SourceUser {
				t.Errorf("source = %q, want %q", entry.Source, SourceUser)
			}
			if entry.Label != "projects" {
				t.Errorf("label = %q, want %q", entry.Label, "projects")
			}
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want the personal root", entries)
	}

	// The personal root survives a reload from disk.
	reloaded := NewStore([]string{"/"}, true)
	reloaded.homeDir = func(string) string { return home }
	entries = reloaded.List("ian")
	found = false
	for _, entry := range entries {
		if entry.Path == target && entry.Source == SourceUser {
			found = true
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want the persisted personal root", entries)
	}

	if err := store.Remove("ian", target); err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	for _, entry := range store.List("ian") {
		if entry.Path == target {
			t.Fatalf("entries = %+v, want the personal root gone", store.List("ian"))
		}
	}
	if err := store.Remove("ian", target); err == nil {
		t.Error("removing an unknown personal root should fail")
	}
}

func TestAddRejectsUnusablePaths(t *testing.T) {
	store, home := newTestStore(t, []string{"/"}, true)

	if err := store.Add("ian", filepath.Join(home, "absent")); err == nil {
		t.Error("a missing directory should be rejected")
	}
	file := filepath.Join(home, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Add("ian", file); err == nil {
		t.Error("a file should be rejected")
	}
}

func TestAddDeduplicatesInstanceRoot(t *testing.T) {
	store, home := newTestStore(t, []string{"/"}, true)
	if err := store.Add("ian", "/"); err != nil {
		t.Fatalf("Add error: %v", err)
	}
	count := 0
	for _, entry := range store.List("ian") {
		if entry.Path == "/" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("filesystem root listed %d times, want once", count)
	}
	_ = home
}

func TestPersonalRootsDisabled(t *testing.T) {
	store, home := newTestStore(t, []string{"/"}, false)
	if err := store.Add("ian", home); err == nil {
		t.Error("Add should fail when personal roots are disabled")
	}
	if err := store.Remove("ian", home); err == nil {
		t.Error("Remove should fail when personal roots are disabled")
	}
	for _, entry := range store.List("ian") {
		if entry.Source == SourceUser {
			t.Fatalf("entries = %+v, want no personal roots", store.List("ian"))
		}
	}
}

func TestExpandResolvesHomeAndRelativePaths(t *testing.T) {
	home := "/home/ian"
	for _, tc := range []struct{ in, want string }{
		{"", home},
		{"~", home},
		{"~/work", "/home/ian/work"},
		{"work", "/home/ian/work"},
		{"/data", "/data"},
		{"/data/", "/data"},
		{"/data/../etc", "/etc"},
	} {
		got, err := Expand(tc.in, home)
		if err != nil {
			t.Fatalf("Expand(%q) error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("Expand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExpandRawKeepsParentSegments(t *testing.T) {
	got, err := ExpandRaw("/data/link/..", "/home/ian")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/data/link/.." {
		t.Errorf("ExpandRaw = %q, want the uncleaned path", got)
	}
}

func TestLabel(t *testing.T) {
	home := "/home/ian"
	for _, tc := range []struct{ path, want string }{
		{home, "~"},
		{"/", "/"},
		{"/data", "data"},
		{"/data/projects", "projects"},
	} {
		if got := Label(tc.path, home); got != tc.want {
			t.Errorf("Label(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
