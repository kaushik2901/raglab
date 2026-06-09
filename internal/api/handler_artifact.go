package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) artifactListHandler(w http.ResponseWriter, r *http.Request) {
	artifactType := r.URL.Query().Get("type")
	tag := r.URL.Query().Get("tag")

	baseDir := "artifacts"
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		respondError(w, 500, "INTERNAL_ERROR", "cannot list artifacts")
		return
	}

	var artifacts []ArtifactEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if artifactType != "" && entry.Name() != artifactType {
			continue
		}
		tags, err := os.ReadDir(filepath.Join(baseDir, entry.Name()))
		if err != nil {
			continue
		}
		for _, tagEntry := range tags {
			if !tagEntry.IsDir() {
				continue
			}
			if tag != "" && tagEntry.Name() != tag {
				continue
			}
			artifactDir := filepath.Join(baseDir, entry.Name(), tagEntry.Name())
			info := ArtifactEntry{
				Type: entry.Name(),
				Tag:  tagEntry.Name(),
			}
			outputDir := filepath.Join(artifactDir, "output")
			if fi, err := os.Stat(outputDir); err == nil && fi.IsDir() {
				fileCount := countFiles(outputDir, ".md")
				info.FileCount = &fileCount
			}
			artifacts = append(artifacts, info)
		}
	}

	if artifacts == nil {
		artifacts = []ArtifactEntry{}
	}

	respondJSON(w, 200, map[string]any{"artifacts": artifacts})
}

func countFiles(dir, ext string) int {
	var count int
	filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			return nil
		}
		if strings.HasSuffix(fi.Name(), ext) {
			count++
		}
		return nil
	})
	return count
}
