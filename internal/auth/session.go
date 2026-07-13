package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sam33339999/wikibuild/internal/clock"
)

// Token / session errors.
var (
	ErrTokenMalformed = errors.New("malformed token")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenInvalid   = errors.New("invalid token signature")
)

// Signer produces and verifies HMAC-SHA256 signed tokens of the form
//
//	base64(payload).expiry.base64(hmac)
//
// where hmac = HMAC-SHA256(secret, payload + "." + expiry). Time is injected
// via clock.Clock so expiry is deterministic in tests.
type Signer struct {
	secret []byte
	clock  clock.Clock
}

// NewSigner builds a Signer. The secret must be at least 16 bytes; this is
// enforced by config validation for the app secret, but NewSigner trusts its
// caller.
func NewSigner(secret string, clk clock.Clock) *Signer {
	return &Signer{secret: []byte(secret), clock: clk}
}

// Sign returns a signed token carrying payload that expires after ttl from now.
func (s *Signer) Sign(payload string, ttl time.Duration) (string, error) {
	exp := s.clock.Now().Add(ttl).Unix()
	expStr := strconv.FormatInt(exp, 10)
	mac := s.mac(payload, expStr)
	return payload + "." + expStr + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

// Verify checks the signature and expiry and returns the payload on success.
func (s *Signer) Verify(token string) (string, error) {
	// Token is payload.exp.sig. Split from the right in case payload itself
	// contains '.', though our admin payload is a plain username.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrTokenMalformed
	}
	payload, expStr, sig := parts[0], parts[1], parts[2]

	wantMac, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return "", ErrTokenMalformed
	}
	if !hmac.Equal(wantMac, s.mac(payload, expStr)) {
		return "", ErrTokenInvalid
	}

	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", ErrTokenMalformed
	}
	if s.clock.Now().Unix() >= exp {
		return "", ErrTokenExpired
	}
	return payload, nil
}

func (s *Signer) mac(payload, expStr string) []byte {
	h := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(h, "%s.%s", payload, expStr)
	return h.Sum(nil)
}
