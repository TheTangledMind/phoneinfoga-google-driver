package searchlist

import (
	"strings"
	"testing"
)

func TestExpandAndValidate(t *testing.T) {
	q, e := Parse(strings.NewReader(`{"version":1,"queries":[{"category":"reputation","query":"site:example.com \"{number}\"","enabled":true},{"category":"general","query":"{national}","enabled":false}]}`), "+12025550123", "2025550123")
	if e != nil || len(q) != 1 || !strings.Contains(q[0].Text, "+12025550123") {
		t.Fatalf("%v %v", q, e)
	}
	for _, s := range []string{`{}`, `{"version":2,"queries":[]}`, `{"version":1,"queries":[{"category":"bad","query":"{number}","enabled":true}]}`, `{"version":1,"queries":[{"category":"general","query":"{secret}","enabled":true}]}`} {
		if _, e := Parse(strings.NewReader(s), "+12025550123", "2025550123"); e == nil {
			t.Fatal("accepted", s)
		}
	}
}
