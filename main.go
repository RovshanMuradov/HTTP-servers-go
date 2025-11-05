package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

// middlewareMetricsInc is a middleware that increments the file server hit counter.
func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

// handlerMetrics handles the /api/metrics endpoint.
func (cfg *apiConfig) handlerMetrics(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	html := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	w.Write([]byte(html))
}

// handlerReset handles the /api/reset endpoint.
func (cfg *apiConfig) handlerReset(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

// handleHealth handles the /api/healthz endpoint.
// func (cfg *apiConfig) handleHealth(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
// 	w.WriteHeader(http.StatusOK)
// 	w.Write([]byte("OK"))
// }

func main() {

	const filepathRoot = "."
	const port = "8080"

	mux := http.NewServeMux()
	apiCfg := &apiConfig{}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(
		http.StripPrefix("/app/", http.FileServer(http.Dir(filepathRoot))),
	))
	//mux.Handle("/app/assets/", apiCfg.middlewareMetricsInc(
	//	http.StripPrefix("/app/assets", http.FileServer(http.Dir("./assets"))),
	//))

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	// mux.HandleFunc("GET /healthz", apiCfg.handleHealth)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving files from %s on port: %s\n", filepathRoot, port)
	log.Fatal(srv.ListenAndServe())
}
