package handler

import (
	"crypto/rand"
	"encoding/hex"
)

// newPreviewToken returns a 32-char hex token for unlisted draft previews.
func newPreviewToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
