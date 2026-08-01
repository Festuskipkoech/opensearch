package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"opensearch/internal/api"
	"opensearch/internal/cache"
	"opensearch/internal/classifier"
	"opensearch/internal/config"
	"opensearch/internal/orchestrator"
	"opensearch/internal/router"
	"opensearch/internal/searxng"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	// redis
	redisCache, err := cache.New(cfg.RedisURL, cfg.RedisDB)
	if err != nil {
		slog.Error("redis init failed", "error", err)
		os.Exit(1)
	}
	defer redisCache.Close()

	// verify redis reachable before accepting traffic
	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisCache.Ping(pingCtx); err != nil {
		slog.Error("redis ping failed", "error", err)
		os.Exit(1)
	}
	slog.Info("redis connected")

	// model service gRPC connection
	modelConn, err := grpc.NewClient(
		cfg.ModelServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("model service connection failed", "error", err)
		os.Exit(1)
	}
	defer modelConn.Close()
	slog.Info("model service connected", "addr", cfg.ModelServiceAddr)

	// classifier
	clf := classifier.New(modelConn, float32(cfg.ClassifierConfidenceThreshold))

	// router
	rtr := router.New()

	// searxng client
	searxngClient := searxng.New(cfg.SearXNGURL, cfg.MinFanOutResults)

	// orchestrator
	orch := orchestrator.New(
		redisCache,
		clf,
		rtr,
		searxngClient,
		cfg.TTLForIntent,
	)

	// api
	mux := http.NewServeMux()
	handler := api.NewHandler(orch)
	api.RegisterRoutes(mux, handler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      api.Chain(mux, api.CORS, api.RequestID, api.Logger),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// start server in goroutine
	go func() {
		slog.Info("server running", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// wait for shutdown signal
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	slog.Info("shutdown signal received")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("server stopped cleanly")
}