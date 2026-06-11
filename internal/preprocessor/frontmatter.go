package preprocessor

import (
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

const fmDelim = "---"

func HasFrontMatter(content string) bool {
	_, _, ok := extractFrontMatter(content)
	return ok
}

func extractFrontMatter(content string) (fm string, body string, ok bool) {
	s := strings.TrimLeft(content, "\r\n")
	if !strings.HasPrefix(s, fmDelim) {
		return "", content, false
	}
	rest := s[len(fmDelim):]
	rest = strings.TrimLeft(rest, "\r\n")

	type marker struct{ search, newline, delim string }
	markers := []marker{
		{"\n---", "\n", "---"},
		{"\r\n---", "\r\n", "---"},
		{"\n...", "\n", "..."},
		{"\r\n...", "\r\n", "..."},
	}
	var best struct {
		idx     int
		newline string
		delim   string
		found   bool
	}
	for _, m := range markers {
		idx := strings.Index(rest, m.search)
		if idx >= 0 && (!best.found || idx < best.idx) {
			best.idx = idx
			best.newline = m.newline
			best.delim = m.delim
			best.found = true
		}
	}
	if !best.found {
		return "", content, false
	}

	fm = strings.TrimLeft(rest[:best.idx], "\r\n")
	closeLen := len(best.newline) + len(best.delim)
	body = strings.TrimLeft(rest[best.idx+closeLen:], "\r\n")
	return fm, body, true
}

func InjectSourceURL(content, sourceURL string) string {
	if sourceURL == "" {
		return content
	}

	fm, body, hasFM := extractFrontMatter(content)

	var fmData map[string]any
	if hasFM {
		if err := yaml.Unmarshal([]byte(fm), &fmData); err != nil {
			slog.Warn("failed to parse existing front matter, prepending new block",
				"error", err)
			hasFM = false
		}
	}

	if !hasFM {
		fmData = make(map[string]any)
		body = content
	}

	fmData["source_url"] = sourceURL

	fmBytes, err := yaml.Marshal(fmData)
	if err != nil {
		slog.Warn("failed to marshal front matter, returning content unchanged",
			"error", err)
		return content
	}

	var b strings.Builder
	b.WriteString(fmDelim)
	b.WriteString("\n")
	b.Write(fmBytes)
	b.WriteString(fmDelim)
	b.WriteString("\n")
	if body != "" {
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}
