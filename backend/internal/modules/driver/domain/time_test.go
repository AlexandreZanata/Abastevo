package domain

import (
	"os"
	"strings"
	"testing"
)

func TestActualOnlineUnionVectors(t *testing.T) {
	raw, e := os.ReadFile("../../../../../contracts/testdata/driver-suite/driver-manual-reference-v1.psv")
	if e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n")[1:] {
		p := strings.Split(line, "|")
		if p[1] != "UNION" {
			continue
		}
		count++
		t.Run(p[0], func(t *testing.T) {
			var spans []TimeSpan
			for _, s := range strings.Split(fields(p[2])["intervals"], ",") {
				a, b, _ := strings.Cut(s, ":")
				spans = append(spans, TimeSpan{number(a), number(b)})
			}
			got, e := OnlineUnion(spans)
			if e != nil || got != number(fields(p[3])["union"]) {
				t.Fatal(got, e)
			}
		})
	}
	if count != 3 {
		t.Fatal(count)
	}
	if _, e = OnlineUnion([]TimeSpan{{2, 1}}); e == nil {
		t.Fatal("reversed time accepted")
	}
}
