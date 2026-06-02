package config

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestValidate_EmptyRepoURL(t *testing.T) {
	cfg := &Config{RepoPath: "/path", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty RepoURL")
	}
}

func TestValidate_EmptyRepoPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", OutputPath: "/out", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty RepoPath")
	}
}

func TestValidate_EmptyOutputPath(t *testing.T) {
	cfg := &Config{RepoURL: "https://example.com", RepoPath: "/path", MaxRetries: 3, RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty OutputPath")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com/repo",
		RepoPath:     "/tmp/repo",
		OutputPath:   "/tmp/output",
		MaxRetries:   3,
		RetryBackoff: 5 * time.Second,
		LogLevel:     "info",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidate_NegativeMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   -1,
		RetryBackoff: 5 * time.Second,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative MaxRetries")
	}
}

func TestValidate_ZeroMaxRetries(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   0,
		RetryBackoff: 5 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil for zero MaxRetries, got: %v", err)
	}
}

func TestValidate_ZeroRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   3,
		RetryBackoff: 0,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero RetryBackoff")
	}
}

func TestValidate_NegativeRetryBackoff(t *testing.T) {
	cfg := &Config{
		RepoURL:      "https://example.com",
		RepoPath:     "/path",
		OutputPath:   "/out",
		MaxRetries:   3,
		RetryBackoff: -5 * time.Second,
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative RetryBackoff")
	}
}

func TestValidate_AllEmpty(t *testing.T) {
	cfg := &Config{RetryBackoff: 5 * time.Second}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for all empty required fields")
	}
}

func TestEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_ENV_KEY", "from_env")
	defer os.Unsetenv("TEST_ENV_KEY")

	if got := envOrDefault("TEST_ENV_KEY", "default"); got != "from_env" {
		t.Errorf("envOrDefault = %q, want %q", got, "from_env")
	}
}

func TestEnvOrDefault_Fallback(t *testing.T) {
	if got := envOrDefault("TEST_ENV_KEY_NONEXISTENT", "default"); got != "default" {
		t.Errorf("envOrDefault = %q, want %q", got, "default")
	}
}

func TestEnvOrDefault_EmptyVar(t *testing.T) {
	os.Setenv("TEST_ENV_EMPTY", "")
	defer os.Unsetenv("TEST_ENV_EMPTY")

	if got := envOrDefault("TEST_ENV_EMPTY", "default"); got != "default" {
		t.Errorf("envOrDefault = %q, want %q", got, "default")
	}
}

func TestIntEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_INT_KEY", "42")
	defer os.Unsetenv("TEST_INT_KEY")

	if got := intEnvOrDefault("TEST_INT_KEY", 1); got != 42 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 42)
	}
}

func TestIntEnvOrDefault_Fallback(t *testing.T) {
	if got := intEnvOrDefault("TEST_INT_NONEXISTENT", 10); got != 10 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 10)
	}
}

func TestIntEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_INT_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INT_INVALID")

	if got := intEnvOrDefault("TEST_INT_INVALID", 7); got != 7 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 7)
	}
}

func TestIntEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_INT_EMPTY", "")
	defer os.Unsetenv("TEST_INT_EMPTY")

	if got := intEnvOrDefault("TEST_INT_EMPTY", 5); got != 5 {
		t.Errorf("intEnvOrDefault = %d, want %d", got, 5)
	}
}

func TestDurationEnvOrDefault(t *testing.T) {
	os.Setenv("TEST_DUR_KEY", "10s")
	defer os.Unsetenv("TEST_DUR_KEY")

	if got := durationEnvOrDefault("TEST_DUR_KEY", time.Second); got != 10*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 10*time.Second)
	}
}

func TestDurationEnvOrDefault_Fallback(t *testing.T) {
	if got := durationEnvOrDefault("TEST_DUR_NONEXISTENT", 30*time.Second); got != 30*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 30*time.Second)
	}
}

func TestDurationEnvOrDefault_InvalidValue(t *testing.T) {
	os.Setenv("TEST_DUR_INVALID", "not-a-duration")
	defer os.Unsetenv("TEST_DUR_INVALID")

	if got := durationEnvOrDefault("TEST_DUR_INVALID", 3*time.Second); got != 3*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 3*time.Second)
	}
}

func TestDurationEnvOrDefault_EmptyValue(t *testing.T) {
	os.Setenv("TEST_DUR_EMPTY", "")
	defer os.Unsetenv("TEST_DUR_EMPTY")

	if got := durationEnvOrDefault("TEST_DUR_EMPTY", 2*time.Second); got != 2*time.Second {
		t.Errorf("durationEnvOrDefault = %v, want %v", got, 2*time.Second)
	}
}

func parseTestFlags(args []string) (*Config, error) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := &Config{}

	fs.StringVar(&cfg.RepoURL, "repo-url", "https://gitlab.com/gitlab-com/content-sites/handbook", "")
	fs.StringVar(&cfg.RepoPath, "repo-path", "./handbook", "")
	fs.StringVar(&cfg.OutputPath, "output", "./output", "")
	fs.IntVar(&cfg.MaxRetries, "max-retries", 3, "")
	fs.DurationVar(&cfg.RetryBackoff, "retry-backoff", 5*time.Second, "")
	fs.StringVar(&cfg.LogLevel, "log-level", "info", "")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cfg, cfg.Validate()
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := parseTestFlags([]string{})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.RepoURL != "https://gitlab.com/gitlab-com/content-sites/handbook" {
		t.Errorf("RepoURL = %q, want %q", cfg.RepoURL, "https://gitlab.com/gitlab-com/content-sites/handbook")
	}
	if cfg.RepoPath != "./handbook" {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, "./handbook")
	}
	if cfg.OutputPath != "./output" {
		t.Errorf("OutputPath = %q, want %q", cfg.OutputPath, "./output")
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 3)
	}
	if cfg.RetryBackoff != 5*time.Second {
		t.Errorf("RetryBackoff = %v, want %v", cfg.RetryBackoff, 5*time.Second)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestLoad_WithFlags(t *testing.T) {
	cfg, err := parseTestFlags([]string{
		"--repo-url", "https://custom.com/repo",
		"--repo-path", "/custom/path",
		"--output", "/custom/output",
		"--max-retries", "5",
		"--retry-backoff", "10s",
		"--log-level", "debug",
	})
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, want %d", cfg.MaxRetries, 5)
	}
	if cfg.RetryBackoff != 10*time.Second {
		t.Errorf("RetryBackoff = %v, want %v", cfg.RetryBackoff, 10*time.Second)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
}
