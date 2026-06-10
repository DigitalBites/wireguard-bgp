package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/orchestrator"
	"peplink-wg-bgp/internal/server"
	"peplink-wg-bgp/internal/supervisor"
	"peplink-wg-bgp/web"
)

func main() {
	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	switch mode {
	case "serve":
		runServe()
	case "supervisor":
		runSupervisor()
	default:
		log.Fatalf("unknown mode %q", mode)
	}
}

func runServe() {
	configPath := os.Getenv("APP_CONFIG")
	if configPath == "" {
		configPath = "/app-state/app.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	token, err := server.GenerateToken()
	if err != nil {
		log.Fatalf("generate login token: %v", err)
	}
	srv, err := server.NewWithAuth(cfg, web.Templates, web.Static, server.AuthConfig{
		Token:        token,
		SessionTTL:   time.Hour,
		CookieSecure: os.Getenv("COOKIE_SECURE") == "true",
	})
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	log.Printf("login path: http://<router-or-container-host>/login")
	log.Printf("login token: %s", srv.LoginToken())
	log.Printf("login sessions expire after 1 hour")
	log.Printf("listening on %s", cfg.ListenAddr)
	maybeAutoStart(cfg)
	httpServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(httpServer.ListenAndServe())
}

func maybeAutoStart(cfg config.App) {
	if !cfg.Runtime.AutoStart {
		return
	}
	if missing, err := missingAutoStartConfig(cfg); err != nil {
		log.Printf("auto start skipped: required config %s is not available: %v", missing, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		log.Printf("auto start enabled; starting WireGuard, routes, and BIRD")
		result := (orchestrator.Routing{Client: supervisor.Client{}}).Start(ctx)
		if !result.OK {
			log.Printf("auto start failed: %s", orchestrator.ActionSummary(result))
			return
		}
		log.Printf("auto start completed: %s", orchestrator.ActionSummary(result))
	}()
}

func missingAutoStartConfig(cfg config.App) (string, error) {
	wgPath := filepath.Join(cfg.ConfigDir, "wireguard", "wg0.conf")
	for _, path := range []string{wgPath, cfg.BIRDConfigPath} {
		if _, err := os.Stat(path); err != nil {
			return path, err
		}
	}
	return "", nil
}

func runSupervisor() {
	socketPath := os.Getenv("SUPERVISOR_SOCKET")
	if socketPath == "" {
		socketPath = supervisor.DefaultSocketPath
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("supervisor listening on %s", socketPath)
	if err := (supervisor.Server{SocketPath: socketPath, AllowedUID: appUID()}).Serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func appUID() int {
	if raw := os.Getenv("APP_UID"); raw != "" {
		uid, err := strconv.Atoi(raw)
		if err != nil {
			log.Fatalf("APP_UID must be numeric: %v", err)
		}
		return uid
	}
	appUser, err := user.Lookup("app")
	if err != nil {
		return 0
	}
	uid, err := strconv.Atoi(appUser.Uid)
	if err != nil {
		log.Fatalf("app user UID must be numeric: %v", err)
	}
	return uid
}
