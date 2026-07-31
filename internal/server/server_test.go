package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/filebrowser"
	"github.com/ianf339/roambench/internal/terminal"
)

type stubAuthProvider struct {
	err error
}

func (s stubAuthProvider) Authenticate(username, password string) error {
	return s.err
}

func TestLegacyAuthStatusRouteAcceptsPost(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/rpc/auth_status", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /rpc/auth_status status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}

	var resp struct {
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if !resp.Authenticated {
		t.Fatal("Authenticated = false, want true")
	}
	if resp.Username != "ian" {
		t.Fatalf("Username = %q, want %q", resp.Username, "ian")
	}
}

func TestLegacyAuthStatusRouteWithoutCookieReturnsUnauthenticated(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	srv := NewServer(cfg, nil, sessions, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/rpc/auth_status", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /rpc/auth_status status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if resp.Authenticated {
		t.Fatal("Authenticated = true, want false")
	}
}

func TestSecureHeadersAllowViewerCDNForOptionalPreviewLibraries(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	handler := secureHeaders(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") {
		t.Fatalf("Content-Security-Policy = %q, want inline style allowance for terminal renderer", csp)
	}
	if !strings.Contains(csp, "script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://cdn.sheetjs.com") {
		t.Fatalf("Content-Security-Policy = %q, want preview CDNs allowed for optional preview libraries", csp)
	}
	if !strings.Contains(csp, "frame-src 'self' blob: data: chrome-extension: edge-extension: moz-extension:") {
		t.Fatalf("Content-Security-Policy = %q, want PDF viewer frame sources allowed", csp)
	}
	if !strings.Contains(csp, "child-src 'self' blob: data: chrome-extension: edge-extension: moz-extension:") {
		t.Fatalf("Content-Security-Policy = %q, want PDF viewer child sources allowed", csp)
	}
}

func TestSecureHeadersDenyFramingByDefault(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	handler := secureHeaders(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want %q", got, "DENY")
	}
}

func TestTerminalResizeSharedDecisionUsesAttachmentCountAndForce(t *testing.T) {
	srv := &Server{terminalAttachCounts: make(map[string]int)}

	srv.registerTerminalAttachment("ian", "lt-one")
	if !srv.shouldResizeSharedTerminal("ian", "lt-one", false) {
		t.Fatal("single attachment automatic resize should resize the shared tmux window")
	}

	srv.registerTerminalAttachment("ian", "lt-one")
	if srv.shouldResizeSharedTerminal("ian", "lt-one", false) {
		t.Fatal("multiple attachments automatic resize should not resize the shared tmux window")
	}
	if !srv.shouldResizeSharedTerminal("ian", "lt-one", true) {
		t.Fatal("forced resize should resize the shared tmux window with multiple attachments")
	}

	srv.unregisterTerminalAttachment("ian", "lt-one")
	if !srv.shouldResizeSharedTerminal("ian", "lt-one", false) {
		t.Fatal("automatic resize should resize shared tmux window after count returns to one")
	}

	srv.unregisterTerminalAttachment("ian", "lt-one")
	if got := srv.terminalAttachmentCount("ian", "lt-one"); got != 0 {
		t.Fatalf("terminalAttachmentCount after unregister = %d, want 0", got)
	}
}

func TestDownloadInlinePDFUsesExtensionCompatibleFramePolicy(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	tempDir, err := os.MkdirTemp(currentUser.HomeDir, "roambench-download-test-")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pdfPath := filepath.Join(tempDir, "sample.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\n"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	fb := filebrowser.New()
	handler := secureHeaders(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fb.Download(currentUser.Username, r.URL.Query().Get("path"), w, r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/files/download?path="+url.QueryEscape(pdfPath)+"&inline=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Frame-Options"); got != "" {
		t.Fatalf("X-Frame-Options = %q, want empty for extension-backed PDF viewers", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self' chrome-extension: edge-extension: moz-extension:" {
		t.Fatalf("Content-Security-Policy = %q, want extension-compatible frame ancestors", got)
	}
}

func TestDownloadInlineSymlinkToHTMLDeniesFrame(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	tempDir, err := os.MkdirTemp(currentUser.HomeDir, "roambench-download-test-")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	htmlPath := filepath.Join(tempDir, "sample.html")
	if err := os.WriteFile(htmlPath, []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	symlinkPath := filepath.Join(tempDir, "safe.pdf")
	if err := os.Symlink(htmlPath, symlinkPath); err != nil {
		t.Fatalf("Symlink error: %v", err)
	}

	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	fb := filebrowser.New()
	handler := secureHeaders(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fb.Download(currentUser.Username, r.URL.Query().Get("path"), w, r)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/files/download?path="+url.QueryEscape(symlinkPath)+"&inline=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want %q", got, "DENY")
	}
}

func TestHandleFilesUploadStoresMultipartFile(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	tempDir, err := os.MkdirTemp(currentUser.HomeDir, "roambench-upload-test-")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	uploadDir := filepath.Join(tempDir, "issues")
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}

	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = currentUser.Username

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession(currentUser.Username)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, filebrowser.New())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "screenshot.png")
	if err != nil {
		t.Fatalf("CreateFormFile error: %v", err)
	}
	if _, err := part.Write([]byte("upload-body")); err != nil {
		t.Fatalf("part.Write error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/files/upload?path="+url.QueryEscape(uploadDir), &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/files/upload status = %d, want %d", rec.Code, http.StatusOK)
	}

	data, err := os.ReadFile(filepath.Join(uploadDir, "screenshot.png"))
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(data) != "upload-body" {
		t.Fatalf("uploaded file = %q, want %q", string(data), "upload-body")
	}
}

func TestStaticFaviconIsServed(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	srv := NewServer(cfg, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /favicon.svg status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/svg+xml") {
		t.Fatalf("Content-Type = %q, want image/svg+xml", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<svg") {
		t.Fatalf("body = %q, want svg document", body)
	}
}

func TestStaticTerminalComposerAssetIsServed(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true

	srv := NewServer(cfg, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/js/terminal-composer.js", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /js/terminal-composer.js status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, want javascript", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "RoamBenchTerminalComposer") {
		t.Fatalf("body missing terminal composer export: %q", body)
	}
}

func TestBasePathRedirectsRootAndServesPrefixedAssets(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Server.BasePath = "/home/ian"

	srv := NewServer(cfg, nil, nil, nil, nil)
	handler := srv.basePathHandler(srv.mux)

	redirectReq := httptest.NewRequest(http.MethodGet, "/", nil)
	redirectRec := httptest.NewRecorder()
	handler.ServeHTTP(redirectRec, redirectReq)

	if redirectRec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("GET / status = %d, want %d", redirectRec.Code, http.StatusTemporaryRedirect)
	}
	if got := redirectRec.Header().Get("Location"); got != "/home/ian" {
		t.Fatalf("Location = %q, want %q", got, "/home/ian")
	}

	indexReq := httptest.NewRequest(http.MethodGet, "/home/ian", nil)
	indexRec := httptest.NewRecorder()
	handler.ServeHTTP(indexRec, indexReq)

	if indexRec.Code != http.StatusOK {
		t.Fatalf("GET /home/ian status = %d, want %d", indexRec.Code, http.StatusOK)
	}
	body := indexRec.Body.String()
	if !strings.Contains(body, `window.__BASE_PATH__ = "/home/ian"`) {
		t.Fatalf("index missing base-path bootstrap: %q", body)
	}
	if !strings.Contains(body, `href="/home/ian/favicon.svg"`) {
		t.Fatalf("index missing prefixed favicon href: %q", body)
	}
	if !strings.Contains(body, `src="/home/ian/js/app.js"`) {
		t.Fatalf("index missing prefixed app.js src: %q", body)
	}
	if !strings.Contains(body, `src="/home/ian/js/terminal-composer.js"`) {
		t.Fatalf("index missing prefixed terminal composer src: %q", body)
	}

	faviconReq := httptest.NewRequest(http.MethodGet, "/home/ian/favicon.svg", nil)
	faviconRec := httptest.NewRecorder()
	handler.ServeHTTP(faviconRec, faviconReq)

	if faviconRec.Code != http.StatusOK {
		t.Fatalf("GET /home/ian/favicon.svg status = %d, want %d", faviconRec.Code, http.StatusOK)
	}
}

func TestLoginCookieUsesBasePath(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Server.BasePath = "/home/ian"
	cfg.Auth.SingleUser = "ian"

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	srv := NewServer(cfg, stubAuthProvider{}, sessions, nil, nil)
	handler := srv.basePathHandler(srv.mux)

	req := httptest.NewRequest(http.MethodPost, "/home/ian/api/auth/login", strings.NewReader(`{"username":"ian","password":"pw"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /home/ian/api/auth/login status = %d, want %d", rec.Code, http.StatusOK)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("login response did not set any cookies")
	}
	if got := cookies[0].Path; got != "/home/ian" {
		t.Fatalf("cookie Path = %q, want %q", got, "/home/ian")
	}
}

func TestParseProcMemoryValueBytes(t *testing.T) {
	content := "Name:\troambench\nVmRSS:\t   12345 kB\nThreads:\t7\n"

	got, err := parseProcMemoryValueBytes(content, "VmRSS:")
	if err != nil {
		t.Fatalf("parseProcMemoryValueBytes error = %v", err)
	}
	if want := uint64(12345 * 1024); got != want {
		t.Fatalf("parseProcMemoryValueBytes = %d, want %d", got, want)
	}
}

func TestReadMemoryStatusParsesUsedMemory(t *testing.T) {
	meminfo := "MemTotal:       16384 kB\nMemAvailable:    4096 kB\n"

	total, err := parseProcMemoryValueBytes(meminfo, "MemTotal:")
	if err != nil {
		t.Fatalf("MemTotal parse error = %v", err)
	}
	available, err := parseProcMemoryValueBytes(meminfo, "MemAvailable:")
	if err != nil {
		t.Fatalf("MemAvailable parse error = %v", err)
	}

	used := total - available
	if want := uint64((16384 - 4096) * 1024); used != want {
		t.Fatalf("used memory = %d, want %d", used, want)
	}
}

func TestMemoryStatusRouteReturnsStats(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/system/memory", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/system/memory status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}

	var resp struct {
		ProcessRSSBytes  uint64 `json:"processRSSBytes"`
		SystemUsedBytes  uint64 `json:"systemUsedBytes"`
		TotalMemoryBytes uint64 `json:"totalMemoryBytes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if resp.ProcessRSSBytes == 0 {
		t.Fatal("ProcessRSSBytes = 0, want non-zero")
	}
	if resp.TotalMemoryBytes == 0 {
		t.Fatal("TotalMemoryBytes = 0, want non-zero")
	}
	if resp.SystemUsedBytes == 0 {
		t.Fatal("SystemUsedBytes = 0, want non-zero")
	}
}

func TestFilesPreviewRouteReturnsScaledImage(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	tempDir, err := os.MkdirTemp(currentUser.HomeDir, ".roambench-preview-test-")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	imagePath := filepath.Join(tempDir, "preview-source.png")
	if err := writeTestPNG(imagePath, 480, 320); err != nil {
		t.Fatalf("writeTestPNG error: %v", err)
	}

	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = currentUser.Username

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession(currentUser.Username)
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	srv := NewServer(cfg, nil, sessions, nil, filebrowser.New())
	req := httptest.NewRequest(http.MethodGet, "/api/files/preview?path="+url.QueryEscape(imagePath)+"&size=64", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/files/preview status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}

	cfgImage, err := png.DecodeConfig(strings.NewReader(rec.Body.String()))
	if err != nil {
		t.Fatalf("png.DecodeConfig error: %v", err)
	}
	if cfgImage.Width > 64 || cfgImage.Height > 64 {
		t.Fatalf("preview size = %dx%d, want max 64px on each edge", cfgImage.Width, cfgImage.Height)
	}
}

func TestHandleTerminalWebSocketClosesWithPolicyViolationWhenAttachUnavailable(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	termMgr := terminal.NewManager(&cfg.Terminal)
	defer termMgr.Stop()

	srv := NewServer(cfg, nil, sessions, termMgr, filebrowser.New())
	_, err = srv.projectControl.withStateLocked("ian", func(state *projectControlState) error {
		state.Tasks = []projectControlTask{{
			ID:               "task-attach-policy",
			ProjectID:        "project-attach",
			WorkstreamID:     "workstream-attach",
			State:            "running",
			AcceptanceStatus: "not_ready",
			RuntimeID:        projectControlRuntimeID,
			SelectedSkill:    projectControlDefaultSkillID,
			RunbookID:        projectControlDefaultRunbookID,
			CurrentPhase:     "review",
			RunbookState:     "in_progress",
		}}
		state.PhaseAttempts = []projectControlPhaseAttempt{{
			ID:        "attempt-review",
			TaskID:    "task-attach-policy",
			RunbookID: projectControlDefaultRunbookID,
			PhaseID:   "review",
			SessionID: projectControlSessionIDForTerminal("term-review"),
			Status:    "running",
		}}
		return nil
	})
	if err != nil {
		t.Fatalf("withStateLocked error: %v", err)
	}

	server := httptest.NewServer(srv.mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/terminals/ws/term-review"
	header := http.Header{}
	header.Set("Origin", server.URL)
	header.Set("Cookie", auth.CookieName+"="+token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("ReadMessage error = nil, want close error")
	} else if closeErr, ok := err.(*websocket.CloseError); !ok {
		t.Fatalf("ReadMessage error = %T, want *websocket.CloseError", err)
	} else {
		if closeErr.Code != websocket.ClosePolicyViolation {
			t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.ClosePolicyViolation)
		}
		if closeErr.Text != terminalCloseReasonAttachUnavailable {
			t.Fatalf("close reason = %q, want %q", closeErr.Text, terminalCloseReasonAttachUnavailable)
		}
	}
}

func TestHandleTerminalWebSocketClosesWithSessionUnavailableReason(t *testing.T) {
	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = "ian"
	cfg.Terminal.PersistDir = t.TempDir()

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	token, err := sessions.CreateSession("ian")
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	termMgr := terminal.NewManager(&cfg.Terminal)
	defer termMgr.Stop()

	srv := NewServer(cfg, nil, sessions, termMgr, filebrowser.New())
	server := httptest.NewServer(srv.mux)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/terminals/ws/missing-session"
	header := http.Header{}
	header.Set("Origin", server.URL)
	header.Set("Cookie", auth.CookieName+"="+token)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("ReadMessage error = nil, want close error")
	} else if closeErr, ok := err.(*websocket.CloseError); !ok {
		t.Fatalf("ReadMessage error = %T, want *websocket.CloseError", err)
	} else {
		if closeErr.Code != websocket.CloseNormalClosure {
			t.Fatalf("close code = %d, want %d", closeErr.Code, websocket.CloseNormalClosure)
		}
		if closeErr.Text != terminalCloseReasonSessionUnavailable {
			t.Fatalf("close reason = %q, want %q", closeErr.Text, terminalCloseReasonSessionUnavailable)
		}
	}
}

func TestHandleTerminalHistoryReturnsOwnedTmuxSnapshot(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current error: %v", err)
	}

	cfg := config.Defaults()
	cfg.Server.AllowAllIPs = true
	cfg.Auth.SingleUser = currentUser.Username
	cfg.Terminal.PersistDir = t.TempDir()
	cfg.Terminal.Scrollback = 100

	sessions, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		t.Fatalf("NewSessionManager error: %v", err)
	}
	defer sessions.Stop()

	ownerToken, err := sessions.CreateSession(currentUser.Username)
	if err != nil {
		t.Fatalf("CreateSession token error: %v", err)
	}
	otherToken, err := sessions.CreateSession(currentUser.Username + "-other")
	if err != nil {
		t.Fatalf("CreateSession other token error: %v", err)
	}

	termMgr := terminal.NewManager(&cfg.Terminal)
	defer termMgr.Stop()
	session, err := termMgr.CreateSession(currentUser.Username)
	if err != nil {
		t.Fatalf("CreateSession terminal error: %v", err)
	}
	defer termMgr.KillSessionForUser(currentUser.Username, session.ID)

	command := "i=1; while [ $i -le 80 ]; do printf 'history-%03d\\n' $i; i=$((i+1)); done; printf '\\033[31mhistory-marker\\033[0m\\n'"
	if err := exec.Command("tmux", "send-keys", "-t", session.ID, command, "Enter").Run(); err != nil {
		t.Fatalf("tmux send-keys error: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		snapshot, captureErr := termMgr.CaptureHistoryForUser(currentUser.Username, session.ID)
		if captureErr == nil && bytes.Contains(snapshot.Data, []byte("history-marker")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("history marker did not appear before deadline; last error: %v", captureErr)
		}
		time.Sleep(20 * time.Millisecond)
	}

	srv := NewServer(cfg, nil, sessions, termMgr, filebrowser.New())
	req := httptest.NewRequest(http.MethodGet, "/api/terminals/"+session.ID+"/history", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: ownerToken})
	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET terminal history status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if rec.Header().Get("X-Terminal-Columns") == "" || rec.Header().Get("X-Terminal-Rows") == "" {
		t.Fatalf("missing terminal dimensions: columns=%q rows=%q", rec.Header().Get("X-Terminal-Columns"), rec.Header().Get("X-Terminal-Rows"))
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("history-marker")) {
		t.Fatalf("history response does not contain marker: %q", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("history-001")) {
		t.Fatalf("history response does not include output older than the visible pane")
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("\x1b[31m")) {
		t.Fatalf("history response does not preserve ANSI color attributes")
	}

	forbiddenReq := httptest.NewRequest(http.MethodGet, "/api/terminals/"+session.ID+"/history", nil)
	forbiddenReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: otherToken})
	forbiddenRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(forbiddenRec, forbiddenReq)
	if forbiddenRec.Code != http.StatusNotFound {
		t.Fatalf("GET terminal history as other user status = %d, want %d", forbiddenRec.Code, http.StatusNotFound)
	}

	methodReq := httptest.NewRequest(http.MethodPost, "/api/terminals/"+session.ID+"/history", nil)
	methodReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: ownerToken})
	methodRec := httptest.NewRecorder()
	srv.mux.ServeHTTP(methodRec, methodReq)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST terminal history status = %d, want %d", methodRec.Code, http.StatusMethodNotAllowed)
	}
}

func writeTestPNG(path string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x * 255) / max(width-1, 1)),
				G: uint8((y * 255) / max(height-1, 1)),
				B: 190,
				A: 255,
			})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
