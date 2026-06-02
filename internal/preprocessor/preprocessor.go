package preprocessor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

func ProcessFile(filePath string, repoRoot string) (*types.Document, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := string(data)

	content, err = ResolveIncludes(content, repoRoot, make(map[string]bool))
	if err != nil {
		return nil, fmt.Errorf("resolve includes: %w", err)
	}

	rules := defaultShortcodeRules()
	content = StripShortcodes(content, rules)

	content = ProcessHTML(content)

	content, err = ResolveRefs(content, repoRoot, filePath)
	if err != nil {
		return nil, fmt.Errorf("resolve refs: %w", err)
	}

	relPath, err := filepath.Rel(repoRoot, filePath)
	if err != nil {
		return nil, fmt.Errorf("relative path: %w", err)
	}
	relPath = filepath.ToSlash(relPath)

	return &types.Document{
		Path:    relPath,
		Content: content,
		Size:    int64(len(content)),
	}, nil
}

func ProcessAllFiles(srcDir string, dstDir string, concurrency int) (int, error) {
	if concurrency <= 0 {
		concurrency = 10
	}

	var mdFiles []string
	_, statErr := os.Stat(srcDir)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat source dir: %w", statErr)
	}

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(info.Name()))
		if ext == ".md" || ext == ".markdown" {
			mdFiles = append(mdFiles, path)
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk source dir: %w", err)
	}

	sem := make(chan struct{}, concurrency)
	errCh := make(chan error, len(mdFiles))
	type result struct {
		doc *types.Document
		err error
	}
	resultCh := make(chan result, len(mdFiles))

	for _, filePath := range mdFiles {
		sem <- struct{}{}
		go func(fp string) {
			defer func() { <-sem }()

			doc, err := ProcessFile(fp, srcDir)
			if err != nil {
				errCh <- fmt.Errorf("process %s: %w", fp, err)
				resultCh <- result{err: err}
				return
			}
			resultCh <- result{doc: doc}
		}(filePath)
	}

	for i := 0; i < cap(sem); i++ {
		sem <- struct{}{}
	}

	close(resultCh)
	close(errCh)

	processed := 0
	for r := range resultCh {
		if r.err != nil {
			continue
		}
		doc := r.doc

		outPath := filepath.Join(dstDir, doc.Path)
		outDir := filepath.Dir(outPath)
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return processed, fmt.Errorf("create output dir: %w", err)
		}

		if err := os.WriteFile(outPath, []byte(doc.Content), 0644); err != nil {
			return processed, fmt.Errorf("write output: %w", err)
		}
		processed++
	}

	return processed, nil
}
