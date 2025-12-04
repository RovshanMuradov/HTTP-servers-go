package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rovshanmuradov/HTTP-servers-go/internal/auth"
	"github.com/rovshanmuradov/HTTP-servers-go/internal/database"
)

type User struct {
	ID            uuid.UUID `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Email         string    `json:"email"`
	Token         string    `json:"token"`
	Refresh_token string    `json:"refresh_token"`
}

func (cfg *apiConfig) handlerUsersCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		User
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}
	hashPass, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password", err)
		return
	}

	user, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{Email: params.Email, HashedPassword: hashPass})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user", err)
		return
	}

	respondWithJSON(w, http.StatusCreated, response{
		User: User{
			ID:        user.ID,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email:     user.Email,
		},
	})
}

func (cfg *apiConfig) handlerUsersLogin(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		User
	}
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}

	user, err := cfg.db.GetUserByEmail(r.Context(), params.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password", nil)
		return
	}

	match, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil || !match {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password", nil)
		return
	}

	expiresIn := time.Hour
	createJWT, err := auth.MakeJWT(user.ID, cfg.jwtSecret, time.Duration(expiresIn))
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't create jwt", nil)
		return
	}

	refreshToken, err := auth.MakeRefreshToken()
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't create refresh token", nil)
		return
	}
	refreshExpiresAt := time.Now().Add(60 * 24 * time.Hour) // 60 дней
	createRefreshToken, err := cfg.db.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:     refreshToken,
		UserID:    user.ID,
		ExpiresAt: refreshExpiresAt,
	})
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't create refresh token", nil)
		return
	}

	respondWithJSON(w, http.StatusOK, response{
		User: User{
			ID:            user.ID,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
			Email:         user.Email,
			Token:         createJWT,
			Refresh_token: createRefreshToken.Token,
		},
	})

}

func (cfg *apiConfig) handlerRefreshCreate(w http.ResponseWriter, r *http.Request) {
	// 1. Extract token from the Header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", nil)
		return
	}
	// 2. Find in DB
	refreshToken, err := cfg.db.GetUserFromRefreshToken(r.Context(), token)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", nil)
		return
	}
	// 3. Check expires_at
	// 401 "token expired"
	if time.Now().After(refreshToken.ExpiresAt) {
		respondWithError(w, http.StatusUnauthorized, "Token has expired", nil)
		return
	}
	// 4. Check revoked_at
	// 401 "token revoked
	if refreshToken.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "Token has been revoked", nil)
		return
	}
	// 5. Create new JWT
	accessToken, err := auth.MakeJWT(refreshToken.UserID, cfg.jwtSecret, time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create JWT", nil)
		return
	}
	// 6. Return JSON
	type response struct {
		Token string `json:"token"`
	}
	respondWithJSON(w, 200, response{Token: accessToken})

}

func (cfg *apiConfig) handlerRevokeCreate(w http.ResponseWriter, r *http.Request) {
	// 1. Extract token from the Header
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", nil)
		return
	}
	// 2. Update DB
	err = cfg.db.RevokeRefreshToken(r.Context(), token)
	if err != nil && err != sql.ErrNoRows {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get info from DB", nil)
		return
	}
	w.WriteHeader(204)
}

func (cfg *apiConfig) handlerUsersUpdate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	type response struct {
		User
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
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters", err)
		return
	}
	hashNewPass, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash password", err)
		return
	}
	updateUser, err := cfg.db.UpdateUser(r.Context(), database.UpdateUserParams{
		ID:             userID,
		Email:          params.Email,
		HashedPassword: hashNewPass,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update user", err)
		return
	}
	respondWithJSON(w, 200, response{
		User: User{
			ID:        updateUser.ID,
			CreatedAt: updateUser.CreatedAt,
			UpdatedAt: updateUser.UpdatedAt,
			Email:     updateUser.Email,
		},
	})
}

func (cfg *apiConfig) handlerChirpDelete(w http.ResponseWriter, r *http.Request) {
	// 1. Аутентификация
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", nil)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Invalid token", err)
		return
	}

	// 2. Parse chirpID из URL
	chirpID := r.PathValue("chirpID")
	chirpUUID, err := uuid.Parse(chirpID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid chirp ID", err)
		return
	}

	// 3. Проверить существование и владельца
	chirp, err := cfg.db.GetChirpByID(r.Context(), chirpUUID)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Chirp not found", nil)
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err)
		return
	}

	// 4. Проверить владельца
	if chirp.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't own this chirp", nil)
		return
	}

	// 5. Удалить
	err = cfg.db.DeleteChirp(r.Context(), chirpUUID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't delete chirp", err)
		return
	}

	// 6. Успех
	w.WriteHeader(http.StatusNoContent)
}
