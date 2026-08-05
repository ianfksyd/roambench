//go:build phase0_repro

package server

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/terminal"
)

// TestOSCNotificationIngestContinuesWithoutTerminalWebSocket is the phase 0
// reproducer for the attach-scoped OSC scanner. It is intentionally behind the
// phase0_repro build tag until the persistent tmux observer is implemented.
//
// Production change that makes this test pass: start one server-owned observer
// for every managed tmux pane and feed its output to the notification ingest
// path independently of terminal WebSocket attachment.
func TestOSCNotificationIngestContinuesWithoutTerminalWebSocket(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is required for the phase 0 OSC lifecycle reproducer")
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	// Isolate this test from the user's normal tmux server and sessions.
	t.Setenv("TMUX_TMPDIR", t.TempDir())

	cfg := config.Defaults()
	cfg.Auth.SingleUser = currentUser.Username
	cfg.Terminal.PersistDir = t.TempDir()
	mgr := terminal.NewManager(&cfg.Terminal)
	defer mgr.Stop()
	if !mgr.HasTmux() {
		t.Fatal("terminal manager did not enable tmux")
	}

	session, err := mgr.CreateSession(currentUser.Username)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	defer mgr.KillSessionForUser(currentUser.Username, session.ID)

	srv := NewServer(cfg, nil, nil, mgr, nil)
	notifications := srv.notifHub.subscribe()
	defer srv.notifHub.unsubscribe(notifications)

	message := "roambench-phase0-browser-closed"
	command := fmt.Sprintf("printf '\\033]9;%s\\007'", message)
	if output, err := exec.Command("tmux", "send-keys", "-t", session.ID, command, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys error: %v (%s)", err, strings.TrimSpace(string(output)))
	}

	const observationWindow = 750 * time.Millisecond
	select {
	case notification := <-notifications:
		if notification.Body != message {
			t.Fatalf("notification body = %q, want %q", notification.Body, message)
		}
	case <-time.After(observationWindow):
		t.Fatalf("OSC notification was not ingested without a terminal WebSocket within %s; scanner lifecycle is still attach-scoped", observationWindow)
	}
}
