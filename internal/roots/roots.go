// Package roots resolves the browse roots offered by the file browser, the
// folder picker and the project path contract.
//
// Roots are quick-access entries, not a security boundary: the picker may
// navigate to any absolute path on the machine, and the instance owner
// decides where the shortcuts point. The instance ships its defaults in
// roambench.toml ([filesystem] roots); every user may keep personal extras
// in ~/.config/roambench/roots.toml.
package roots

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// SourceInstance marks a root defined by the instance configuration.
	SourceInstance = "instance"
	// SourceUser marks a root the signed-in user added for themselves.
	SourceUser = "user"
)

// userRootsFile is the per-user file holding personal roots.
const userRootsFile = "roots.toml"

// Entry is one browse root offered to the user.
type Entry struct {
	Path   string `json:"path"`
	Label  string `json:"label"`
	Source string `json:"source"`
}

// Store resolves the effective root list for a user.
type Store struct {
	instance  []string
	allowUser bool
	// homeDir resolves the acting user's home directory.
	homeDir func(username string) string
	// configDir resolves the directory holding the per-user roots file.
	configDir func(username string) string
}

// NewStore builds a store from the instance configuration. A nil or empty
// instance list falls back to the filesystem root, so a fresh install can
// browse the whole machine.
func NewStore(instanceRoots []string, allowUser bool) *Store {
	roots := make([]string, 0, len(instanceRoots))
	for _, root := range instanceRoots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			roots = append(roots, trimmed)
		}
	}
	if len(roots) == 0 {
		roots = []string{string(filepath.Separator)}
	}
	return &Store{
		instance:  roots,
		allowUser: allowUser,
		homeDir:   HomeDir,
		configDir: defaultConfigDir,
	}
}

// List returns the effective roots for a user: instance defaults first, then
// the user's personal extras. Entries are de-duplicated by resolved path.
func (s *Store) List(username string) []Entry {
	home := s.homeDir(username)
	entries := make([]Entry, 0, len(s.instance))
	seen := make(map[string]bool)
	add := func(path, source string) {
		expanded, err := Expand(path, home)
		if err != nil || expanded == "" {
			return
		}
		if seen[expanded] {
			return
		}
		seen[expanded] = true
		entries = append(entries, Entry{Path: expanded, Label: Label(expanded, home), Source: source})
	}
	for _, root := range s.instance {
		add(root, SourceInstance)
	}
	if s.allowUser {
		for _, root := range s.readUserRoots(username) {
			add(root, SourceUser)
		}
	}
	return entries
}

// Add records a personal root for the user. The path must be an existing
// directory so the shortcut never points at something unusable.
func (s *Store) Add(username, path string) error {
	if !s.allowUser {
		return errors.New("adding personal roots is disabled")
	}
	home := s.homeDir(username)
	expanded, err := Expand(path, home)
	if err != nil {
		return err
	}
	if expanded == "" {
		return errors.New("root path is required")
	}
	info, err := os.Stat(expanded)
	if err != nil {
		return fmt.Errorf("root path does not exist: %s", expanded)
	}
	if !info.IsDir() {
		return fmt.Errorf("root path is not a directory: %s", expanded)
	}
	personal := s.readUserRoots(username)
	for _, existing := range personal {
		existingExpanded, err := Expand(existing, home)
		if err == nil && existingExpanded == expanded {
			return nil
		}
	}
	personal = append(personal, expanded)
	return s.writeUserRoots(username, personal)
}

// Remove drops a personal root. Instance roots are not removable.
func (s *Store) Remove(username, path string) error {
	if !s.allowUser {
		return errors.New("removing personal roots is disabled")
	}
	home := s.homeDir(username)
	expanded, err := Expand(path, home)
	if err != nil {
		return err
	}
	personal := s.readUserRoots(username)
	kept := make([]string, 0, len(personal))
	found := false
	for _, existing := range personal {
		existingExpanded, expandErr := Expand(existing, home)
		if expandErr == nil && existingExpanded == expanded {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	if !found {
		return fmt.Errorf("root is not a personal root: %s", expanded)
	}
	return s.writeUserRoots(username, kept)
}

// readUserRoots loads the personal roots file. A missing or broken file
// yields no personal roots instead of failing the caller.
func (s *Store) readUserRoots(username string) []string {
	path := s.userRootsPath(username)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var file struct {
		Roots []string `toml:"roots"`
	}
	if _, err := toml.Decode(string(data), &file); err != nil {
		return nil
	}
	roots := make([]string, 0, len(file.Roots))
	for _, root := range file.Roots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			roots = append(roots, trimmed)
		}
	}
	return roots
}

func (s *Store) writeUserRoots(username string, roots []string) error {
	dir := s.configDir(username)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}
	file, err := os.OpenFile(s.userRootsPath(username), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("cannot write roots file: %w", err)
	}
	defer file.Close()
	encoder := toml.NewEncoder(file)
	encoder.Indent = ""
	if err := encoder.Encode(struct {
		Roots []string `toml:"roots"`
	}{Roots: roots}); err != nil {
		return fmt.Errorf("cannot write roots file: %w", err)
	}
	return nil
}

func (s *Store) userRootsPath(username string) string {
	return filepath.Join(s.configDir(username), userRootsFile)
}

// defaultConfigDir returns the per-user configuration directory, honouring
// XDG_CONFIG_HOME when it is set.
func defaultConfigDir(username string) string {
	if base := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); base != "" {
		return filepath.Join(base, "roambench")
	}
	home := HomeDir(username)
	if home == "" {
		return filepath.Join(os.TempDir(), "roambench")
	}
	return filepath.Join(home, ".config", "roambench")
}

// HomeDir resolves a user's home directory, falling back to the process home.
func HomeDir(username string) string {
	trimmed := strings.TrimSpace(username)
	if trimmed != "" {
		// The process home honours $HOME, so the signed-in user browses the
		// same tree the service account sees.
		if current, err := user.Current(); err == nil && current.Username == trimmed {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				return home
			}
		}
		if u, err := user.Lookup(trimmed); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home
	}
	return ""
}

// Expand turns a user-supplied path into an absolute, cleaned path. "~" and
// "~/x" resolve against the home directory, relative paths resolve against it
// as well.
func Expand(path, home string) (string, error) {
	expanded, err := ExpandRaw(path, home)
	if err != nil {
		return "", err
	}
	return filepath.Clean(expanded), nil
}

// ExpandRaw expands "~" and relative paths without cleaning the result, so
// that "link/.." keeps its POSIX meaning when the path is resolved later.
func ExpandRaw(path, home string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return home, nil
	}
	if trimmed == "~" {
		return home, nil
	}
	if strings.HasPrefix(trimmed, "~/") {
		if home == "" {
			return "", errors.New("home directory is unavailable")
		}
		return filepath.Join(home, trimmed[2:]), nil
	}
	if !filepath.IsAbs(trimmed) {
		if home == "" {
			return "", errors.New("home directory is unavailable")
		}
		return filepath.Join(home, trimmed), nil
	}
	return trimmed, nil
}

// Label renders a short display name for a root: "~" for the home directory,
// "/" for the filesystem root, otherwise the last path segment.
func Label(path, home string) string {
	if path == "" {
		return ""
	}
	if home != "" && path == home {
		return "~"
	}
	if path == string(filepath.Separator) {
		return string(filepath.Separator)
	}
	return filepath.Base(path)
}
