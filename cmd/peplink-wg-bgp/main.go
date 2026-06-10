package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"peplink-wg-bgp/internal/config"
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
	srv, err := server.New(cfg, web.Templates, web.Static)
	if err != nil {
		log.Fatalf("create server: %v", err)
	}
	log.Printf("login token URL: http://<router-or-container-host>%s", srv.LoginURL())
	log.Printf("login sessions expire after 1 hour")
	log.Printf("listening on %s", cfg.ListenAddr)
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

func runSupervisor() {
	socketPath := os.Getenv("SUPERVISOR_SOCKET")
	if socketPath == "" {
		socketPath = supervisor.DefaultSocketPath
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("supervisor listening on %s", socketPath)
	if err := (supervisor.Server{SocketPath: socketPath}).Serve(ctx); err != nil {
		log.Fatal(err)
	}
}
