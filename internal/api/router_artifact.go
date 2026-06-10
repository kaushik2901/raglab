package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
)

type ArtifactRouter struct {
	artifactsDir string
}

func NewArtifactRouter(artifactsDir string) *ArtifactRouter {
	return &ArtifactRouter{artifactsDir: artifactsDir}
}

func (r *ArtifactRouter) Register(mux chi.Router) {
	mux.Get("/artifacts", r.listHandler)
}

func (r *ArtifactRouter) listHandler(w http.ResponseWriter, req *http.Request) {
	artifactType := req.URL.Query().Get("type")
	tag := req.URL.Query().Get("tag")

	baseDir := r.artifactsDir
	if baseDir == "" {
		baseDir = "artifacts"
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", "cannot list artifacts")
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
