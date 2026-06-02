package preprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var includeRe = regexp.MustCompile(`\{\{%\s*include\s+"([^"]+)"\s*%\}\}`)

func ResolveIncludes(content string, repoRoot string, visited map[string]bool) (string, error) {
	return resolveIncludes(content, repoRoot, visited, 0)
}

func resolveIncludes(content string, repoRoot string, visited map[string]bool, depth int) (string, error) {
	if depth > 100 {
		return "", fmt.Errorf("max include depth exceeded")
	}

	result := includeRe.ReplaceAllStringFunc(content, func(match string) string {
		matches := includeRe.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		includePath := matches[1]

		absPath := includePath
		if !filepath.IsAbs(includePath) {
			absPath = filepath.Join(repoRoot, includePath)
		}
		absPath = filepath.Clean(absPath)

		if visited[absPath] {
			return ""
		}

		ext := strings.ToLower(filepath.Ext(absPath))
		if ext != ".md" && ext != ".markdown" {
			return match
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			return match
		}

		visited[absPath] = true
		defer delete(visited, absPath)

		replaced, err := resolveIncludes(string(data), repoRoot, visited, depth+1)
		if err != nil {
			return match
		}

		return replaced
	})

	return result, nil
}
