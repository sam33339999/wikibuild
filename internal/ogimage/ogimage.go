// Package ogimage renders simple branded Open Graph share images (PNG).
// No external font files — uses Go's bundled goregular.
package ogimage

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Standard OG / Twitter large card size.
const (
	Width  = 1200
	Height = 630
)

// Theme colors aligned with WikiBuild khaki reading UI (light).
var (
	bgColor    = color.RGBA{R: 0xf4, G: 0xf1, B: 0xe8, A: 0xff} // warm paper
	bandColor  = color.RGBA{R: 0xc4, G: 0xb5, B: 0x8a, A: 0xff} // khaki accent
	titleColor = color.RGBA{R: 0x2a, G: 0x29, B: 0x26, A: 0xff}
	mutedColor = color.RGBA{R: 0x6b, G: 0x66, B: 0x5c, A: 0xff}
)

// Render draws a 1200×630 PNG with site name and title.
func Render(title, siteName string) ([]byte, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Untitled"
	}
	siteName = strings.TrimSpace(siteName)
	if siteName == "" {
		siteName = "WikiBuild"
	}

	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: bgColor}, image.Point{}, draw.Src)
	// Top accent band.
	draw.Draw(img, image.Rect(0, 0, Width, 18), &image.Uniform{C: bandColor}, image.Point{}, draw.Src)
	// Bottom band.
	draw.Draw(img, image.Rect(0, Height-12, Width, Height), &image.Uniform{C: bandColor}, image.Point{}, draw.Src)

	faceTitle, err := parseFace(48)
	if err != nil {
		return nil, err
	}
	faceSite, err := parseFace(28)
	if err != nil {
		return nil, err
	}
	defer faceTitle.Close()
	defer faceSite.Close()

	// Site name top-left.
	drawString(img, faceSite, mutedColor, 64, 90, siteName)

	// Title — wrap to ~2–3 lines.
	lines := wrapRunes(title, 22)
	if len(lines) > 4 {
		lines = lines[:4]
		// Ellipsis on last line if truncated.
		last := lines[3]
		if utf8.RuneCountInString(last) > 2 {
			r := []rune(last)
			lines[3] = string(r[:len(r)-1]) + "…"
		}
	}
	y := 220
	for _, line := range lines {
		drawString(img, faceTitle, titleColor, 64, y, line)
		y += 64
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseFace(size float64) (font.Face, error) {
	ft, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ft, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func drawString(img *image.RGBA, face font.Face, col color.Color, x, y int, s string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

// wrapRunes packs runes into lines of at most maxRunes (approx width for CJK/Latin mix).
func wrapRunes(s string, maxRunes int) []string {
	if maxRunes < 4 {
		maxRunes = 4
	}
	var lines []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		lines = append(lines, string(cur))
		cur = cur[:0]
	}
	for _, r := range s {
		if r == '\n' {
			flush()
			continue
		}
		cur = append(cur, r)
		if len(cur) >= maxRunes {
			flush()
		}
	}
	flush()
	if len(lines) == 0 {
		return []string{s}
	}
	return lines
}
