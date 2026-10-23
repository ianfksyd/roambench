package agentmem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPointerCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")

	result, err := ApplyPointer(path, "project-abc")
	if err != nil {
		t.Fatalf("ApplyPointer error: %v", err)
	}
	if !result.Created || !result.Changed {
		t.Fatalf("result = %+v, want created and changed", result)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "roambench-agent memory get --project project-abc") {
		t.Fatalf("pointer missing:\n%s", content)
	}
	if !strings.Contains(content, "roambench-agent memory append --project project-abc") {
		t.Fatalf("append hint missing:\n%s", content)
	}
}

func TestApplyPointerIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	if _, err := ApplyPointer(path, "project-abc"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyPointer(path, "project-abc")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed {
		t.Fatal("second apply should not change the file")
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("content changed on re-apply:\n%s\n---\n%s", first, second)
	}
	if strings.Count(string(second), "roambench:memory:begin") != 1 {
		t.Fatalf("pointer duplicated:\n%s", second)
	}
}

func TestApplyPointerKeepsExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	original := "# 项目约定\n\n不要提交密钥。\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyPointer(path, "project-abc"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "不要提交密钥。") {
		t.Fatalf("existing content lost:\n%s", content)
	}
	if !strings.Contains(content, "roambench:memory:begin") {
		t.Fatalf("pointer missing:\n%s", content)
	}
}

func TestApplyPointerKeepsOtherProjectBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	if _, err := ApplyPointer(path, "project-one"); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPointer(path, "project-two"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "project-one") || !strings.Contains(content, "project-two") {
		t.Fatalf("both pointers expected:\n%s", content)
	}

	// Re-applying one project leaves the other untouched.
	if _, err := ApplyPointer(path, "project-one"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "project-two") {
		t.Fatalf("other project block lost:\n%s", data)
	}
}

func TestApplyPointerBacksUpExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	original := "old content\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := ApplyPointer(path, "project-abc")
	if err != nil {
		t.Fatal(err)
	}
	if result.BackupPath == "" {
		t.Fatal("expected a backup path")
	}
	backup, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatalf("backup missing: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup = %q, want %q", backup, original)
	}
}

func TestApplyPointerRejectsEmptyProjectID(t *testing.T) {
	dir := t.TempDir()
	if _, err := ApplyPointer(filepath.Join(dir, "CLAUDE.md"), "  "); err == nil {
		t.Fatal("expected an error for an empty project id")
	}
}

func TestGenericPointerIsProjectAgnostic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if _, err := ApplyGenericPointer(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, GenericScope) {
		t.Fatalf("generic scope missing:\n%s", content)
	}
	if strings.Contains(content, "project=project-") {
		t.Fatalf("generic block must not name a project:\n%s", content)
	}
	// Applying it twice keeps one block.
	if _, err := ApplyGenericPointer(path); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), "roambench:memory:begin") != 1 {
		t.Fatalf("generic pointer duplicated:\n%s", data)
	}
}

func TestGenericAndProjectBlocksCoexist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if _, err := ApplyGenericPointer(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPointer(path, "project-abc"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, GenericScope) || !strings.Contains(content, "project=project-abc") {
		t.Fatalf("both blocks expected:\n%s", content)
	}
}

func TestRemovePointer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("# keep me\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPointer(path, "project-abc"); err != nil {
		t.Fatal(err)
	}
	removed, err := RemovePointer(path, "project=project-abc")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected the block to be removed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "roambench:memory:begin") {
		t.Fatalf("block still present:\n%s", data)
	}
	if !strings.Contains(string(data), "# keep me") {
		t.Fatalf("other content lost:\n%s", data)
	}
	removed, err = RemovePointer(path, "project=project-abc")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("removing a missing block should report nothing")
	}
}

func TestCandidatesFindsExistingMemoryFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	found := Candidates(dir)
	if len(found) != 1 || filepath.Base(found[0]) != "CLAUDE.md" {
		t.Fatalf("candidates = %v, want only CLAUDE.md", found)
	}
}
