package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/rovshanmuradov/HTTP-servers-go/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	DB             *database.Queries
	Platform       string
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

	if cfg.Platform != "dev" {
		respondWithError(w, 403, "Not dev")
		return
	}
	err := cfg.DB.DeleteAllUsers(r.Context())
	if err != nil {
		respondWithError(w, 400, "Failed to delete users")
		return
	}

	cfg.fileserverHits.Store(0)
	w.WriteHeader(http.StatusOK)
}

func (cfg *apiConfig) handlerValidateChirp(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}
	type returnVals struct {
		CleanedBody string `json:"cleaned_body"`
	}
	type errorVals struct {
		Error string `json:"error"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}

	const MaxChirpLenght = 140
	if len(params.Body) > MaxChirpLenght {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	profane := []string{"kerfuffle", "sharbert", "fornax"}

	splitedWords := strings.Split(params.Body, " ")
	for index, word := range splitedWords {
		for _, j := range profane {
			if strings.ToLower(word) == j {
				splitedWords[index] = "****"
				break
			}
		}
	}
	cleanedText := strings.Join(splitedWords, " ")

	respondWithJSON(w, 200, returnVals{CleanedBody: cleanedText})
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorVals struct {
		Error string `json:"error"`
	}
	respondWithJSON(w, code, errorVals{Error: msg})

}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	dat, _ := json.Marshal(payload)
	w.WriteHeader(code)
	w.Write(dat)
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

func (cfg *apiConfig) createUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	user, err := cfg.DB.CreateUser(r.Context(), sql.NullString{String: params.Email, Valid: true})
	if err != nil {
		respondWithError(w, 400, "Failed to create user")
		return
	}
	respondWithJSON(w, 201, User{ID: user.ID.UUID, CreatedAt: user.CreatedAt.Time, UpdatedAt: user.UpdatedAt.Time, Email: user.Email.String})

}

func main() {

	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}
	platform := os.Getenv("PLATFORM")
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	const filepathRoot = "."
	const port = "8080"

	mux := http.NewServeMux()
	apiCfg := &apiConfig{
		DB:       database.New(db),
		Platform: platform,
	}

	mux.Handle("/app/", apiCfg.middlewareMetricsInc(
		http.StripPrefix("/app/", http.FileServer(http.Dir(filepathRoot))),
	))

	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerMetrics)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerReset)
	mux.HandleFunc("POST /api/validate_chirp", apiCfg.handlerValidateChirp)
	mux.HandleFunc("POST /api/users", apiCfg.createUser)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving files from %s on port: %s\n", filepathRoot, port)
	log.Fatal(srv.ListenAndServe())
}
