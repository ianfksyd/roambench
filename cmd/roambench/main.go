package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"strings"
	"syscall"
	"time"

	"github.com/ianf339/roambench/internal/auth"
	"github.com/ianf339/roambench/internal/config"
	"github.com/ianf339/roambench/internal/filebrowser"
	"github.com/ianf339/roambench/internal/server"
	"github.com/ianf339/roambench/internal/terminal"
)

const version = "0.2.0"

func main() {
	port := flag.Int("port", 0, "Server port (overrides config)")
	host := flag.String("host", "", "Server host (overrides config)")
	configPath := flag.String("config", "", "Path to config file")
	hashPassword := flag.Bool("password-hash", false, "Generate bcrypt hash from stdin")
	flag.Parse()

	// Utility: hash password and exit
	if *hashPassword {
		fmt.Fprint(os.Stderr, "Enter password: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			password := strings.TrimSpace(scanner.Text())
			hash, err := auth.HashPassword(password)
			if err != nil {
				log.Fatalf("Error hashing password: %v", err)
			}
			fmt.Println(hash)
		}
		return
	}

	// Load config
	var cfg *config.Config
	if *configPath != "" {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			log.Fatalf("Error loading config from %s: %v", *configPath, err)
		}
	} else {
		cfg = config.FindAndLoad()
	}

	// Apply flag overrides
	if *port > 0 {
		cfg.Server.Port = *port
	}
	if *host != "" {
		cfg.Server.Host = *host
	}

	// Check for password hash from env vars.
	if cfg.Auth.PasswordHash == "" {
		if envHash := firstNonEmpty(os.Getenv("ROAMBENCH_PASSWORD_HASH"), os.Getenv("LITETERM_PASSWORD_HASH")); envHash != "" {
			cfg.Auth.PasswordHash = envHash
		}
	}

	// Check for single user from env vars.
	if cfg.Auth.SingleUser == "" {
		if envUser := firstNonEmpty(os.Getenv("ROAMBENCH_USER"), os.Getenv("LITETERM_USER")); envUser != "" {
			cfg.Auth.SingleUser = envUser
		}
	}

	// If still no password hash and using password auth, error out
	if cfg.Auth.Method == "password" && cfg.Auth.PasswordHash == "" {
		fmt.Fprintln(os.Stderr, "Error: No password configured.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Option 1: Generate a hash and set env var:")
		fmt.Fprintln(os.Stderr, "  ./roambench --password-hash")
		fmt.Fprintln(os.Stderr, "  export ROAMBENCH_PASSWORD_HASH='<hash>'")
		fmt.Fprintln(os.Stderr, "  ./roambench")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Option 2: Set password_hash in config file:")
		fmt.Fprintln(os.Stderr, "  cp configs/roambench.example.toml roambench.toml")
		fmt.Fprintln(os.Stderr, "  # edit roambench.toml and set password_hash")
		os.Exit(1)
	}

	if err := validateSecurityConfig(cfg); err != nil {
		log.Fatalf("Security configuration error: %v", err)
	}
	if err := config.Validate(cfg); err != nil {
		log.Fatalf("Config validation error: %v", err)
	}

	// Create auth provider
	authProv, err := auth.NewAuthProvider(&cfg.Auth)
	if err != nil {
		log.Fatalf("Auth setup error: %v", err)
	}

	// Create session manager
	sessionMgr, err := auth.NewSessionManager(&cfg.Auth)
	if err != nil {
		log.Fatalf("Session manager error: %v", err)
	}

	// Create terminal manager
	termMgr := terminal.NewManager(&cfg.Terminal)

	// Create file browser
	fb := filebrowser.New()

	// Create and start server
	srv := server.NewServer(cfg, authProv, sessionMgr, termMgr, fb)

	// Startup banner
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "  RoamBench v%s\n", version)
	fmt.Fprintf(os.Stderr, "  Listening on http://%s:%d%s\n", cfg.Server.Host, cfg.Server.Port, cfg.Server.GetBasePath())
	fmt.Fprintf(os.Stderr, "  Auth: %s", cfg.Auth.Method)
	if cfg.Auth.SingleUser != "" {
		fmt.Fprintf(os.Stderr, " (user: %s)", cfg.Auth.SingleUser)
	}
	fmt.Fprintf(os.Stderr, "\n")
	if cfg.Server.AllowAllIPs {
		fmt.Fprintf(os.Stderr, "  IP Filter: DISABLED (allow_all_ips = true)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  IP Filter: %v\n", cfg.Server.AllowedIPs)
	}
	if termMgr.HasTmux() {
		fmt.Fprintf(os.Stderr, "  Terminal: tmux (sessions persist)\n")
	} else {
		fmt.Fprintf(os.Stderr, "  Terminal: plain pty (install tmux for session persistence)\n")
	}
	fmt.Fprintf(os.Stderr, "\n")

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Fprintf(os.Stderr, "\nShutting down...\n")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Shutdown error: %v", err)
		}
		termMgr.Stop()
	}()

	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server error: %v", err)
	}
}

func validateSecurityConfig(cfg *config.Config) error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("cannot determine current Unix user: %w", err)
	}

	if cfg.Auth.SingleUser == "" {
		return errors.New("single_user is required; RoamBench is single-user only")
	}
	if cfg.Auth.SingleUser != currentUser.Username {
		return fmt.Errorf("single_user %q must match the Unix account running roambench (%q)", cfg.Auth.SingleUser, currentUser.Username)
	}

	if (cfg.Server.TLSCert == "") != (cfg.Server.TLSKey == "") {
		return errors.New("tls_cert and tls_key must both be set together")
	}

	if cfg.Server.TLSCert == "" && !cfg.Server.AllowInsecureHTTP && !isLoopbackHost(cfg.Server.Host) {
		return fmt.Errorf("refusing insecure HTTP on %q; configure TLS or set allow_insecure_http = true for trusted testing", cfg.Server.Host)
	}

	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	trimmed := strings.Trim(host, "[]")
	ip := net.ParseIP(trimmed)
	return ip != nil && ip.IsLoopback()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
