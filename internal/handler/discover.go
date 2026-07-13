package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	publicviews "github.com/sam33339999/wikibuild/views/public"
)

// Search renders the public search form and (when q is non-empty) results
// limited to published + public articles. Uses the store's ILIKE/title-body
// Search filter (same as admin).
func (h *Public) Search(c fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	var items []model.Article
	if q != "" {
		var err error
		items, _, err = h.repo.ListArticles(c.Context(), store.ListQuery{
			Status:     model.StatusPublished,
			Visibility: model.VisibilityPublic,
			Search:     q,
		})
		if err != nil {
			return err
		}
	}
	return renderPage(c, "搜尋", publicviews.Search(q, items))
}

// Tag lists published public articles carrying the given tag.
func (h *Public) Tag(c fiber.Ctx) error {
	tag := strings.TrimSpace(c.Params("tag"))
	if tag == "" {
		return c.SendStatus(http.StatusBadRequest)
	}
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
		Tag:        tag,
	})
	if err != nil {
		return err
	}
	return renderPage(c, "標籤："+tag, publicviews.Tag(tag, items))
}

// ArchiveIndex lists year/month buckets that have at least one published
// public article, newest first.
func (h *Public) ArchiveIndex(c fiber.Ctx) error {
	items, err := h.publicPublished(c)
	if err != nil {
		return err
	}
	months := groupByMonth(items)
	return renderPage(c, "封存", publicviews.ArchiveIndex(months))
}

// ArchiveYear lists all published public articles in the given year
// (newest first within the year).
func (h *Public) ArchiveYear(c fiber.Ctx) error {
	year, ok := parseYear(c.Params("year"))
	if !ok {
		return c.SendStatus(http.StatusBadRequest)
	}
	items, err := h.publicPublished(c)
	if err != nil {
		return err
	}
	var out []model.Article
	for _, a := range items {
		if articleDate(a).Year() == year {
			out = append(out, a)
		}
	}
	return renderPage(c, strconv.Itoa(year)+" 年", publicviews.ArchivePeriod(strconv.Itoa(year), out))
}

// ArchiveMonth lists published public articles in year/month.
func (h *Public) ArchiveMonth(c fiber.Ctx) error {
	year, ok := parseYear(c.Params("year"))
	if !ok {
		return c.SendStatus(http.StatusBadRequest)
	}
	month, ok := parseMonth(c.Params("month"))
	if !ok {
		return c.SendStatus(http.StatusBadRequest)
	}
	items, err := h.publicPublished(c)
	if err != nil {
		return err
	}
	var out []model.Article
	for _, a := range items {
		d := articleDate(a)
		if d.Year() == year && int(d.Month()) == month {
			out = append(out, a)
		}
	}
	title := strconv.Itoa(year) + " 年 " + strconv.Itoa(month) + " 月"
	return renderPage(c, title, publicviews.ArchivePeriod(title, out))
}

// publicPublished loads every published + public article (no pagination).
// Suitable for personal-scale archives and discovery pages.
func (h *Public) publicPublished(c fiber.Ctx) ([]model.Article, error) {
	items, _, err := h.repo.ListArticles(c.Context(), store.ListQuery{
		Status:     model.StatusPublished,
		Visibility: model.VisibilityPublic,
	})
	return items, err
}

// articleDate is the date used for archives: PublishedAt when set, else CreatedAt.
func articleDate(a model.Article) time.Time {
	if a.PublishedAt != nil {
		return a.PublishedAt.UTC()
	}
	return a.CreatedAt.UTC()
}

func parseYear(s string) (int, bool) {
	y, err := strconv.Atoi(s)
	if err != nil || y < 1970 || y > 3000 {
		return 0, false
	}
	return y, true
}

func parseMonth(s string) (int, bool) {
	m, err := strconv.Atoi(s)
	if err != nil || m < 1 || m > 12 {
		return 0, false
	}
	return m, true
}

// groupByMonth aggregates articles into year-month buckets, newest first.
func groupByMonth(items []model.Article) []publicviews.ArchiveMonth {
	type key struct{ y, m int }
	counts := make(map[key]int)
	for _, a := range items {
		d := articleDate(a)
		if d.IsZero() {
			continue
		}
		counts[key{d.Year(), int(d.Month())}]++
	}
	out := make([]publicviews.ArchiveMonth, 0, len(counts))
	for k, n := range counts {
		out = append(out, publicviews.ArchiveMonth{
			Year:  k.y,
			Month: k.m,
			Count: n,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Year != out[j].Year {
			return out[i].Year > out[j].Year
		}
		return out[i].Month > out[j].Month
	})
	return out
}
