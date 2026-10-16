package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/user"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/terminal"
)

type sessionReader func(*config.TerminalConfig, string) ([]terminal.SessionInfo, error)

func runSessionCommand(args []string, out, errOut io.Writer, read sessionReader) error {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(errOut)
	configPath := flags.String("config", "", "Path to config file")
	var stale time.Duration
	var jsonOutput bool
	if args[0] == "ls" {
		flags.DurationVar(&stale, "stale", 0, "Only sessions idle longer than this duration (e.g. 2h)")
		flags.BoolVar(&jsonOutput, "json", false, "Output JSON")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if stale < 0 {
		return errors.New("--stale must be a positive duration")
	}
	var invalidStale bool
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "stale" && stale == 0 {
			invalidStale = true
		}
	})
	if invalidStale {
		return errors.New("--stale must be a positive duration")
	}
	if args[0] == "ls" && flags.NArg() != 0 {
		return errors.New("usage: roambench ls [--stale 2h] [--json] [--config path]")
	}
	if args[0] == "attach" && (flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "") {
		return errors.New("usage: roambench attach [--config path] <name fragment>")
	}
	cfg := config.FindAndLoad()
	if *configPath != "" {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			return err
		}
	}
	current, err := user.Current()
	if err != nil {
		return err
	}
	sessions, err := read(&cfg.Terminal, current.Username)
	if err != nil {
		return err
	}
	if args[0] == "attach" {
		return printAttachCommand(out, sessions, flags.Arg(0))
	}
	selected := make([]terminal.SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		if stale > 0 && time.Duration(session.IdleSeconds)*time.Second <= stale {
			continue
		}
		selected = append(selected, session)
	}
	if jsonOutput {
		return json.NewEncoder(out).Encode(selected)
	}
	table := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "NAME\tPROJECT\tSTATUS\tIDLE")
	labels := map[string]string{terminal.StatusActive: "运行中", terminal.StatusIdle: "空闲>2h", terminal.StatusExited: "已退出", terminal.StatusDetached: "已分离>24h"}
	for _, session := range selected {
		project := session.ProjectID
		if project == "" {
			project = "-"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", tableText(session.Name), tableText(project), labels[session.Status], (time.Duration(session.IdleSeconds) * time.Second).String())
	}
	return table.Flush()
}

func tableText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

func printAttachCommand(out io.Writer, sessions []terminal.SessionInfo, query string) error {
	var matches []terminal.SessionInfo
	for _, session := range sessions {
		if strings.Contains(strings.ToLower(session.Name), strings.ToLower(strings.TrimSpace(query))) {
			matches = append(matches, session)
		}
	}
	if len(matches) == 0 {
		return fmt.Errorf("no session matches %q", query)
	}
	if len(matches) > 1 {
		return fmt.Errorf("%d sessions match %q; use a more specific name", len(matches), query)
	}
	match := matches[0]
	if match.Status == terminal.StatusExited || match.Backend != "tmux" {
		return fmt.Errorf("session %q has no live tmux session to attach", match.Name)
	}
	// Exact tmux target and shell quoting keep names/metadata out of shell syntax.
	target := "'" + strings.ReplaceAll("="+match.ID, "'", "'\"'\"'") + "'"
	_, err := fmt.Fprintf(out, "tmux attach-session -t %s\n", target)
	return err
}
