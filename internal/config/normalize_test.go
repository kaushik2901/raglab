package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeBaseURL_WithV1Slash(t *testing.T) {
	assert.Equal(t, "https://api.openai.com", NormalizeBaseURL("https://api.openai.com/v1/"))
}

func TestNormalizeBaseURL_WithV1(t *testing.T) {
	assert.Equal(t, "https://api.openai.com", NormalizeBaseURL("https://api.openai.com/v1"))
}

func TestNormalizeBaseURL_NoSuffix(t *testing.T) {
	assert.Equal(t, "https://api.openai.com", NormalizeBaseURL("https://api.openai.com"))
}

func TestNormalizeBaseURL_Empty(t *testing.T) {
	assert.Equal(t, "", NormalizeBaseURL(""))
}

func TestNormalizeBaseURL_V1InPath(t *testing.T) {
	assert.Equal(t, "https://example.com/v1/api", NormalizeBaseURL("https://example.com/v1/api/v1/"))
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	assert.Equal(t, 5*time.Second, ParseRetryAfter("5"))
}

func TestParseRetryAfter_Zero(t *testing.T) {
	assert.Equal(t, 0*time.Second, ParseRetryAfter("0"))
}

func TestParseRetryAfter_Large(t *testing.T) {
	assert.Equal(t, 120*time.Second, ParseRetryAfter("120"))
}

func TestParseRetryAfter_Empty(t *testing.T) {
	assert.Equal(t, 0*time.Second, ParseRetryAfter(""))
}

func TestParseRetryAfter_Invalid(t *testing.T) {
	assert.Equal(t, 0*time.Second, ParseRetryAfter("not-a-number"))
}

func TestParseRetryAfter_RFC1123(t *testing.T) {
	d := ParseRetryAfter(time.Now().Add(10 * time.Second).Format(time.RFC1123))
	assert.Greater(t, d, 5*time.Second)
	assert.Less(t, d, 15*time.Second)
}

func TestParseRetryAfter_RFC1123_Past(t *testing.T) {
	d := ParseRetryAfter(time.Now().Add(-1 * time.Hour).Format(time.RFC1123))
	assert.Equal(t, 0*time.Second, d)
}

func TestWarnOnInsecure(t *testing.T) {
	var buf []byte
	h := slog.NewJSONHandler(&bufWriter{&buf}, &slog.HandlerOptions{Level: slog.LevelWarn})
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(orig)

	WarnOnInsecure("http://localhost:1234", "sk-test-key", "embedder")
	assert.NotEmpty(t, buf, "should log warning for http + api key")
}

func TestWarnOnInsecure_HTTPS(t *testing.T) {
	var buf []byte
	h := slog.NewJSONHandler(&bufWriter{&buf}, &slog.HandlerOptions{Level: slog.LevelWarn})
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(orig)

	WarnOnInsecure("https://api.openai.com", "sk-test-key", "embedder")
	assert.Empty(t, buf, "should not warn for https")
}

func TestWarnOnInsecure_HTTPNoKey(t *testing.T) {
	var buf []byte
	h := slog.NewJSONHandler(&bufWriter{&buf}, &slog.HandlerOptions{Level: slog.LevelWarn})
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(orig)

	WarnOnInsecure("http://localhost:1234", "", "embedder")
	assert.Empty(t, buf, "should not warn when no api key")
}

type bufWriter struct {
	buf *[]byte
}

func (w *bufWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
