package gitstatus

import (
	"strconv"
	"strings"
)

// CodeChanges compares tracked working-tree content (index + unstaged) with
// HEAD. Untracked files are counted separately; binary files have no line count.
type CodeChanges struct {
	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Files     int    `json:"files"`
	Base      string `json:"base"`
	Untracked int    `json:"untracked"`
	Binary    int    `json:"binary"`
	Error     string `json:"error,omitempty"`
}

func FetchCodeChanges(repoPath string) CodeChanges {
	var result CodeChanges
	run := func(args ...string) (string, error) { return runGit("git", repoPath, CommandTimeout, args...) }
	if _, err := run("rev-parse", "--show-toplevel"); err != nil {
		result.Error = err.Error()
		return result
	}
	base, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		base, err = run("hash-object", "-t", "tree", "--stdin")
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Base = base
	out, err := run("diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--numstat", "-z", base, "--")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, record := range strings.Split(out, "\x00") {
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		result.Files++
		if fields[0] == "-" || fields[1] == "-" {
			result.Binary++
			continue
		}
		added, _ := strconv.Atoi(fields[0])
		removed, _ := strconv.Atoi(fields[1])
		result.Added += added
		result.Removed += removed
	}
	out, err = run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, file := range strings.Split(out, "\x00") {
		if file != "" {
			result.Untracked++
		}
	}
	return result
}
