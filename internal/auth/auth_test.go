package auth_test

import (
	"errors"
	"testing"
	"time"

	"github.com/sam33339999/wikibuild/internal/auth"
	"github.com/sam33339999/wikibuild/internal/clock"
	"github.com/stretchr/testify/require"
)

func TestPassword_HashAndCompare(t *testing.T) {
	h := auth.NewPasswordHasher()

	hash, err := h.Hash("correct horse battery staple")
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	require.NoError(t, h.Compare(hash, "correct horse battery staple"))
}

func TestPassword_CompareWrongPassword(t *testing.T) {
	h := auth.NewPasswordHasher()
	hash, err := h.Hash("right")
	require.NoError(t, err)

	err = h.Compare(hash, "wrong")
	require.ErrorIs(t, err, auth.ErrPasswordMismatch)
}

func TestPassword_HashIsSalted(t *testing.T) {
	h := auth.NewPasswordHasher()
	h1, _ := h.Hash("same")
	h2, _ := h.Hash("same")
	require.NotEqual(t, h1, h2, "bcrypt must salt")
}

func TestSigner_SignAndVerify_RoundTrip(t *testing.T) {
	s := auth.NewSigner("supersecretkey1234", clock.NewFake(time.Unix(1_700_000_000, 0)))

	tok, err := s.Sign("admin", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, tok)

	payload, err := s.Verify(tok)
	require.NoError(t, err)
	require.Equal(t, "admin", payload)
}

func TestSigner_VerifyTamperedToken(t *testing.T) {
	s := auth.NewSigner("supersecretkey1234", clock.NewFake(time.Unix(1_700_000_000, 0)))
	tok, _ := s.Sign("admin", time.Hour)

	tampered := tok[:len(tok)-2] + "xx"
	_, err := s.Verify(tampered)
	require.Error(t, err)
}

func TestSigner_VerifyExpiredToken(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	s := auth.NewSigner("supersecretkey1234", fc)

	tok, _ := s.Sign("admin", time.Hour)

	fc.Set(time.Unix(1_700_000_000, 0).Add(2 * time.Hour))
	_, err := s.Verify(tok)
	require.ErrorIs(t, err, auth.ErrTokenExpired)
}

func TestSigner_VerifyWrongSecret(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	signer := auth.NewSigner("supersecretkey1234", fc)
	verifier := auth.NewSigner("completelydifferentkey", fc)

	tok, _ := signer.Sign("admin", time.Hour)
	_, err := verifier.Verify(tok)
	require.Error(t, err)
	require.False(t, errors.Is(err, auth.ErrTokenExpired), "must be signature failure, not expiry")
}

func TestSigner_MalformedToken(t *testing.T) {
	s := auth.NewSigner("supersecretkey1234", clock.NewFake(time.Unix(1_700_000_000, 0)))
	_, err := s.Verify("not-a-valid-token")
	require.Error(t, err)
}
