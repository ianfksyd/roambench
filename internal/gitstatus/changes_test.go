package gitstatus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodeChangesCombinedIndexWorkingTreeAndBinary(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "text\tfile\n.txt", "one\ntwo\n")
	base := commitFile(t, dir, "image.bin", "\x00old")
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("text\tfile\n.txt", "one\nstaged\n")
	gitRun(t, dir, "add", ".")
	write("text\tfile\n.txt", "one\nworking\nextra\n")
	write("image.bin", "\x00new")
	write("untracked\nfile", "not included in line counts\n")
	changes := FetchCodeChanges(dir)
	if changes.Error != "" || changes.Base != base || changes.Added != 2 || changes.Removed != 1 || changes.Files != 2 || changes.Binary != 1 || changes.Untracked != 1 {
		t.Fatalf("changes: %+v", changes)
	}
}

func TestCodeChangesUnbornAndNonRepository(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "new.txt")
	changes := FetchCodeChanges(dir)
	if changes.Error != "" || changes.Base == "" || changes.Added != 1 || changes.Files != 1 {
		t.Fatalf("unborn: %+v", changes)
	}
	missing := FetchCodeChanges(t.TempDir())
	if missing.Error == "" {
		t.Fatal("non-repository reported as zero changes")
	}
}

func TestCodeChangesDoesNotRunExternalDiff(t *testing.T) {
	dir := initRepo(t)
	commitFile(t, dir, "test.txt", "before\n")
	marker := filepath.Join(t.TempDir(), "unsafe")
	gitRun(t, dir, "config", "diff.external", "touch "+marker)
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte("after\n"), 0600); err != nil {
		t.Fatal(err)
	}
	changes := FetchCodeChanges(dir)
	if changes.Error != "" || changes.Added != 1 {
		t.Fatal(changes)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("external diff executed")
	}
}
