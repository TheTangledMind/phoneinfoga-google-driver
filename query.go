package main

import (
	"errors"
	"regexp"

	"github.com/sundowndev/phoneinfoga/v2/lib/number"
	"github.com/sundowndev/phoneinfoga/v2/lib/remote"
)

const phoneinfogaVersion = "2.11.0"

type Query struct {
	Category string `json:"category"`
	Text     string `json:"query"`
	URL      string `json:"search_url"`
}

// This generator is local-only. Do not initialize the other remote scanners.
func generateQueries(input string) ([]Query, error) {
	if len(input) > 40 || !regexp.MustCompile(`^\+[0-9 ()-]+$`).MatchString(input) {
		return nil, errors.New("provide an international number beginning with +")
	}
	n, err := number.NewNumber(input)
	if err != nil || n == nil || !n.Valid {
		return nil, errors.New("number is not valid under PhoneInfoga 2.11.0 metadata")
	}
	raw, err := remote.NewGoogleSearchScanner().Run(*n, remote.ScannerOptions{})
	if err != nil {
		return nil, errors.New("PhoneInfoga query generation failed")
	}
	res, ok := raw.(remote.GoogleSearchResponse)
	if !ok {
		return nil, errors.New("unexpected PhoneInfoga query response")
	}
	groups := []struct {
		name  string
		dorks []*remote.GoogleSearchDork
	}{
		{"general", res.General}, {"individuals", res.Individuals}, {"reputation", res.Reputation},
		{"social_media", res.SocialMedia}, {"disposable_providers", res.DisposableProviders},
	}
	var queries []Query
	for _, group := range groups {
		for _, d := range group.dorks {
			queries = append(queries, Query{Category: group.name, Text: d.Dork, URL: d.URL})
		}
	}
	return queries, nil
}
