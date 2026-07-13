package render

import (
	"math"
	"strings"
	"unicode"
)

// Reading-speed assumptions (words/chars per minute). CJK text is read by
// character; Latin by word.
const (
	cjkCharsPerMinute   = 500
	latinWordsPerMinute = 200
)

// ReadingTime estimates the reading time in minutes for a markdown body. CJK
// characters count at cjkCharsPerMinute; Latin words at latinWordsPerMinute;
// the two are summed and rounded up. Returns 0 for empty/whitespace-only
// input, otherwise at least 1.
func ReadingTime(body string) int {
	if strings.TrimSpace(body) == "" {
		return 0
	}

	cjk := 0
	latinWords := 0
	inWord := false
	for _, r := range body {
		if unicode.Is(unicode.Han, r) {
			cjk++
			inWord = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if !inWord {
				latinWords++
				inWord = true
			}
			continue
		}
		inWord = false
	}

	minutes := math.Ceil(float64(cjk)/cjkCharsPerMinute + float64(latinWords)/latinWordsPerMinute)
	if minutes < 1 {
		minutes = 1
	}
	return int(minutes)
}
