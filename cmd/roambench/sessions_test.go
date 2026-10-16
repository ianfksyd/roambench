package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/terminal"
)

func TestSessionCommands(t *testing.T) {
	sessions := []terminal.SessionInfo{
		{ID: "lt-active", Name: "rb-build", ProjectID: "roambench", Backend: "tmux", Status: terminal.StatusActive, IdleSeconds: 30},
		{ID: "lt-idle", Name: "rb-review", ProjectID: "roambench", Backend: "tmux", Status: terminal.StatusIdle, IdleSeconds: 10800},
		{ID: "lt-exited", Name: "old-task", Backend: "tmux", Status: terminal.StatusExited, IdleSeconds: 14400},
		{ID: "lt-detached", Name: "infra", Backend: "tmux", Status: terminal.StatusDetached, IdleSeconds: 90000},
		{ID: "pty", Name: "plain", Backend: "pty", Status: terminal.StatusActive},
	}
	read := func(_ *config.TerminalConfig, username string) ([]terminal.SessionInfo, error) {
		if username == "" {
			t.Fatal("missing local user")
		}
		return sessions, nil
	}
	for _, tc := range []struct {
		name   string
		args   []string
		want   []string
		absent []string
		fail   bool
	}{
		{name: "table", args: []string{"ls"}, want: []string{"NAME", "PROJECT", "STATUS", "IDLE", "rb-build", "roambench", "运行中", "空闲>2h", "已退出", "已分离>24h", "3h0m0s"}},
		{name: "stale", args: []string{"ls", "--stale", "2h"}, want: []string{"rb-review", "old-task", "infra"}, absent: []string{"rb-build", "plain"}},
		{name: "strict threshold", args: []string{"ls", "--stale", "3h"}, want: []string{"old-task"}, absent: []string{"rb-review"}},
		{name: "attach", args: []string{"attach", "BUILD"}, want: []string{"tmux attach-session -t '=lt-active'\n"}},
		{name: "ambiguous", args: []string{"attach", "rb-"}, fail: true},
		{name: "missing", args: []string{"attach", "absent"}, fail: true},
		{name: "empty", args: []string{"attach", " "}, fail: true},
		{name: "exited", args: []string{"attach", "old-task"}, fail: true},
		{name: "pty", args: []string{"attach", "plain"}, fail: true},
		{name: "negative", args: []string{"ls", "--stale", "-2h"}, fail: true},
		{name: "zero", args: []string{"ls", "--stale", "0s"}, fail: true},
		{name: "invalid duration", args: []string{"ls", "--stale", "soon"}, fail: true},
		{name: "extra argument", args: []string{"ls", "extra"}, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			err := runSessionCommand(tc.args, &out, &errOut, read)
			if (err != nil) != tc.fail {
				t.Fatalf("error=%v output=%s", err, out.String())
			}
			if tc.fail && out.Len() != 0 {
				t.Fatalf("failure printed command/output: %s", out.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output %q missing %q", out.String(), want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(out.String(), absent) {
					t.Errorf("output contains %q", absent)
				}
			}
		})
	}
	var out, errOut bytes.Buffer
	if err := runSessionCommand([]string{"ls", "--stale", "2h", "--json"}, &out, &errOut, read); err != nil {
		t.Fatal(err)
	}
	var decoded []terminal.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 3 || decoded[0].Name != "rb-review" || decoded[0].ProjectID != "roambench" || decoded[0].IdleSeconds != 10800 {
		t.Fatalf("JSON: %s", out.String())
	}
	out.Reset()
	if err := runSessionCommand([]string{"ls", "--json"}, &out, &errOut, func(*config.TerminalConfig, string) ([]terminal.SessionInfo, error) { return nil, nil }); err != nil || out.String() != "[]\n" {
		t.Fatalf("empty JSON = %q, %v", out.String(), err)
	}
	out.Reset()
	if err := runSessionCommand([]string{"ls"}, &out, &errOut, func(*config.TerminalConfig, string) ([]terminal.SessionInfo, error) {
		return nil, errors.New("probe failed")
	}); err == nil || out.Len() != 0 {
		t.Fatalf("probe failure lost: %v %q", err, out.String())
	}
}
