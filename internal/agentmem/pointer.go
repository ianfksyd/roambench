// Package agentmem writes the pointer that tells a coding agent where the
// canonical project memory lives.
//
// Every agent CLI keeps its own memory file (CLAUDE.md, AGENTS.md, ...). If
// each of them stores its own copy of the project facts, the facts get
// re-read and re-written by every agent. Instead the file only carries a
// pointer to the shared memory, and the agent reads it through
// `roambench-agent memory get`.
package agentmem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// beginPrefix and endMarker delimit the pointer block. The key after the
	// prefix selects the block: "scope=all" for the generic pointer,
	// "project=<id>" for a project specific one.
	beginPrefix = "<!-- roambench:memory:begin"
	endMarker   = "<!-- roambench:memory:end -->"
)

// GenericScope is the key of the project agnostic pointer block.
const GenericScope = "scope=all"

// memoryFiles are the file names agents read for their own instructions.
var memoryFiles = []string{"CLAUDE.md", "AGENTS.md"}

// PointerBlock renders the pointer block for one project.
func PointerBlock(projectID string) string {
	projectID = strings.TrimSpace(projectID)
	return fmt.Sprintf(`%s project=%s -->
## 项目记忆(RoamBench canonical memory)

本项目的共享事实只有一份,存放在 RoamBench 里。不要在本地另存副本,
也不要凭记忆复述:每次需要项目上下文时读取,需要补充时追加。

- 读取: roambench-agent memory get --project %s
- 追加: roambench-agent memory append --project %s --text "..."
%s`, beginPrefix, projectID, projectID, projectID, endMarker)
}

// GenericPointerBlock renders the project agnostic pointer block.
//
// It does not name a project: an agent learns the project from the task it
// claimed, so one block per file is enough and the file does not grow with
// the number of projects.
func GenericPointerBlock() string {
	return fmt.Sprintf(`%s %s -->
## 项目记忆(RoamBench canonical memory)

项目的共享事实存在 RoamBench 里,不要在本地另存副本,也不要凭记忆复述。

- 读取(把 <项目 id> 换成当前任务所属项目): roambench-agent memory get --project <项目 id>
- 追加: roambench-agent memory append --project <项目 id> --text "..."
- 当前任务的共享记忆会随 claim 响应一起下发(projectMemory)
%s`, beginPrefix, GenericScope, endMarker)
}

// Result reports what ApplyPointer changed.
type Result struct {
	Path       string `json:"path"`
	Changed    bool   `json:"changed"`
	Created    bool   `json:"created"`
	BackupPath string `json:"backupPath,omitempty"`
}

// Candidates returns the agent memory files that exist in a directory.
func Candidates(dir string) []string {
	var found []string
	for _, name := range memoryFiles {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			found = append(found, path)
		}
	}
	return found
}

// ApplyPointer writes the pointer for projectID into path, idempotently.
//
// An existing block for the same key is replaced in place; other blocks and
// the rest of the file are preserved. The previous content is backed up next
// to the file before it is modified.
func ApplyPointer(path, projectID string) (Result, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return Result{}, fmt.Errorf("project id is required")
	}
	return applyBlock(path, "project="+projectID, PointerBlock(projectID))
}

// ApplyGenericPointer writes the project agnostic pointer into path.
func ApplyGenericPointer(path string) (Result, error) {
	return applyBlock(path, GenericScope, GenericPointerBlock())
}

// RemovePointer removes the block with the given key from path. It reports
// whether something was removed.
func RemovePointer(path, key string) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("pointer key is required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	updated, changed := removeBlock(string(content), key)
	if !changed {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// applyBlock writes a block with the given key into a file.
func applyBlock(path, key, block string) (Result, error) {
	result := Result{Path: path}
	if strings.TrimSpace(path) == "" {
		return result, fmt.Errorf("pointer path is required")
	}
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	exists := err == nil
	content := string(original)
	updated, changed := replaceBlock(content, key, block)
	if exists && !changed {
		return result, nil
	}
	if exists {
		backup := backupPath(path)
		if err := os.WriteFile(backup, original, 0644); err != nil {
			return result, fmt.Errorf("cannot back up %s: %w", path, err)
		}
		result.BackupPath = backup
	} else {
		result.Created = true
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return result, err
	}
	if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}

// beginMarker returns the marker of a block key.
func beginMarker(key string) string {
	return beginPrefix + " " + key + " -->"
}

// replaceBlock swaps the pointer block of one key and reports whether the
// file content changed.
func replaceBlock(content, key, block string) (string, bool) {
	begin := beginMarker(key)
	start := strings.Index(content, begin)
	if start < 0 {
		trimmed := strings.TrimRight(content, "\n")
		if trimmed == "" {
			return block + "\n", true
		}
		return trimmed + "\n\n" + block + "\n", true
	}
	end := strings.Index(content[start:], endMarker)
	if end < 0 {
		// Unterminated block: replace to the end of the file.
		return content[:start] + block + "\n", true
	}
	end += start + len(endMarker)
	updated := content[:start] + block + content[end:]
	return updated, updated != content
}

// removeBlock drops the pointer block of one key.
func removeBlock(content, key string) (string, bool) {
	begin := beginMarker(key)
	start := strings.Index(content, begin)
	if start < 0 {
		return content, false
	}
	end := strings.Index(content[start:], endMarker)
	if end < 0 {
		return strings.TrimRight(content[:start], "\n") + "\n", true
	}
	end += start + len(endMarker)
	updated := content[:start] + content[end:]
	updated = strings.TrimRight(updated, "\n\n") + "\n"
	return updated, true
}

// backupPath returns the path the previous content is copied to.
func backupPath(path string) string {
	return path + ".roambench-bak"
}
