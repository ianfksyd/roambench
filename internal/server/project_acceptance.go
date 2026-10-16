package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Do not inherit service credentials or interpreter injection variables.
func acceptanceEnvironment() []string {
	env := []string{}
	for _, key := range []string{"PATH", "HOME", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR", "TZ"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// Snapshot common test/build inputs and local command references, including
// additions/deletions, modes and symlink targets. This is a review tripwire,
// not a sandbox against an agent with the server's Unix identity.
func (s *projectControlStore) acceptanceFingerprint(state projectControlState, task projectControlTask) (string, error) {
	if len(task.AcceptanceCommand) == 0 {
		return "", nil
	}
	root, err := s.controlledProjectDir(state, task)
	if err != nil {
		return "", err
	}
	refs := map[string]bool{}
	for _, arg := range task.AcceptanceCommand {
		for _, token := range append([]string{arg}, strings.Fields(arg)...) {
			token = strings.Trim(token, "'\";()")
			if !filepath.IsAbs(token) {
				token = filepath.Join(root, token)
			}
			refs[filepath.Clean(token)] = true
		}
	}
	h := sha256.New()
	command, _ := json.Marshal(task.AcceptanceCommand)
	fmt.Fprintf(h, "command:%s\n", command)
	var total int64
	count := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				switch entry.Name() {
				case ".git", "node_modules", ".roambench", ".venv", "vendor":
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		name := strings.ToLower(entry.Name())
		testInput := false
		for _, part := range strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/") {
			if part == "test" || part == "tests" || part == "__tests__" || part == "testdata" || part == "spec" || part == "specs" {
				testInput = true
			}
		}
		switch name {
		case "makefile", "gnumakefile", "justfile", "dockerfile", "go.mod", "go.sum", "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "pyproject.toml", "pytest.ini", "tox.ini", "setup.cfg", "cargo.toml", "cargo.lock", "cmakelists.txt":
			testInput = true
		}
		testInput = testInput || strings.Contains(name, "test") || strings.Contains(name, ".spec.") || strings.HasSuffix(name, ".sh")
		if !testInput && !refs[path] && info.Mode().Perm()&0111 == 0 {
			return nil
		}
		count++
		if count > 10000 {
			return fmt.Errorf("too many acceptance inputs; human review required")
		}
		fmt.Fprintf(h, "file:%q mode:%s\n", rel, info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "link:%q\n", target)
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("acceptance input escapes repository: %s", rel)
			}
			info, err = os.Stat(path)
			if err != nil {
				return err
			}
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("acceptance input is not a regular file: %s", rel)
		}
		total += info.Size()
		if total > 64<<20 {
			return fmt.Errorf("acceptance inputs exceed 64 MiB; human review required")
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		digest := sha256.New()
		n, copyErr := io.Copy(digest, io.LimitReader(f, (64<<20)+1))
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n > 64<<20 {
			return fmt.Errorf("acceptance input exceeds 64 MiB")
		}
		fmt.Fprintf(h, "sha256:%x\n", digest.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
