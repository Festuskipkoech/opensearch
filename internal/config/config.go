package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all values the server needs to start.
// All fields are required. Missing any is a fatal startup error.
type Config struct {
	Port string
	RedisURL string
	RedisDB int
	SearXNGURL string
	ModelServiceAddr string
	CrawlerAddr string

	TTLNews int
	TTLFactual int
	TTLCode int
	TTLResearch int
	TTLCommercial int
	TTLGeneral int

	ClassifierConfidenceThreshold float64
	MaxCrawlURLs int
	MinFanOutResults int
}

// Load reads all configuration from environment variables.
// Reports every missing or invalid variable before returning.
func Load() (*Config, error) {
	var missing, invalid []string

	cfg := &Config{
		Port: require("PORT", &missing),
		RedisURL: require("REDIS_URL", &missing),
		SearXNGURL: require("SEARXNG_URL", &missing),
		ModelServiceAddr: require("MODEL_SERVICE_ADDR", &missing),
		CrawlerAddr:      require("CRAWLER_ADDR", &missing),

		RedisDB: requireInt("REDIS_DB", &missing, &invalid),
		TTLNews: requireInt("TTL_NEWS", &missing, &invalid),
		TTLFactual: requireInt("TTL_FACTUAL", &missing, &invalid),
		TTLCode: requireInt("TTL_CODE", &missing, &invalid),
		TTLResearch: requireInt("TTL_RESEARCH", &missing, &invalid),
		TTLCommercial: requireInt("TTL_COMMERCIAL", &missing, &invalid),
		TTLGeneral: requireInt("TTL_GENERAL", &missing, &invalid),
		MaxCrawlURLs: requireInt("MAX_CRAWL_URLS", &missing, &invalid),
		MinFanOutResults: requireInt("MIN_FAN_OUT_RESULTS", &missing, &invalid),

		ClassifierConfidenceThreshold: requireFloat("CLASSIFIER_CONFIDENCE_THRESHOLD", &missing, &invalid),
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing env vars: %s", strings.Join(missing, ", "))
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("invalid env vars: %s", strings.Join(invalid, ", "))
	}

	return cfg, nil
}

// TTLForIntent returns the cache TTL in seconds for a given intent class.
func (c *Config) TTLForIntent(intent string) int {
	switch intent {
	case "news":
		return c.TTLNews
	case "factual":
		return c.TTLFactual
	case "code":
		return c.TTLCode
	case "research":
		return c.TTLResearch
	case "commercial":
		return c.TTLCommercial
	default:
		return c.TTLGeneral
	}
}

func require(key string, missing *[]string) string {
	val := os.Getenv(key)
	if val == "" {
		*missing = append(*missing, key)
	}
	return val
}

func requireInt(key string, missing, invalid *[]string) int {
	val := os.Getenv(key)
	if val == "" {
		*missing = append(*missing, key)
		return 0
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		*invalid = append(*invalid, fmt.Sprintf("%s=%q (must be integer)", key, val))
		return 0
	}
	return n
}

func requireFloat(key string, missing, invalid *[]string) float64 {
	val := os.Getenv(key)
	if val == "" {
		*missing = append(*missing, key)
		return 0
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		*invalid = append(*invalid, fmt.Sprintf("%s=%q (must be float)", key, val))
		return 0
	}
	return f
}