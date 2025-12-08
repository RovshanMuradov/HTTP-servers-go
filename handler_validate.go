package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rovshanmuradov/HTTP-servers-go/internal/auth"
	"github.com/rovshanmuradov/HTTP-servers-go/internal/database"
)

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func (cfg *apiConfig) handlerChirpsCreate(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Body string `json:"body"`
	}
	type response struct {
		Chirp
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}

	const maxChirpLength = 140
	if len(params.Body) > maxChirpLength {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long", nil)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", err)
		return
	}

	badWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}
	cleaned := getCleanedBody(params.Body, badWords)

	chirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   cleaned,
		UserID: userID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, response{
		Chirp: Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		},
	})

}

func (cfg *apiConfig) handlerChirpsGet(w http.ResponseWriter, r *http.Request) {
	authorID := r.URL.Query().Get("author_id")
	sortOrder := r.URL.Query().Get("sort")

	// Default sort order
	if sortOrder == "" {
		sortOrder = "asc"
	}

	var chirps []database.Chirp
	var err error

	if authorID != "" {
		authorUUID, err := uuid.Parse(authorID)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "Invalid author_id", err)
			return
		}
		chirps, err = cfg.db.GetChirpsByAuthor(r.Context(), authorUUID)
	} else {
		chirps, err = cfg.db.GetAllChirps(r.Context())
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get chirps", err)
		return
	}

	// Convert to response format
	result := make([]Chirp, len(chirps))
	for i, dbChirp := range chirps {
		result[i] = Chirp{
			ID:        dbChirp.ID,
			CreatedAt: dbChirp.CreatedAt,
			UpdatedAt: dbChirp.UpdatedAt,
			Body:      dbChirp.Body,
			UserID:    dbChirp.UserID,
		}
	}

	// Sort in-memory
	sort.Slice(result, func(i, j int) bool {
		if sortOrder == "desc" {
			return result[i].CreatedAt.After(result[j].CreatedAt)
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	respondWithJSON(w, 200, result)
}

func (cfg *apiConfig) handlerChirpGetId(w http.ResponseWriter, r *http.Request) {
	chirpID := r.PathValue("chirpID")

	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Invalid chirp ID", err)
		return
	}

	chirps, err := cfg.db.GetChirpByID(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get chirp ID", err)
		return
	}

	respondWithJSON(w, 200, Chirp{
		ID:        chirps.ID,
		CreatedAt: chirps.CreatedAt,
		UpdatedAt: chirps.UpdatedAt,
		Body:      chirps.Body,
		UserID:    chirps.UserID,
	})
}

func getCleanedBody(body string, badWords map[string]struct{}) string {
	words := strings.Split(body, " ")
	for i, word := range words {
		loweredWord := strings.ToLower(word)
		if _, ok := badWords[loweredWord]; ok {
			words[i] = "****"
		}
	}
	cleaned := strings.Join(words, " ")
	return cleaned
}
