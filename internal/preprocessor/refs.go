package preprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var refRe = regexp.MustCompile(`\{\{<\s*(ref|relref)\s+"([^"]+)"\s*>}}`)

func ResolveRefs(content string, repoRoot string, currentFilePath string) (string, error) {
	var errs []string

	result := refRe.ReplaceAllStringFunc(content, func(match string) string {
		matches := refRe.FindStringSubmatch(match)
		if len(matches) < 3 {
			return match
		}

		refType := matches[1]
		refPath := matches[2]

		var anchor string
		if idx := strings.Index(refPath, "#"); idx >= 0 {
			anchor = refPath[idx:]
			refPath = refPath[:idx]
		}

		targetFile := refPath
		if !strings.HasSuffix(targetFile, ".md") {
			targetFile = targetFile + ".md"
		}

		var resolvedPath string
		var displayPath string

		if refType == "relref" {
			currentDir := filepath.Dir(currentFilePath)
			resolvedPath = filepath.Join(currentDir, targetFile)
			displayPath = refPath
		} else {
			resolvedPath = filepath.Join(repoRoot, targetFile)
			displayPath = refPath
		}

		resolvedPath = filepath.Clean(resolvedPath)

		if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("ref target not found: %s", resolvedPath))
		}

		linkTarget := displayPath + ".md"
		if anchor != "" {
			linkTarget = displayPath + ".md" + anchor
		}

		return fmt.Sprintf("[%s%s](%s)", displayPath, anchor, linkTarget)
	})

	if len(errs) > 0 {
		return result, fmt.Errorf("ref resolution warnings: %s", strings.Join(errs, "; "))
	}

	return result, nil
}
