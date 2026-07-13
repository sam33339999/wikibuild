package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CatalogEntry is one site article for related-link suggestions (no body).
type CatalogEntry struct {
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	Tags    []string `json:"tags,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

// RelatedSuggestion is one recommended wikilink target.
type RelatedSuggestion struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// BuildRelatedMessages builds prompts for related-article suggestions.
func BuildRelatedMessages(selection string, catalog []CatalogEntry) []Message {
	system := strings.TrimSpace(`You help an author link related notes in a personal wiki.
Given the author's current selection (or paragraph) and a catalog of site articles, return a JSON object only:
{"suggestions":[{"slug":"...","title":"...","reason":"short reason"}, ...]}

Rules:
- Only suggest slugs that appear in the catalog.
- Return 0–5 suggestions, most relevant first.
- reason is one short phrase (prefer Traditional Chinese if selection is Chinese).
- If nothing is relevant, return {"suggestions":[]}.
- No markdown fences, no commentary.`)

	var cat strings.Builder
	for _, e := range catalog {
		cat.WriteString("- slug=")
		cat.WriteString(e.Slug)
		cat.WriteString(" | title=")
		cat.WriteString(e.Title)
		if len(e.Tags) > 0 {
			cat.WriteString(" | tags=")
			cat.WriteString(strings.Join(e.Tags, ","))
		}
		if s := strings.TrimSpace(e.Summary); s != "" {
			cat.WriteString(" | summary=")
			cat.WriteString(s)
		}
		cat.WriteByte('\n')
	}
	user := fmt.Sprintf("SELECTION:\n%s\n\nCATALOG:\n%s", strings.TrimSpace(selection), cat.String())
	return []Message{
		{Role: "system", Content: system},
		{Role: "user", Content: user},
	}
}

// ParseRelatedResult extracts suggestions from model content.
func ParseRelatedResult(content string) ([]RelatedSuggestion, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: empty content", ErrBadResponse)
	}
	if s, ok := extractFencedJSON(content); ok {
		content = s
	} else if s, ok := extractFirstObject(content); ok {
		content = s
	}
	var raw struct {
		Suggestions []RelatedSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadResponse, err)
	}
	if raw.Suggestions == nil {
		return nil, fmt.Errorf("%w: missing suggestions", ErrBadResponse)
	}
	out := make([]RelatedSuggestion, 0, len(raw.Suggestions))
	for _, s := range raw.Suggestions {
		slug := strings.TrimSpace(s.Slug)
		if slug == "" {
			continue
		}
		out = append(out, RelatedSuggestion{
			Slug:   slug,
			Title:  strings.TrimSpace(s.Title),
			Reason: strings.TrimSpace(s.Reason),
		})
	}
	return out, nil
}
