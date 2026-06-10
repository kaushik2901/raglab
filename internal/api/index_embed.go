package api

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var indexContent string

func indexHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(indexContent))
}
