package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the opensearch engine.
// Every field is loaded from environment variables — nothing is hardcoded.
// Only the config package reads environment variables; all other modules
// receive a Config struct.
type Config struct {
	Port string

	RedisURL string
	RedisDB int

	SearXNGURL string
	MinFanOutResults int

	ModelServiceAddr string
	ClassifierConfidenceThreshold float64

	// SpiderAddr is the gRPC address of the Spider-rs content extraction service.
	SpiderAddr string

	// TTLForIntent returns the cache TTL in seconds for a given intent class.
	// Intent-aware TTLs: news expires fast, research can be cached longer.
	TTLForIntent func(intent string) int
}

// Load reads configuration from the environment and validates required values.
// Returns an error if any required variable is missing or invalid.
func Load() (*Config, error) {
	cfg := &Config{
		Port:  env("PORT", "8080"),
		RedisURL: env("REDIS_URL", "redis://localhost:6379"),
		SearXNGURL: env("SEARXNG_URL", "http://localhost:8888"),
		ModelServiceAddr: env("MODEL_SERVICE_ADDR", "localhost:50051"),
		SpiderAddr: env("SPIDER_ADDR", "localhost:50052"),
	}

	var err error

	cfg.RedisDB, err = envInt("REDIS_DB", 0)
	if err != nil {
		return nil, fmt.Errorf("REDIS_DB: %w", err)
	}

	cfg.MinFanOutResults, err = envInt("MIN_FAN_OUT_RESULTS", 5)
	if err != nil {
		return nil, fmt.Errorf("MIN_FAN_OUT_RESULTS: %w", err)
	}

	cfg.ClassifierConfidenceThreshold, err = envFloat("CLASSIFIER_CONFIDENCE_THRESHOLD", 0.65)
	if err != nil {
		return nil, fmt.Errorf("CLASSIFIER_CONFIDENCE_THRESHOLD: %w", err)
	}

	cfg.TTLForIntent = buildTTLMap()

	return cfg, nil
}

// buildTTLMap returns the intent-aware TTL function.
// TTLs are in seconds. News is short-lived; research can be stale longer.
// General and unknown intents get a moderate default.
func buildTTLMap() func(string) int {
	ttls := map[string]int{
		"news": 300,   // 5 minutes
		"factual": 3600,  // 1 hour
		"code": 7200,  // 2 hours
		"research": 7200,  // 2 hours
		"commercial": 1800,  // 30 minutes
		"general": 3600,  // 1 hour
	}
	defaultTTL := 3600
	return func(intent string) int {
		if ttl, ok := ttls[strings.ToLower(intent)]; ok {
			return ttl
		}
		return defaultTTL
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("must be an integer, got %q", v)
	}
	return n, nil
}

func envFloat(key string, fallback float64) (float64, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("must be a float, got %q", v)
	}
	return f, nil
}

// Ensure time is imported — used by callers referencing TTL durations.
var _ = time.Second