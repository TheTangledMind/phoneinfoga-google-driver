// Package searchlist expands data-only Google search templates. No code is evaluated.
package searchlist

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"unicode"
)

type Query struct {
	Category string `json:"category"`
	Text     string `json:"query"`
	URL      string `json:"search_url"`
}
type Entry struct {
	Category string `json:"category"`
	Query    string `json:"query"`
	Enabled  bool   `json:"enabled"`
}
type Config struct {
	Version int     `json:"version"`
	Queries []Entry `json:"queries"`
}

func Load(path, number, national string) ([]Query, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("cannot open search list")
	}
	defer f.Close()
	return Parse(f, number, national)
}
func Parse(r io.Reader, number, national string) ([]Query, error) {
	b, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil || len(b) > 65536 {
		return nil, errors.New("search list exceeds 64 KiB or cannot be read")
	}
	var c Config
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || c.Version != 1 || len(c.Queries) > 100 {
		return nil, errors.New("invalid search list: require version 1 and at most 100 entries")
	}
	var extra interface{}
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("unexpected trailing search-list data")
	}
	out := []Query{}
	seen := map[string]bool{}
	for _, q := range c.Queries {
		switch q.Category {
		case "general", "individuals", "reputation", "social_media", "disposable_providers":
		default:
			return nil, errors.New("unknown search category")
		}
		if len(q.Query) > 2048 || !strings.Contains(q.Query, "{number}") && !strings.Contains(q.Query, "{national}") {
			return nil, errors.New("query must contain {number} or {national} and be at most 2048 bytes")
		}
		text := strings.NewReplacer("{number}", number, "{national}", national).Replace(q.Query)
		if strings.ContainsAny(text, "{}") || strings.IndexFunc(text, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
			return nil, errors.New("invalid placeholder or control character in query")
		}
		key := q.Category + "\x00" + text
		if !q.Enabled || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Query{q.Category, text, "https://www.google.com/search?" + url.Values{"q": {text}}.Encode()})
	}
	return out, nil
}
