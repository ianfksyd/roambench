package filebrowser

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func currentUserHome(t *testing.T) (*user.User, string) {
	t.Helper()

	u, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}
	if u.HomeDir == "" {
		t.Fatal("current user home directory is empty")
	}
	return u, u.HomeDir
}

func makeHomeScopedTempDir(t *testing.T) (string, string) {
	t.Helper()

	u, home := currentUserHome(t)
	dir, err := os.MkdirTemp(home, ".liteterm-filebrowser-test-")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return u.Username, dir
}

func TestCopyFileCreatesIndependentTarget(t *testing.T) {
	username, dir := makeHomeScopedTempDir(t)
	fb := New()

	src := filepath.Join(dir, "notes.txt")
	dest := filepath.Join(dir, "notes-copy.txt")
	if err := os.WriteFile(src, []byte("hello"), 0640); err != nil {
		t.Fatalf("WriteFile src error: %v", err)
	}

	if err := fb.Copy(username, src, dest); err != nil {
		t.Fatalf("Copy error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile dest error: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("dest content = %q, want %q", string(data), "hello")
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("Stat dest error: %v", err)
	}
	if info.Mode().Perm() != 0640 {
		t.Fatalf("dest mode = %v, want %v", info.Mode().Perm(), os.FileMode(0640))
	}
}

func TestCopyDirectoryRecursivelyCopiesChildren(t *testing.T) {
	username, dir := makeHomeScopedTempDir(t)
	fb := New()

	srcDir := filepath.Join(dir, "src")
	nestedDir := filepath.Join(srcDir, "nested")
	destDir := filepath.Join(dir, "src-copy")
	if err := os.MkdirAll(nestedDir, 0750); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "a.txt"), []byte("alpha"), 0600); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	if err := fb.Copy(username, srcDir, destDir); err != nil {
		t.Fatalf("Copy error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "nested", "a.txt"))
	if err != nil {
		t.Fatalf("ReadFile copied child error: %v", err)
	}
	if string(data) != "alpha" {
		t.Fatalf("copied child content = %q, want %q", string(data), "alpha")
	}
}

func TestCopyRejectsDirectoryIntoItself(t *testing.T) {
	username, dir := makeHomeScopedTempDir(t)
	fb := New()

	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	err := fb.Copy(username, srcDir, filepath.Join(srcDir, "nested", "copy"))
	if err == nil {
		t.Fatal("Copy error = nil, want failure when copying directory into itself")
	}
}
