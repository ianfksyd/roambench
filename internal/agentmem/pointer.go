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
	// beginPrefix and endPrefix delimit the pointer block. The project id is
	// part of the marker so several projects can share one file.
	beginPrefix = "<!-- roambench:memory:begin"
	endMarker   = "<!-- roambench:memory:end -->"
)

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

// Result reports what ApplyPointer changed.
type Result struct {
	Path       string
	Changed    bool
	Created    bool
	BackupPath string
}

// ApplyPointer writes the pointer for projectID into path, idempotently.
//
// An existing block for the same project is replaced in place; other
// projects' blocks and the rest of the file are preserved. The previous
// content is backed up next to the file before it is modified.
func ApplyPointer(path, projectID string) (Result, error) {
	result := Result{Path: path}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return result, fmt.Errorf("project id is required")
	}
	if strings.TrimSpace(path) == "" {
		return result, fmt.Errorf("pointer path is required")
	}

	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return result, err
	}
	exists := err == nil
	content := string(original)
	block := PointerBlock(projectID)

	updated, changed := replaceBlock(content, projectID, block)
	if exists && !changed {
		return result, nil
	}
	if !exists {
		result.Created = true
	} else {
		backup := backupPath(path)
		if err := os.WriteFile(backup, original, 0644); err != nil {
			return result, fmt.Errorf("cannot back up %s: %w", path, err)
		}
		result.BackupPath = backup
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

// replaceBlock swaps the pointer block of one project and reports whether
// the file content changed.
func replaceBlock(content, projectID, block string) (string, bool) {
	begin := beginPrefix + " project=" + projectID + " -->"
	start := strings.Index(content, begin)
	if start < 0 {
		// No block for this project yet: append one.
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

// backupPath returns the path the previous content is copied to.
func backupPath(path string) string {
	return path + ".roambench-bak"
}
