package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"opensearch/internal/api"
	"opensearch/internal/cache"
	"opensearch/internal/classifier"
	"opensearch/internal/config"
	"opensearch/internal/orchestrator"
	"opensearch/internal/router"
	"opensearch/internal/searxng"
	"opensearch/internal/crawler"
)

const (
	startupRetries = 10
	startupInterval = 3 * time.Second
)

func main() {
	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file, reading from environment", "error", err)
	}

	slog.Info("env loaded")

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config error", "error", err)
		os.Exit(1)
	}

	// Redis
	redisCache, err := cache.New(cfg.RedisURL, cfg.RedisDB)
	if err != nil {
		slog.Error("redis init failed", "error", err)
		os.Exit(1)
	}
	defer redisCache.Close()

	if err := waitForRedis(redisCache); err != nil {
		slog.Error("redis not ready", "error", err)
		os.Exit(1)
	}

	// SearXNG
	if err := waitForSearXNG(cfg.SearXNGURL); err != nil {
		slog.Error("searxng not ready", "error", err)
		os.Exit(1)
	}

	// Model service gRPC connection
	modelConn, err := grpc.NewClient(
		cfg.ModelServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("model service connection failed", "error", err)
		os.Exit(1)
	}
	defer modelConn.Close()

	if err := waitForGRPC(modelConn, "model-service"); err != nil {
		slog.Error("model service not ready", "error", err)
		os.Exit(1)
	}

	// Spider-rs gRPC connection
	spiderConn, err := grpc.NewClient(
		cfg.SpiderAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		slog.Error("spider connection failed", "error", err)
		os.Exit(1)
	}
	defer spiderConn.Close()

	if err := waitForGRPC(spiderConn, "spider"); err != nil {
		slog.Error("spider not ready", "error", err)
		os.Exit(1)
	}

	// Wire dependencies
	clf := classifier.New(modelConn, float32(cfg.ClassifierConfidenceThreshold))
	rtr := router.New()
	searxngClient := searxng.New(cfg.SearXNGURL, cfg.MinFanOutResults)

	orch := orchestrator.New(
		redisCache,
		clf,
		rtr,
		searxngClient,
		&crawler.Decider{
			ModelConn: modelConn,
			SpiderConn: spiderConn,
		},
		cfg.TTLForIntent,
	)

	mux := http.NewServeMux()
	handler := api.NewHandler(orch)
	api.RegisterRoutes(mux, handler)

	srv := &http.Server{
		Addr: ":" + cfg.Port,
		Handler: api.Chain(mux, api.CORS, api.RequestID, api.Logger),
		ReadTimeout: 30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout: 120 * time.Second,
	}

	go func() {
		slog.Info("server running", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	slog.Info("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}

	slog.Info("server stopped cleanly")
}

func waitForRedis(c *cache.Cache) error {
	for i := range startupRetries {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := c.Ping(ctx)
		cancel()
		if err == nil {
			slog.Info("redis connected")
			return nil
		}
		slog.Warn("redis not ready, retrying", "attempt", i+1, "of", startupRetries, "error", err)
		time.Sleep(startupInterval)
	}
	return fmt.Errorf("redis did not become ready after %d attempts", startupRetries)
}

func waitForSearXNG(baseURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	url := baseURL + "/healthz"

	for i := range startupRetries {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				slog.Info("searxng connected")
				return nil
			}
		}
		slog.Warn("searxng not ready, retrying", "attempt", i+1, "of", startupRetries)
		time.Sleep(startupInterval)
	}
	return fmt.Errorf("searxng did not become ready after %d attempts", startupRetries)
}

func waitForGRPC(conn *grpc.ClientConn, name string) error {
	hc := grpc_health_v1.NewHealthClient(conn)

	for i := range startupRetries {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		cancel()
		if err == nil && resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
			slog.Info("connected", "service", name)
			return nil
		}
		slog.Warn("not ready, retrying", "service", name, "attempt", i+1, "of", startupRetries)
		time.Sleep(startupInterval)
	}
	return fmt.Errorf("%s did not become ready after %d attempts", name, startupRetries)
}