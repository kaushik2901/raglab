package config

import (
	"errors"
	"flag"
	"os"
	"strconv"
	"time"
)

type Config struct {
	RepoURL      string
	RepoPath     string
	OutputPath   string
	MaxRetries   int
	RetryBackoff time.Duration
	LogLevel     string
}

func Load() (*Config, error) {
	cfg := &Config{}

	flag.StringVar(&cfg.RepoURL, "repo-url", envOrDefault("REPO_URL", "https://gitlab.com/gitlab-com/content-sites/handbook"), "Repository URL to clone")
	flag.StringVar(&cfg.RepoPath, "repo-path", envOrDefault("REPO_PATH", "./handbook"), "Local path for repository clone")
	flag.StringVar(&cfg.OutputPath, "output", envOrDefault("OUTPUT_PATH", "./output"), "Output directory for cleaned markdown")
	flag.IntVar(&cfg.MaxRetries, "max-retries", intEnvOrDefault("MAX_RETRIES", 3), "Maximum retry count for stages")
	flag.DurationVar(&cfg.RetryBackoff, "retry-backoff", durationEnvOrDefault("RETRY_BACKOFF", 5*time.Second), "Retry backoff duration")
	flag.StringVar(&cfg.LogLevel, "log-level", envOrDefault("LOG_LEVEL", "info"), "Log level (debug/info/warn)")
	flag.Parse()

	return cfg, cfg.Validate()
}

func (c *Config) Validate() error {
	if c.RepoURL == "" {
		return errors.New("repo-url is required")
	}
	if c.RepoPath == "" {
		return errors.New("repo-path is required")
	}
	if c.OutputPath == "" {
		return errors.New("output-path is required")
	}
	if c.MaxRetries < 0 {
		return errors.New("max-retries must be non-negative")
	}
	if c.RetryBackoff <= 0 {
		return errors.New("retry-backoff must be positive")
	}
	return nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func intEnvOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func durationEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
