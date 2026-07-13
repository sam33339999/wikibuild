// Package public holds templ components for the reader-facing site.
package public

import "time"

// formatDate renders a nullable publish time as YYYY-MM-DD, or "" if unset.
func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
