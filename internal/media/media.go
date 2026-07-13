// Package media handles simple image uploads: type sniffing, size limits,
// unique filename generation, and safe path resolution under a media root.
// No DB involvement — images live on disk and are referenced by URL in
// markdown bodies.
package media

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxBytes is the maximum accepted image payload (5 MiB). Simple paste/drag
// upload for personal use; optimisation/thumbnails are out of scope for M4.
const MaxBytes = 5 << 20

var (
	ErrEmpty           = errors.New("empty image")
	ErrTooLarge        = errors.New("image too large")
	ErrUnsupportedType = errors.New("unsupported image type")
	ErrUnsafeName      = errors.New("unsafe media name")
	ErrNotFound        = errors.New("media not found")
)

// Result is returned by Save: Name is the on-disk basename; URL is the
// public path clients embed in markdown (e.g. /media/abc.png).
type Result struct {
	Name string
	URL  string
}

// DetectExt sniffs data and returns a canonical extension for allowed image
// types (.png / .jpg / .gif / .webp), or an error for empty/unsupported input.
func DetectExt(data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrEmpty
	}
	// http.DetectContentType only looks at the first 512 bytes.
	ct := http.DetectContentType(data)
	switch ct {
	case "image/png":
		return ".png", nil
	case "image/jpeg":
		return ".jpg", nil
	case "image/gif":
		return ".gif", nil
	case "image/webp":
		return ".webp", nil
	default:
		// DetectContentType often returns application/octet-stream for WebP
		// on short buffers; fall back to a manual RIFF/WEBP check.
		if isWebP(data) {
			return ".webp", nil
		}
		return "", ErrUnsupportedType
	}
}

func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		string(data[0:4]) == "RIFF" &&
		string(data[8:12]) == "WEBP"
}

// Save validates data, writes it under dir with a unique name, and returns
// the public URL. dir is created if missing.
func Save(dir string, data []byte) (Result, error) {
	if len(data) == 0 {
		return Result{}, ErrEmpty
	}
	if len(data) > MaxBytes {
		return Result{}, ErrTooLarge
	}
	ext, err := DetectExt(data)
	if err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}
	name, err := uniqueName(ext)
	if err != nil {
		return Result{}, err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Result{}, err
	}
	return Result{Name: name, URL: "/media/" + name}, nil
}

// uniqueName returns <16 hex bytes><ext>, collision-resistant for personal use.
func uniqueName(ext string) (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate media name: %w", err)
	}
	return hex.EncodeToString(b[:]) + ext, nil
}

// SafeName reports whether name is a single path segment safe to serve
// (no separators, no "..", non-empty, only alnum/dot/dash/underscore).
func SafeName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if filepath.Base(name) != name {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// Open opens a file under dir by basename. Rejects unsafe names and missing files.
func Open(dir, name string) (*os.File, error) {
	if !SafeName(name) {
		return nil, ErrUnsafeName
	}
	path := filepath.Join(dir, name)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return f, nil
}

// ContentType maps a filename extension to a Content-Type for serving.
func ContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
