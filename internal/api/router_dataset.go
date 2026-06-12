package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type DatasetRouter struct {
	datasetsDir string
}

func NewDatasetRouter(datasetsDir string) *DatasetRouter {
	return &DatasetRouter{datasetsDir: datasetsDir}
}

func (r *DatasetRouter) Register(mux chi.Router) {
	mux.Post("/datasets", r.uploadHandler)
	mux.Get("/datasets", r.listHandler)
	mux.Get("/datasets/{name}", r.downloadHandler)
	mux.Delete("/datasets/{name}", r.deleteHandler)
}

func (r *DatasetRouter) uploadHandler(w http.ResponseWriter, req *http.Request) {
	req.Body = http.MaxBytesReader(w, req.Body, 500<<20)

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		respondProblem(w, 400, "Invalid Request", "failed to parse multipart form: "+err.Error())
		return
	}

	file, header, err := req.FormFile("file")
	if err != nil {
		respondProblem(w, 400, "Invalid Request", "missing 'file' field in form")
		return
	}
	defer file.Close()

	name := header.Filename
	if name == "" {
		respondProblem(w, 400, "Invalid Request", "filename is required")
		return
	}
	if !strings.HasSuffix(strings.ToLower(name), ".jsonl") {
		respondProblem(w, 400, "Invalid Request", "file must have .jsonl extension")
		return
	}

	if err := os.MkdirAll(r.datasetsDir, 0o755); err != nil {
		respondProblem(w, 500, "Internal Server Error", "failed to create datasets directory")
		return
	}

	destPath := filepath.Join(r.datasetsDir, filepath.Base(name))
	dest, err := os.Create(destPath)
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", "failed to create file: "+err.Error())
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		os.Remove(destPath)
		respondProblem(w, 500, "Internal Server Error", "failed to write file: "+err.Error())
		return
	}

	questionCount, parseErr := countJSONLLines(destPath)
	if parseErr != nil {
		os.Remove(destPath)
		respondProblem(w, 400, "Invalid Dataset", "failed to parse JSONL: "+parseErr.Error())
		return
	}

	respondJSON(w, 201, DatasetEntry{
		Name:          name,
		Size:          written,
		QuestionCount: questionCount,
	})
}

func (r *DatasetRouter) listHandler(w http.ResponseWriter, req *http.Request) {
	entries, err := os.ReadDir(r.datasetsDir)
	if err != nil {
		if os.IsNotExist(err) {
			respondJSON(w, 200, map[string]any{"datasets": []DatasetEntry{}})
			return
		}
		respondProblem(w, 500, "Internal Server Error", "cannot list datasets")
		return
	}

	var datasets []DatasetEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		d := DatasetEntry{
			Name:      entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().Format(time.RFC3339),
		}
		if qc, err := countJSONLLines(filepath.Join(r.datasetsDir, entry.Name())); err == nil {
			d.QuestionCount = qc
		}
		datasets = append(datasets, d)
	}

	if datasets == nil {
		datasets = []DatasetEntry{}
	}
	respondJSON(w, 200, map[string]any{"datasets": datasets})
}

func (r *DatasetRouter) downloadHandler(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	path := filepath.Join(r.datasetsDir, filepath.Base(name))

	if _, err := os.Stat(path); os.IsNotExist(err) {
		respondProblem(w, 404, "Not Found", "dataset not found")
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	http.ServeFile(w, req, path)
}

func (r *DatasetRouter) deleteHandler(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	path := filepath.Join(r.datasetsDir, filepath.Base(name))

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			respondProblem(w, 404, "Not Found", "dataset not found")
			return
		}
		respondProblem(w, 500, "Internal Server Error", "failed to delete dataset")
		return
	}
	respondJSON(w, 200, map[string]string{"deleted": name})
}

func countJSONLLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 10<<20)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			return 0, fmt.Errorf("line %d: invalid JSON", count+1)
		}
		count++
	}
	return count, scanner.Err()
}
