package auth

import (
	"github.com/alexedwards/argon2id"
)

func HashPassword(password string) (string, error) {
	hash, _ := argon2id.CreateHash(password, argon2id.DefaultParams)
	return hash, nil
}
func CheckPasswordHash(password, hash string) (bool, error) {
	match, _ := argon2id.ComparePasswordAndHash(password, hash)
	return match, nil
}
