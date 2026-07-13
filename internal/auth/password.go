// Package auth provides password hashing (bcrypt) and HMAC-signed session
// tokens for admin authentication and protected-article unlock cookies.
//
// PasswordHasher is an interface so handler/logic tests can substitute a
// fast fake; the bcrypt implementation is the production default.
package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordMismatch is returned when a plaintext password does not match
// the stored hash (or the hash is malformed).
var ErrPasswordMismatch = errors.New("password mismatch")

// PasswordHasher hashes plaintext passwords and verifies them.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// bcryptHasher is the production PasswordHasher. The bcrypt cost is fixed at
// the library default (10), which is adequate for a single-admin blog and
// avoids config surface for now.
type bcryptHasher struct{}

// NewPasswordHasher returns the bcrypt-backed PasswordHasher.
func NewPasswordHasher() PasswordHasher { return bcryptHasher{} }

func (bcryptHasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (bcryptHasher) Compare(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrPasswordMismatch
	}
	return nil
}
