package config

import (
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func NormalizeBaseURL(baseURL string) string {
	for _, suffix := range []string{"/v1/", "/v1"} {
		if len(baseURL) >= len(suffix) && baseURL[len(baseURL)-len(suffix):] == suffix {
			return baseURL[:len(baseURL)-len(suffix)]
		}
	}
	return baseURL
}

func WarnOnInsecure(baseURL, apiKey, label string) {
	if apiKey != "" && strings.HasPrefix(baseURL, "http://") {
		slog.Warn("API key sent over non-TLS connection", "label", label, "base_url", baseURL)
	}
}

func ParseRetryAfter(val string) time.Duration {
	if val == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(val); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := time.Parse(time.RFC1123, val); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
