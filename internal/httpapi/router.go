package httpapi

import (
	"net/http"
	"strings"
)

func NewRouter(h *JobsHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.CreateJob(w, r)
	})
	mux.HandleFunc("/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/jobs/")
		if id == "" || id == r.URL.Path {
			http.NotFound(w, r)
			return
		}
		h.GetJobByID(w, r)
	})
	return mux
}
