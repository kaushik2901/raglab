package parser

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/types"
)

type Parser interface {
	Parse(filePath string) (types.ElementReader, error)
}

var Default Parser = &MarkdownParser{}

var parsers = map[string]func() Parser{
	"markdown": func() Parser { return &MarkdownParser{} },
}

func New(strategy string) (Parser, error) {
	fn, ok := parsers[strategy]
	if !ok {
		return nil, fmt.Errorf("unknown parser %q", strategy)
	}
	return fn(), nil
}

func RegisterParser(name string, fn func() Parser) {
	parsers[name] = fn
}

func ParseDir(dirPath string) ([]types.Document, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("parse dir %s: %w", dirPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("parse dir %s: not a directory", dirPath)
	}

	var docs []types.Document
	err = filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing %s: %w", path, err)
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)

		doc, err := ParseFile(path, relPath)
		if err != nil {
			return err
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}

func ParseFile(filePath string, relPath string) (types.Document, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return types.Document{}, fmt.Errorf("parse file %s: %w", filePath, err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return types.Document{}, fmt.Errorf("read file %s: %w", filePath, err)
	}

	return types.Document{
		Path:    relPath,
		Content: string(content),
		Size:    int64(len(content)),
	}, nil
}
