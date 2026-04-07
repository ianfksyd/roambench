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
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/liteterm-web/internal/auth"
	"github.com/user/liteterm-web/internal/config"
	"github.com/user/liteterm-web/internal/filebrowser"
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

func TestSecureHeadersAllowsInlineStylesForTerminalRenderer(t *testing.T) {
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
	if strings.Contains(csp, "cdn.jsdelivr.net") {
		t.Fatalf("Content-Security-Policy = %q, want terminal assets served locally", csp)
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
	content := "Name:\tliteterm\nVmRSS:\t   12345 kB\nThreads:\t7\n"

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

	tempDir, err := os.MkdirTemp(currentUser.HomeDir, ".liteterm-preview-test-")
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
