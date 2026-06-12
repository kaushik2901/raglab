package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	qstore "github.com/kaushik2901/gitlab-handbook-rag-pipeline/internal/store"
)

type IndexRouter struct {
	store qstore.VectorStore
}

func NewIndexRouter(store qstore.VectorStore) *IndexRouter {
	return &IndexRouter{store: store}
}

func (r *IndexRouter) Register(mux chi.Router) {
	mux.Get("/indexes", r.listHandler)
	mux.Get("/indexes/{name}", r.getHandler)
	mux.Delete("/indexes/{name}", r.deleteHandler)
}

func (r *IndexRouter) listHandler(w http.ResponseWriter, req *http.Request) {
	collections, err := r.store.ListCollections(req.Context())
	if err != nil {
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	if collections == nil {
		collections = []qstore.CollectionInfo{}
	}
	respondJSON(w, 200, collections)
}

func (r *IndexRouter) getHandler(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	info, err := r.store.GetCollection(req.Context(), name)
	if err != nil {
		if errors.Is(err, qstore.ErrCollectionNotFound) {
			respondProblem(w, 404, "Not Found", err.Error())
			return
		}
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, info)
}

func (r *IndexRouter) deleteHandler(w http.ResponseWriter, req *http.Request) {
	name := chi.URLParam(req, "name")
	if err := r.store.DeleteCollection(req.Context(), name); err != nil {
		if errors.Is(err, qstore.ErrCollectionNotFound) {
			respondProblem(w, 404, "Not Found", err.Error())
			return
		}
		respondProblem(w, 500, "Internal Server Error", err.Error())
		return
	}
	respondJSON(w, 200, map[string]string{"deleted": name})
}
