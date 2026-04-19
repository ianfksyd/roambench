package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/filebrowser"
	"github.com/ianf339/roambench/internal/terminal"
	"github.com/ianf339/roambench/web"
)

type notificationHub struct {
	mu      sync.Mutex
	clients map[chan terminal.OSCNotification]struct{}
}

func newNotificationHub() *notificationHub {
	return &notificationHub{clients: make(map[chan terminal.OSCNotification]struct{})}
}

func (h *notificationHub) subscribe() chan terminal.OSCNotification {
	ch := make(chan terminal.OSCNotification, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *notificationHub) unsubscribe(ch chan terminal.OSCNotification) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

func (h *notificationHub) broadcast(n terminal.OSCNotification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- n:
		default:
		}
	}
}

type Server struct {
	cfg            *config.Config
	authProv       auth.AuthProvider
	sessions       *auth.SessionManager
	rateLimiter    *auth.RateLimiter
	terminals      *terminal.Manager
	workspaceState *workspaceStateStore
	fileBrowser    *filebrowser.FileBrowser
	projectControl *projectControlStore
	notifHub       *notificationHub
	mux            *http.ServeMux
	upgrader       websocket.Upgrader
	httpServer     *http.Server
}

func NewServer(
	cfg *config.Config,
	authProv auth.AuthProvider,
	sessions *auth.SessionManager,
	terminals *terminal.Manager,
	fb *filebrowser.FileBrowser,
) *Server {
	s := &Server{
		cfg:            cfg,
		authProv:       authProv,
		sessions:       sessions,
		rateLimiter:    auth.NewRateLimiter(cfg.Auth.MaxLoginAttempts, cfg.Auth.GetLockoutDuration()),
		terminals:      terminals,
		workspaceState: newWorkspaceStateStore(cfg.Terminal.GetPersistDir()),
		fileBrowser:    fb,
		projectControl: newProjectControlStore(cfg.Terminal.GetPersistDir()),
		notifHub:       newNotificationHub(),
		mux:            http.NewServeMux(),
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			if origin == "" {
				return false
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			return sameOriginHost(u.Host, r.Host, u.Scheme, isSecureRequest(r, cfg.Server.TrustProxy))
		},
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Auth
	s.mux.HandleFunc("/api/auth/login", s.handleLogin)
	s.mux.HandleFunc("/api/auth/logout", s.handleLogout)
	s.mux.HandleFunc("/api/auth/status", s.handleAuthStatus)
	s.mux.HandleFunc("/rpc/auth_status", s.handleLegacyAuthStatus)
	s.mux.HandleFunc("/api/ui-config", s.handleUIConfig)
	s.mux.HandleFunc("/api/workspace-state", authRequired(s.sessions, s.handleWorkspaceState))
	s.mux.HandleFunc("/api/system/memory", authRequired(s.sessions, s.handleMemoryStatus))
	s.mux.HandleFunc("/api/project-control/checkpoints/", authRequired(s.sessions, s.handleProjectControlCheckpointDecision))
	s.mux.HandleFunc("/api/project-control/events", authRequired(s.sessions, s.handleProjectControlEvents))
	s.mux.HandleFunc("/api/project-control/tasks/", authRequired(s.sessions, s.handleProjectControlTaskReplay))
	s.mux.HandleFunc("/api/project-control/projects", authRequired(s.sessions, s.handleProjectControlProjects))
	s.mux.HandleFunc("/api/project-control/workstreams/", authRequired(s.sessions, s.handleProjectControlWorkstreamUpdate))
	s.mux.HandleFunc("/api/project-control/workstreams", authRequired(s.sessions, s.handleProjectControlWorkstreams))
	s.mux.HandleFunc("/api/project-control/tasks", authRequired(s.sessions, s.handleProjectControlTasks))
	s.mux.HandleFunc("/api/project-control/agent-token", authRequired(s.sessions, s.handleAgentToken))
	s.mux.HandleFunc("/api/project-control", authRequired(s.sessions, s.handleProjectControlSnapshot))

	// Agent API (token auth)
	s.mux.HandleFunc("/api/agent/v1/task", s.handleAgentGetTask)
	s.mux.HandleFunc("/api/agent/v1/artifact", s.handleAgentSubmitArtifact)
	s.mux.HandleFunc("/api/agent/v1/checkpoint", s.handleAgentRequestCheckpoint)

	// Terminals (auth required)
	s.mux.HandleFunc("/api/terminals/ws/", authRequired(s.sessions, s.handleTerminalWebSocket))
	s.mux.HandleFunc("/api/notifications/ws", authRequired(s.sessions, s.handleNotificationWebSocket))
	s.mux.HandleFunc("/api/terminals/", authRequired(s.sessions, s.handleTerminal))
	s.mux.HandleFunc("/api/terminals", authRequired(s.sessions, s.handleTerminals))

	// File browser (auth required)
	s.mux.HandleFunc("/api/files/mkdir", authRequired(s.sessions, s.handleFilesMkdir))
	s.mux.HandleFunc("/api/files/copy", authRequired(s.sessions, s.handleFilesCopy))
	s.mux.HandleFunc("/api/files/rename", authRequired(s.sessions, s.handleFilesRename))
	s.mux.HandleFunc("/api/files/read", authRequired(s.sessions, s.handleFilesRead))
	s.mux.HandleFunc("/api/files/save", authRequired(s.sessions, s.handleFilesSave))
	s.mux.HandleFunc("/api/files/upload", authRequired(s.sessions, s.handleFilesUpload))
	s.mux.HandleFunc("/api/files/download", authRequired(s.sessions, s.handleFilesDownload))
	s.mux.HandleFunc("/api/files/preview", authRequired(s.sessions, s.handleFilesPreview))
	s.mux.HandleFunc("/api/files", authRequired(s.sessions, s.handleFiles))

	// Static files — no-store forces the browser to always fetch fresh content
	staticFS := http.FileServer(http.FS(web.StaticFiles))
	s.mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			s.serveIndex(w)
			return
		}
		staticFS.ServeHTTP(w, r)
	}))
}

func (s *Server) basePath() string {
	return s.cfg.Server.GetBasePath()
}

func (s *Server) browserBasePath() string {
	basePath := s.basePath()
	if basePath == "/" {
		return ""
	}
	return basePath
}

func (s *Server) cookiePath() string {
	return s.basePath()
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	content, err := web.StaticFiles.ReadFile("index.html")
	if err != nil {
		http.Error(w, "index unavailable", http.StatusInternalServerError)
		return
	}

	rendered := strings.ReplaceAll(string(content), "__ROAMBENCH_BASE_PATH__", s.browserBasePath())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, rendered)
}

func (s *Server) basePathHandler(next http.Handler) http.Handler {
	basePath := s.basePath()
	if basePath == "/" {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			target := basePath
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusTemporaryRedirect)
			return
		case r.URL.Path == basePath:
			clone := r.Clone(r.Context())
			clone.URL.Path = "/"
			clone.URL.RawPath = ""
			next.ServeHTTP(w, clone)
			return
		case strings.HasPrefix(r.URL.Path, basePath+"/"):
			clone := r.Clone(r.Context())
			clone.URL.Path = strings.TrimPrefix(r.URL.Path, basePath)
			if clone.URL.Path == "" {
				clone.URL.Path = "/"
			}
			if r.URL.RawPath != "" {
				clone.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, basePath)
				if clone.URL.RawPath == "" {
					clone.URL.RawPath = "/"
				}
			} else {
				clone.URL.RawPath = ""
			}
			next.ServeHTTP(w, clone)
			return
		default:
			http.NotFound(w, r)
			return
		}
	})
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	handler := recoverer(requestLogger(ipAllowlist(s.cfg, secureHeaders(s.cfg, s.basePathHandler(s.mux)))))
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	if s.cfg.Server.TLSCert != "" && s.cfg.Server.TLSKey != "" {
		log.Printf("Listening on https://%s%s", addr, s.basePath())
		return s.httpServer.ListenAndServeTLS(s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
	}

	log.Printf("Listening on http://%s%s", addr, s.basePath())
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// --- Auth handlers ---

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if err := s.rateLimiter.Check(ip); err != nil {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if err := s.authProv.Authenticate(req.Username, req.Password); err != nil {
		s.rateLimiter.Record(ip)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	sessionUsername := req.Username
	if s.cfg.Auth.SingleUser != "" {
		sessionUsername = s.cfg.Auth.SingleUser
	}

	cookie, err := s.sessions.CreateSession(sessionUsername)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    cookie,
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   isSecureRequest(r, s.cfg.Server.TrustProxy),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.cfg.Auth.GetSessionTimeout().Seconds()),
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"username":      sessionUsername,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie(auth.CookieName)
	if err == nil {
		s.sessions.InvalidateSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     s.cookiePath(),
		HttpOnly: true,
		Secure:   isSecureRequest(r, s.cfg.Server.TrustProxy),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeAuthStatus(w, r)
}

func (s *Server) handleLegacyAuthStatus(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
		s.writeAuthStatus(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) writeAuthStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cookie := authCookie(r)
	if cookie == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
		return
	}
	username, err := s.sessions.ValidateSession(cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"authenticated": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"username":      username,
	})
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	title := strings.TrimSpace(s.cfg.UI.Title)
	if title == "" {
		title = "RoamBench"
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"title": title,
		"motd":  strings.TrimSpace(s.cfg.UI.MOTD),
		"terminal": map[string]interface{}{
			"scrollback": s.cfg.Terminal.Scrollback,
			"tmux":       s.terminals != nil && s.terminals.HasTmux(),
		},
	})
}

// --- Terminal handlers ---

func (s *Server) handleTerminals(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	switch r.Method {
	case http.MethodGet:
		sessions := s.terminals.ListSessions(username)
		if sessions == nil {
			sessions = []terminal.SessionInfo{}
		}
		writeJSON(w, http.StatusOK, sessions)
	case http.MethodPost:
		session, err := s.terminals.CreateSession(username)
		if err != nil {
			log.Printf("create session error: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to create terminal session"})
			return
		}
		writeJSON(w, http.StatusCreated, session)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	// Extract session ID from path: /api/terminals/{id}
	id := strings.TrimPrefix(r.URL.Path, "/api/terminals/")
	if strings.HasPrefix(id, "ws/") {
		return // handled by websocket handler
	}
	username := GetUsername(r)

	if strings.HasSuffix(id, "/rename") {
		sessionID := strings.TrimSuffix(id, "/rename")
		if sessionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing session ID"})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		if err := s.terminals.RenameSession(username, sessionID, req.Name); err != nil {
			status := http.StatusBadRequest
			if err == terminal.ErrNotFound || err == terminal.ErrForbidden {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.terminals.KillSessionForUser(username, id); err != nil {
			status := http.StatusBadRequest
			if err == terminal.ErrNotFound || err == terminal.ErrForbidden {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "killed"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- File browser handlers ---

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	switch r.Method {
	case http.MethodGet:
		path := r.URL.Query().Get("path")
		if path == "" {
			path = "~"
		}
		files, err := s.fileBrowser.ListDir(username, path)
		if err != nil {
			log.Printf("list dir error: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to list directory"})
			return
		}
		writeJSON(w, http.StatusOK, files)
	case http.MethodDelete:
		path := r.URL.Query().Get("path")
		if path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}
		if err := s.fileBrowser.Delete(username, path); err != nil {
			log.Printf("delete error: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to delete file"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	username := GetUsername(r)
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := s.fileBrowser.Mkdir(username, req.Path); err != nil {
		log.Printf("mkdir error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to create directory"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

func (s *Server) handleFilesRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	username := GetUsername(r)
	var req struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := s.fileBrowser.Rename(username, req.OldPath, req.NewPath); err != nil {
		log.Printf("rename error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to rename"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "renamed"})
}

func (s *Server) handleFilesCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	username := GetUsername(r)
	var req struct {
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if err := s.fileBrowser.Copy(username, req.OldPath, req.NewPath); err != nil {
		log.Printf("copy error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to copy"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "copied"})
}

func (s *Server) handleFilesRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	path := r.URL.Query().Get("path")
	content, err := s.fileBrowser.ReadText(username, path)
	if err != nil {
		log.Printf("read file error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read file"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"path":    path,
		"content": content,
	})
}

func (s *Server) handleFilesSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	username := GetUsername(r)
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if req.Path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
		return
	}
	if err := s.fileBrowser.WriteText(username, req.Path, req.Content); err != nil {
		log.Printf("save file error: %v", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to save file"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

func (s *Server) handleFilesUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	targetPath := r.URL.Query().Get("path")

	// Hard limit: reject requests larger than 32MB
	const maxUploadSize = 32 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	filename, data, err := readUploadedFile(r, maxUploadSize)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file too large (max 32MB)"})
			return
		}
		if errors.Is(err, errNoUploadFile) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no file provided"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := s.fileBrowser.Upload(username, targetPath, filename, data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded"})
}

var (
	errNoUploadFile   = errors.New("no file provided")
	errUploadTooLarge = errors.New("file too large")
)

func readUploadedFile(r *http.Request, maxUploadSize int64) (string, []byte, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return "", nil, errors.New("invalid multipart request")
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, errors.New("read error")
		}

		filename := strings.TrimSpace(part.FileName())
		if part.FormName() != "file" || filename == "" {
			part.Close()
			continue
		}

		data, err := io.ReadAll(io.LimitReader(part, maxUploadSize+1))
		part.Close()
		if err != nil {
			return "", nil, errors.New("read error")
		}
		if int64(len(data)) > maxUploadSize {
			return "", nil, errUploadTooLarge
		}
		return filename, data, nil
	}

	return "", nil, errNoUploadFile
}

func (s *Server) handleFilesDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	path := r.URL.Query().Get("path")
	s.fileBrowser.Download(username, path, w, r)
}

func (s *Server) handleFilesPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	path := r.URL.Query().Get("path")

	size := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("size")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err == nil {
			size = parsed
		}
	}

	s.fileBrowser.Preview(username, path, size, w, r)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

type wsControlMessage struct {
	Type  string `json:"type"`
	Rows  uint16 `json:"rows"`
	Cols  uint16 `json:"cols"`
	Lines int    `json:"lines"`
}

type wsServerMessage struct {
	Type           string `json:"type"`
	InMode         bool   `json:"inMode,omitempty"`
	HistorySize    int    `json:"historySize,omitempty"`
	ScrollPosition int    `json:"scrollPosition,omitempty"`
	PaneHeight     int    `json:"paneHeight,omitempty"`
}

const (
	terminalCloseReasonSessionUnavailable = "session unavailable"
	terminalCloseReasonAttachUnavailable  = "terminal attach unavailable"
	terminalCloseReasonAttachFailed       = "terminal attach failed"
	terminalCloseReasonSessionEnded       = "session ended"
)

func channelClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func (s *Server) handleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	username := GetUsername(r)
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/terminals/ws/")
	if sessionID == "" {
		http.Error(w, "missing session ID", http.StatusBadRequest)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	writeClose := func(code int, text string) {
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text),
			time.Now().Add(time.Second),
		)
	}

	if s.projectControl != nil && !s.projectControl.allowsTerminalAttach(username, sessionID) {
		writeClose(websocket.ClosePolicyViolation, terminalCloseReasonAttachUnavailable)
		return
	}

	ptmx, cmd, err := s.terminals.AttachSessionForUser(username, sessionID)
	if err != nil {
		closeCode := websocket.CloseNormalClosure
		closeText := terminalCloseReasonSessionUnavailable
		if errors.Is(err, terminal.ErrForbidden) {
			closeCode = websocket.ClosePolicyViolation
			closeText = terminalCloseReasonAttachUnavailable
		} else if !errors.Is(err, terminal.ErrNotFound) {
			log.Printf("terminal attach error for %s/%s: %v", username, sessionID, err)
			closeCode = websocket.CloseInternalServerErr
			closeText = terminalCloseReasonAttachFailed
		}
		writeClose(closeCode, closeText)
		return
	}

	var once sync.Once
	var wsMu sync.Mutex
	done := make(chan struct{})

	cleanup := func() {
		once.Do(func() {
			close(done)
			ptmx.Close()
			// In tmux mode this is only the attach client; killing it detaches
			// the web bridge without killing the persisted tmux session.
			if cmd != nil && cmd.Process != nil {
				cmd.Process.Kill()
			}
		})
	}
	defer cleanup()

	writeTextMessage := func(payload []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteMessage(websocket.TextMessage, payload)
	}

	writeBinaryMessage := func(payload []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, payload)
	}

	sendScrollState := func() error {
		if !s.terminals.HasTmux() {
			return nil
		}

		scrollState, err := s.terminals.ScrollStateForUser(username, sessionID)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(wsServerMessage{
			Type:           "scroll_state",
			InMode:         scrollState.InMode,
			HistorySize:    scrollState.HistorySize,
			ScrollPosition: scrollState.ScrollPosition,
			PaneHeight:     scrollState.PaneHeight,
		})
		if err != nil {
			return err
		}

		return writeTextMessage(payload)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				wsMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				wsMu.Unlock()
				if err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()

	oscScanner := terminal.NewOSCScanner(sessionID)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.terminals.TouchSessionForUser(username, sessionID)
				passthrough, notifs := oscScanner.Feed(buf[:n])
				for _, notif := range notifs {
					s.notifHub.broadcast(notif)
				}
				if len(passthrough) > 0 {
					writeErr := writeBinaryMessage(passthrough)
					if writeErr != nil {
						cleanup()
						return
					}
				}
			}
			if err != nil {
				if channelClosed(done) {
					return
				}
				if err != io.EOF {
					log.Printf("pty read error: %v", err)
				}
				wsMu.Lock()
				conn.WriteMessage(websocket.TextMessage, []byte("\r\n[session ended]\r\n"))
				conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, terminalCloseReasonSessionEnded),
					time.Now().Add(time.Second))
				wsMu.Unlock()
				cleanup()
				return
			}
		}
	}()

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	if err := sendScrollState(); err != nil && !errors.Is(err, terminal.ErrNotFound) {
		log.Printf("scroll state init error: %v", err)
	}

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("websocket read error: %v", err)
			}
			cleanup()
			return
		}
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		if msgType == websocket.TextMessage {
			var ctrl wsControlMessage
			if err := json.Unmarshal(msg, &ctrl); err == nil {
				if ctrl.Type == "resize" && ctrl.Rows > 0 && ctrl.Cols > 0 {
					s.terminals.ResizeSessionForUser(username, sessionID, ctrl.Rows, ctrl.Cols)
					sendScrollState()
				} else if ctrl.Type == "scroll" && ctrl.Lines != 0 {
					s.terminals.ScrollSessionForUser(username, sessionID, ctrl.Lines)
					sendScrollState()
				} else if ctrl.Type == "scroll_reset" {
					s.terminals.ResetScrollSessionForUser(username, sessionID)
					sendScrollState()
				} else if ctrl.Type == "scroll_state" {
					sendScrollState()
				}
			}
			continue
		}

		if _, err := ptmx.Write(msg); err != nil {
			log.Printf("pty write error: %v", err)
			cleanup()
			return
		}
		s.terminals.TouchSessionForUser(username, sessionID)
	}
}

func sameOriginHost(originHost, requestHost, originScheme string, requestSecure bool) bool {
	originName, originPort := splitHostPort(originHost)
	requestName, requestPort := splitHostPort(requestHost)
	if !strings.EqualFold(originName, requestName) {
		return false
	}
	if originPort == "" {
		originPort = defaultPort(originScheme)
	}
	if requestPort == "" {
		if requestSecure {
			requestPort = "443"
		} else {
			requestPort = "80"
		}
	}
	return originPort == requestPort
}

func splitHostPort(hostport string) (string, string) {
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		return strings.Trim(host, "[]"), port
	}
	return strings.Trim(hostport, "[]"), ""
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "https", "wss":
		return "443"
	default:
		return "80"
	}
}

func (s *Server) handleNotificationWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch := s.notifHub.subscribe()
	defer s.notifHub.unsubscribe(ch)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case notif, ok := <-ch:
			if !ok {
				return
			}
			msg := map[string]interface{}{
				"type":      "notification",
				"code":      notif.Code,
				"title":     notif.Title,
				"subtitle":  notif.Subtitle,
				"body":      notif.Body,
				"sessionId": notif.SessionID,
				"timestamp": notif.Timestamp.Format(time.RFC3339),
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

func (s *Server) agentUsername(r *http.Request) (string, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	username := s.cfg.Auth.SingleUser
	if username == "" {
		return "", false
	}
	if !s.projectControl.validateAgentToken(username, token) {
		return "", false
	}
	return username, true
}

func (s *Server) handleAgentToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username := GetUsername(r)
	token, err := s.projectControl.ensureAgentToken(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) handleAgentGetTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.agentUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
		return
	}
	result, err := s.projectControl.agentGetTask(username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAgentSubmitArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.agentUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
		return
	}
	var req agentArtifactRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	snapshot, err := s.projectControl.agentSubmitArtifact(username, req, s.terminals)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":           true,
		"currentPhase": snapshot.Tasks[0].CurrentPhase,
	})
}

func (s *Server) handleAgentRequestCheckpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	username, ok := s.agentUsername(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid agent token"})
		return
	}
	var req agentCheckpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := s.projectControl.agentRequestCheckpoint(username, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}
