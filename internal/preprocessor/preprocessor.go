package preprocessor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/kaushik2901/raglab/internal/types"
	"golang.org/x/sync/errgroup"
)

func ProcessFile(filePath string, repoRoot string, baseURL string) (*types.Document, error) {
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

	var sourceURL string
	if baseURL != "" {
		pagePath := strings.TrimSuffix(relPath, ".md")
		// Hugo convention: _index.md and /index.md map to their parent directory
		if strings.HasSuffix(pagePath, "/_index") || pagePath == "_index" {
			pagePath = strings.TrimSuffix(pagePath, "_index")
			pagePath = strings.TrimSuffix(pagePath, "/")
		}
		if strings.HasSuffix(pagePath, "/index") || pagePath == "index" {
			pagePath = strings.TrimSuffix(pagePath, "index")
			pagePath = strings.TrimSuffix(pagePath, "/")
		}
		sourceURL = strings.TrimRight(baseURL, "/") + "/" + pagePath
		if !strings.HasSuffix(sourceURL, "/") {
			sourceURL += "/"
		}
		content = InjectSourceURL(content, sourceURL)
	}

	return &types.Document{
		Path:      relPath,
		Content:   content,
		Size:      int64(len(content)),
		SourceURL: sourceURL,
	}, nil
}

func ProcessAllFiles(ctx context.Context, srcRoot string, subdirs []string, dstDir string, concurrency int, baseURL string) (int, error) {
	if concurrency <= 0 {
		concurrency = 10
	}

	var walkDirs []string
	if len(subdirs) == 0 {
		walkDirs = []string{srcRoot}
	} else {
		for _, sd := range subdirs {
			sd = strings.TrimPrefix(sd, "content/")
			sd = strings.TrimPrefix(sd, "content\\")
			walkDirs = append(walkDirs, filepath.Join(srcRoot, sd))
		}
	}

	var (
		mdFiles []string
		err     error
	)
	for _, wd := range walkDirs {
		_, statErr := os.Stat(wd)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				slog.Warn("include directory not found, skipping", "path", wd)
				continue
			}
			return 0, fmt.Errorf("stat walk dir %s: %w", wd, statErr)
		}

		err = filepath.Walk(wd, func(path string, info os.FileInfo, err error) error {
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
			return 0, fmt.Errorf("walk dir %s: %w", wd, err)
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	var processed atomic.Int32

	for _, filePath := range mdFiles {
		fp := filePath
		g.Go(func() error {
			doc, err := ProcessFile(fp, srcRoot, baseURL)
			if err != nil {
				return fmt.Errorf("process %s: %w", fp, err)
			}

			outPath := filepath.Join(dstDir, doc.Path)
			outDir := filepath.Dir(outPath)
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("create output dir: %w", err)
			}
			if err := os.WriteFile(outPath, []byte(doc.Content), 0644); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
			processed.Add(1)
			return nil
		})
	}

	err = g.Wait()
	return int(processed.Load()), err
}
