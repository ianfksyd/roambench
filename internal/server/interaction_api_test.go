package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
)

func newInteractionAPIServer(t *testing.T, persistDir string) (*Server, string, *auth.SessionManager) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = persistDir
	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	sessionToken, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	srv := NewServer(cfg, nil, sessions, nil, nil)
	t.Cleanup(func() {
		if srv.controlPlane != nil {
			_ = srv.controlPlane.Close()
		}
	})
	return srv, sessionToken, sessions
}

func issueAgentToken(t *testing.T, srv *Server, sessionToken string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/project-control/agent-token", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("issue agent token status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode agent token: %v", err)
	}
	return body["token"]
}

func createInteractionViaAPI(t *testing.T, srv *Server, agentToken, vendorID, kind, schema string) map[string]interface{} {
	t.Helper()
	body := fmt.Sprintf(`{
		"taskId":"task-1","runtimeId":"runtime-1","sessionId":"session-1",
		"adapterKind":"generic","vendorRequestId":%q,"requestKind":%q,"riskClass":"R1",
		"title":"Human input required","summary":"Choose safely","preview":"go test ./...",
		"allowedActions":["approve_once","reject","answer"],"responseSchema":%s,"inputHash":"sha256:abc"
	}`, vendorID, kind, schema)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/interactions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "create-"+vendorID)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST interaction status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	var interaction map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&interaction); err != nil {
		t.Fatalf("decode interaction: %v", err)
	}
	return interaction
}

func TestInteractionAPICompletesAgentToBrowserDecisionRoundTrip(t *testing.T) {
	srv, sessionToken, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	agentToken := issueAgentToken(t, srv, sessionToken)
	created := createInteractionViaAPI(t, srv, agentToken, "permission-1", "permission", `{"type":"action","maxFeedbackLength":500}`)
	requestID := created["requestId"].(string)

	respondBody := `{"action":"approve_once","selectedOptionIds":[],"feedback":"","expectedRowVersion":1,"idempotencyKey":"phone-decision-1","inputHash":"sha256:abc","deviceId":"phone"}`
	respondReq := httptest.NewRequest(http.MethodPost, "/api/mobile/interactions/"+requestID+"/respond", strings.NewReader(respondBody))
	respondReq.Header.Set("Content-Type", "application/json")
	respondReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	respondRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(respondRec, respondReq)
	if respondRec.Code != http.StatusOK {
		t.Fatalf("POST respond status = %d, want 200: %s", respondRec.Code, respondRec.Body.String())
	}

	waitReq := httptest.NewRequest(http.MethodGet, "/api/agent/v1/interactions/"+requestID+"/wait?timeout=50ms", nil)
	waitReq.Header.Set("Authorization", "Bearer "+agentToken)
	waitRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(waitRec, waitReq)
	if waitRec.Code != http.StatusOK {
		t.Fatalf("GET wait status = %d: %s", waitRec.Code, waitRec.Body.String())
	}
	var resolved map[string]interface{}
	if err := json.NewDecoder(waitRec.Body).Decode(&resolved); err != nil {
		t.Fatalf("decode wait response: %v", err)
	}
	if resolved["status"] != "resolved" || resolved["finalAction"] != "approve_once" || resolved["rowVersion"] != float64(2) {
		t.Fatalf("resolved interaction = %#v", resolved)
	}
}

func TestMobileInteractionResponseReturnsConflictWithCurrentState(t *testing.T) {
	srv, sessionToken, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	agentToken := issueAgentToken(t, srv, sessionToken)
	created := createInteractionViaAPI(t, srv, agentToken, "permission-2", "permission", `{"type":"action"}`)
	requestID := created["requestId"].(string)

	for index, action := range []string{"approve_once", "reject"} {
		body := fmt.Sprintf(`{"action":%q,"expectedRowVersion":1,"idempotencyKey":"decision-%d","inputHash":"sha256:abc"}`, action, index)
		req := httptest.NewRequest(http.MethodPost, "/api/mobile/interactions/"+requestID+"/respond", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)
		want := http.StatusOK
		if index == 1 {
			want = http.StatusConflict
		}
		if rec.Code != want {
			t.Fatalf("response %d status = %d, want %d: %s", index, rec.Code, want, rec.Body.String())
		}
		if index == 1 {
			var conflict map[string]interface{}
			if err := json.NewDecoder(rec.Body).Decode(&conflict); err != nil {
				t.Fatalf("decode conflict: %v", err)
			}
			current, ok := conflict["interaction"].(map[string]interface{})
			if !ok || current["finalAction"] != "approve_once" {
				t.Fatalf("conflict payload = %#v", conflict)
			}
		}
	}
}

func TestAgentCanCancelAndRetryReadAfterServerRestart(t *testing.T) {
	persistDir := t.TempDir()
	srv, sessionToken, sessions := newInteractionAPIServer(t, persistDir)
	agentToken := issueAgentToken(t, srv, sessionToken)
	created := createInteractionViaAPI(t, srv, agentToken, "cancel-restart", "permission", `{"type":"action"}`)
	requestID := created["requestId"].(string)

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/agent/v1/interactions/"+requestID+"/cancel", strings.NewReader(`{"expectedRowVersion":1,"reason":"adapter stopped"}`))
	cancelReq.Header.Set("Authorization", "Bearer "+agentToken)
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelReq.Header.Set("Idempotency-Key", "cancel-1")
	cancelRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("POST cancel status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	_ = srv.controlPlane.Close()
	sessions.Stop()

	restarted, restartedSessionToken, restartedSessions := newInteractionAPIServer(t, persistDir)
	defer restartedSessions.Stop()
	restartedAgentToken := issueAgentToken(t, restarted, restartedSessionToken)
	getReq := httptest.NewRequest(http.MethodGet, "/api/agent/v1/interactions/"+requestID, nil)
	getReq.Header.Set("Authorization", "Bearer "+restartedAgentToken)
	getRec := httptest.NewRecorder()
	restarted.mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET after restart status = %d: %s", getRec.Code, getRec.Body.String())
	}
	var interaction map[string]interface{}
	_ = json.NewDecoder(getRec.Body).Decode(&interaction)
	if interaction["status"] != "cancelled" {
		t.Fatalf("interaction after restart = %#v", interaction)
	}
}

func TestCreateInteractionIdempotencyReturnsOriginalResultAfterRestart(t *testing.T) {
	persistDir := t.TempDir()
	first, sessionToken, firstSessions := newInteractionAPIServer(t, persistDir)
	agentToken := issueAgentToken(t, first, sessionToken)
	body := `{
		"taskId":"task-create-idempotency","runtimeId":"runtime-1","sessionId":"session-1",
		"adapterKind":"generic","vendorRequestId":"create-idempotency","requestKind":"permission","riskClass":"R1",
		"title":"Approve deployment","summary":"Deploy the reviewed build","preview":"deploy --reviewed",
		"allowedActions":["approve_once","reject"],"responseSchema":{"type":"action"},"inputHash":"sha256:create-idempotency"
	}`
	firstResult, firstStatus := postAgentInteraction(t, first, agentToken, "create-persisted-key", body)
	if firstStatus != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201: %#v", firstStatus, firstResult)
	}
	requestID := firstResult["requestId"].(string)

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/agent/v1/interactions/"+requestID+"/cancel", strings.NewReader(`{"expectedRowVersion":1,"reason":"change live state"}`))
	cancelReq.Header.Set("Authorization", "Bearer "+agentToken)
	cancelReq.Header.Set("Content-Type", "application/json")
	cancelReq.Header.Set("Idempotency-Key", "create-test-cancel")
	cancelRec := httptest.NewRecorder()
	first.mux.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel before retry status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	if err := first.controlPlane.Close(); err != nil {
		t.Fatalf("close first control plane: %v", err)
	}
	firstSessions.Stop()

	restarted, restartedSessionToken, restartedSessions := newInteractionAPIServer(t, persistDir)
	defer restartedSessions.Stop()
	restartedAgentToken := issueAgentToken(t, restarted, restartedSessionToken)
	replayed, replayStatus := postAgentInteraction(t, restarted, restartedAgentToken, "create-persisted-key", body)
	if replayStatus != http.StatusCreated {
		t.Fatalf("replayed create status = %d, want 201: %#v", replayStatus, replayed)
	}
	if !reflect.DeepEqual(replayed, firstResult) {
		t.Fatalf("replayed create = %#v, want original %#v", replayed, firstResult)
	}

	changed := strings.Replace(body, "Deploy the reviewed build", "Deploy a different build", 1)
	conflict, conflictStatus := postAgentInteraction(t, restarted, restartedAgentToken, "create-persisted-key", changed)
	if conflictStatus != http.StatusConflict {
		t.Fatalf("changed create status = %d, want 409: %#v", conflictStatus, conflict)
	}
}

func TestCancelInteractionIdempotencyReturnsOriginalResultAfterRestart(t *testing.T) {
	persistDir := t.TempDir()
	first, sessionToken, firstSessions := newInteractionAPIServer(t, persistDir)
	agentToken := issueAgentToken(t, first, sessionToken)
	created := createInteractionViaAPI(t, first, agentToken, "cancel-idempotency", "permission", `{"type":"action"}`)
	requestID := created["requestId"].(string)
	body := `{"expectedRowVersion":1,"reason":"adapter session ended"}`
	firstResult, firstStatus := postAgentCancel(t, first, agentToken, requestID, "cancel-persisted-key", body)
	if firstStatus != http.StatusOK {
		t.Fatalf("first cancel status = %d, want 200: %#v", firstStatus, firstResult)
	}
	if err := first.controlPlane.Close(); err != nil {
		t.Fatalf("close first control plane: %v", err)
	}
	firstSessions.Stop()

	restarted, restartedSessionToken, restartedSessions := newInteractionAPIServer(t, persistDir)
	defer restartedSessions.Stop()
	restartedAgentToken := issueAgentToken(t, restarted, restartedSessionToken)
	replayed, replayStatus := postAgentCancel(t, restarted, restartedAgentToken, requestID, "cancel-persisted-key", body)
	if replayStatus != http.StatusOK {
		t.Fatalf("replayed cancel status = %d, want 200: %#v", replayStatus, replayed)
	}
	if !reflect.DeepEqual(replayed, firstResult) {
		t.Fatalf("replayed cancel = %#v, want original %#v", replayed, firstResult)
	}

	changed := `{"expectedRowVersion":1,"reason":"different cancellation reason"}`
	conflict, conflictStatus := postAgentCancel(t, restarted, restartedAgentToken, requestID, "cancel-persisted-key", changed)
	if conflictStatus != http.StatusConflict {
		t.Fatalf("changed cancel status = %d, want 409: %#v", conflictStatus, conflict)
	}
}

func postAgentInteraction(t *testing.T, srv *Server, agentToken, idempotencyKey, body string) (map[string]interface{}, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/interactions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode interaction response: %v", err)
	}
	return result, rec.Code
}

func postAgentCancel(t *testing.T, srv *Server, agentToken, requestID, idempotencyKey, body string) (map[string]interface{}, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/v1/interactions/"+requestID+"/cancel", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+agentToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	var result map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode cancel response: %v", err)
	}
	return result, rec.Code
}

func TestMobileInteractionRoutesRequireBrowserSession(t *testing.T) {
	srv, _, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	req := httptest.NewRequest(http.MethodGet, "/api/mobile/interactions", nil)
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET mobile interactions status = %d, want 401", rec.Code)
	}
}

func TestStructuredInteractionAppearsAndResolvesInExistingApprovalsInbox(t *testing.T) {
	srv, sessionToken, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	agentToken := issueAgentToken(t, srv, sessionToken)
	created := createInteractionViaAPI(t, srv, agentToken, "legacy-inbox", "permission", `{"type":"action"}`)
	requestID := created["requestId"].(string)

	snapshotReq := httptest.NewRequest(http.MethodGet, "/api/project-control", nil)
	snapshotReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	snapshotRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(snapshotRec, snapshotReq)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("GET project-control status = %d: %s", snapshotRec.Code, snapshotRec.Body.String())
	}
	snapshot := decodeProjectControlSnapshot(t, snapshotRec)
	foundPending := false
	for _, checkpoint := range snapshot.Checkpoints {
		if checkpoint.ID == requestID {
			foundPending = checkpoint.Status == "pending" && checkpoint.RowVersion == 1
		}
	}
	if !foundPending {
		t.Fatalf("structured interaction %q not projected into approvals: %#v", requestID, snapshot.Checkpoints)
	}

	decisionReq := httptest.NewRequest(http.MethodPost, "/api/project-control/checkpoints/"+requestID+"/decision", strings.NewReader(`{"action":"approve"}`))
	decisionReq.Header.Set("Content-Type", "application/json")
	decisionReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionToken})
	decisionRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(decisionRec, decisionReq)
	if decisionRec.Code != http.StatusOK {
		t.Fatalf("POST legacy decision status = %d: %s", decisionRec.Code, decisionRec.Body.String())
	}
	resolved := decodeProjectControlSnapshot(t, decisionRec)
	foundApproved := false
	for _, checkpoint := range resolved.Checkpoints {
		if checkpoint.ID == requestID {
			foundApproved = checkpoint.Status == "approved" && checkpoint.RowVersion == 2
		}
	}
	if !foundApproved {
		t.Fatalf("resolved structured interaction not reflected in approvals: %#v", resolved.Checkpoints)
	}
}

func TestServerShutdownClosesInteractionRepository(t *testing.T) {
	srv, _, sessions := newInteractionAPIServer(t, t.TempDir())
	defer sessions.Stop()
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	_, err := srv.controlPlane.ListPendingInteractions(context.Background(), "ian", 10)
	if err == nil {
		t.Fatal("control-plane query after Shutdown succeeded, want closed repository error")
	}
}
