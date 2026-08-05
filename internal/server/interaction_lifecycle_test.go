package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/controlplane"
	"github.com/ianf339/roambench/internal/terminal"
)

func TestServerStartupExpiresOverdueInteractionsIdempotently(t *testing.T) {
	persistDir := t.TempDir()
	first, _, firstSessions := newInteractionAPIServer(t, persistDir)
	request := lifecyclePermissionRequest("startup-expiry", "session-expiry")
	request.ExpiresAt = time.Now().UTC().Add(40 * time.Millisecond)
	created, err := first.controlPlane.CreateInteraction(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}
	if err := first.controlPlane.Close(); err != nil {
		t.Fatalf("close first control plane: %v", err)
	}
	firstSessions.Stop()
	time.Sleep(60 * time.Millisecond)

	restarted, _, restartedSessions := newInteractionAPIServer(t, persistDir)
	interaction, err := restarted.controlPlane.GetInteraction(context.Background(), "ian", created.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction after restart: %v", err)
	}
	if interaction.Status != "expired" || interaction.RowVersion != 2 {
		t.Fatalf("interaction after restart = status %q rowVersion %d, want expired/2", interaction.Status, interaction.RowVersion)
	}
	if err := restarted.controlPlane.Close(); err != nil {
		t.Fatalf("close restarted control plane: %v", err)
	}
	restartedSessions.Stop()

	secondRestart, _, secondRestartSessions := newInteractionAPIServer(t, persistDir)
	defer secondRestartSessions.Stop()
	events, err := secondRestart.controlPlane.ListOutbox(context.Background(), "ian", 500)
	if err != nil {
		t.Fatalf("ListOutbox: %v", err)
	}
	expiredEvents := 0
	for _, event := range events {
		if event.AggregateID == created.RequestID && event.EventType == "interaction.expired" {
			expiredEvents++
		}
	}
	if expiredEvents != 1 {
		t.Fatalf("interaction.expired outbox events = %d, want 1", expiredEvents)
	}
}

func TestDeletingTerminalSessionCancelsOnlyItsPendingInteractions(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()
	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	defer sessions.Stop()
	sessionToken, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession auth: %v", err)
	}
	terminals := terminal.NewManager(&cfg.Terminal)
	defer terminals.Stop()
	terminalSession, err := terminals.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession terminal: %v", err)
	}
	srv := NewServer(cfg, nil, sessions, terminals, nil)
	defer srv.controlPlane.Close()

	pending, err := srv.controlPlane.CreateInteraction(context.Background(), lifecyclePermissionRequest("session-pending", terminalSession.ID))
	if err != nil {
		t.Fatalf("CreateInteraction pending: %v", err)
	}
	resolvedRequest := lifecyclePermissionRequest("session-resolved", terminalSession.ID)
	resolved, err := srv.controlPlane.CreateInteraction(context.Background(), resolvedRequest)
	if err != nil {
		t.Fatalf("CreateInteraction resolved: %v", err)
	}
	if _, _, err := srv.controlPlane.Respond(context.Background(), "ian", resolved.RequestID, controlplane.RespondInput{
		Action: "approve_once", Actor: "ian", ExpectedRowVersion: 1,
		IdempotencyKey: "resolved-before-session-end", InputHash: resolvedRequest.InputHash,
	}); err != nil {
		t.Fatalf("Respond resolved interaction: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/terminals/"+terminalSession.ID, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE terminal status = %d: %s", rec.Code, rec.Body.String())
	}

	pendingAfter, err := srv.controlPlane.GetInteraction(context.Background(), "ian", pending.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction pending: %v", err)
	}
	if pendingAfter.Status != "cancelled" || pendingAfter.RowVersion != 2 {
		t.Fatalf("pending interaction after session end = status %q rowVersion %d, want cancelled/2", pendingAfter.Status, pendingAfter.RowVersion)
	}
	resolvedAfter, err := srv.controlPlane.GetInteraction(context.Background(), "ian", resolved.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction resolved: %v", err)
	}
	if resolvedAfter.Status != "resolved" || resolvedAfter.FinalAction != "approve_once" || resolvedAfter.RowVersion != 2 {
		t.Fatalf("resolved interaction changed after session end: %#v", resolvedAfter)
	}
}

func TestTerminalManagerLifecycleCancellationClosesPendingInteractions(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()
	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	defer sessions.Stop()
	terminals := terminal.NewManager(&cfg.Terminal)
	defer terminals.Stop()
	srv := NewServer(cfg, nil, sessions, terminals, nil)
	defer srv.controlPlane.Close()
	terminalSession, err := terminals.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession terminal: %v", err)
	}
	interaction, err := srv.controlPlane.CreateInteraction(context.Background(), lifecyclePermissionRequest("manager-session-end", terminalSession.ID))
	if err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	if err := terminals.KillSessionForUser("ian", terminalSession.ID); err != nil {
		t.Fatalf("KillSessionForUser: %v", err)
	}
	closed, err := srv.controlPlane.GetInteraction(context.Background(), "ian", interaction.RequestID)
	if err != nil {
		t.Fatalf("GetInteraction: %v", err)
	}
	if closed.Status != "cancelled" || closed.RowVersion != 2 {
		t.Fatalf("interaction after manager session end = status %q rowVersion %d, want cancelled/2", closed.Status, closed.RowVersion)
	}
}

func lifecyclePermissionRequest(vendorRequestID, sessionID string) controlplane.CreateInteraction {
	return controlplane.CreateInteraction{
		Username: "ian", TaskID: projectControlTaskPanelID, RuntimeID: "runtime-lifecycle", SessionID: sessionID,
		AdapterKind: "generic", VendorRequestID: vendorRequestID, RequestKind: "permission", RiskClass: "R1",
		Title: "Lifecycle approval", Summary: "Wait for a human", Preview: "safe operation",
		AllowedActions: []string{"approve_once", "reject"}, ResponseSchema: controlplane.ResponseSchema{Type: "action"},
		InputHash: "sha256:" + vendorRequestID,
	}
}
