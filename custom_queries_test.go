package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCustomQueriesAndDefaultCompatibility(t *testing.T) {
	a, e := generateQueries("+12025550123")
	if e != nil {
		t.Fatal(e)
	}
	b, e := generateQueriesFromList("+12025550123", "")
	if e != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("defaults changed")
	}
	p := filepath.Join(t.TempDir(), "queries.json")
	os.WriteFile(p, []byte(`{"version":1,"queries":[{"category":"general","query":"site:example.com {number}","enabled":true}]}`), 0600)
	b, e = generateQueriesFromList("+12025550123", p)
	if e != nil || len(b) != 1 || b[0].Text != "site:example.com +12025550123" {
		t.Fatalf("%+v %v", b, e)
	}
}
