//go:build ignore

// Generate static/css/chroma.css (light github + dark monokai token colors).
// Usage from repo root: go run ./scripts/gen-chroma-css.go
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
)

func main() {
	f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.ClassPrefix("ch-"))
	light := writeStyle(f, "github")
	dark := writeStyle(f, "monokai")

	reBG := regexp.MustCompile(`background-color:\s*#[0-9a-fA-F]+`)
	light = reBG.ReplaceAllString(light, "background-color: transparent")
	dark = reBG.ReplaceAllString(dark, "background-color: transparent")

	var b strings.Builder
	b.WriteString(`/* Generated chroma token colors for WikiBuild (class prefix ch-).
 * Light tokens: github · Dark tokens: monokai.
 * Block background uses site --code-bg.
 * Regenerate from repo root: go run ./scripts/gen-chroma-css.go
 */
.content pre.ch-chroma,
.content .ch-chroma {
  color: var(--code-fg);
  background: var(--code-bg) !important;
}
.content pre.ch-chroma code,
.content .ch-chroma code {
  background: transparent !important;
  color: inherit;
  padding: 0 !important;
  font-size: inherit;
}

`)
	b.WriteString("/* ---- Light ---- */\n")
	b.WriteString(scopeCSS(light, []string{
		`html:not([data-theme="dark"])`,
		`html[data-theme="light"]`,
	}))
	b.WriteString("\n/* ---- Dark (explicit) ---- */\n")
	b.WriteString(scopeCSS(dark, []string{
		`html[data-theme="dark"]`,
	}))
	b.WriteString("\n/* ---- Dark (system, when theme not forced light) ---- */\n")
	b.WriteString("@media (prefers-color-scheme: dark) {\n")
	b.WriteString(scopeCSS(dark, []string{
		`html:not([data-theme="light"])`,
	}))
	b.WriteString("}\n")

	out := filepath.Join("static", "css", "chroma.css")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", out, "bytes", b.Len())
}

func writeStyle(f *chromahtml.Formatter, name string) string {
	var buf bytes.Buffer
	st := styles.Get(name)
	if st == nil {
		panic("missing style " + name)
	}
	if err := f.WriteCSS(&buf, st); err != nil {
		panic(err)
	}
	return buf.String()
}

func scopeCSS(css string, scopes []string) string {
	var out strings.Builder
	for _, line := range strings.Split(css, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" {
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trim, "/*") && !strings.Contains(trim, "{") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		if !strings.Contains(line, "{") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		selPart := line
		if i := strings.Index(line, "*/"); i >= 0 && strings.Contains(line[:i+2], "/*") {
			selPart = strings.TrimSpace(line[i+2:])
		}
		parts := strings.SplitN(selPart, "{", 2)
		if len(parts) != 2 {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}
		sel := strings.TrimSpace(parts[0])
		rest := "{" + parts[1]
		for _, sc := range scopes {
			fmt.Fprintf(&out, "%s %s %s\n", sc, sel, rest)
		}
	}
	return out.String()
}
