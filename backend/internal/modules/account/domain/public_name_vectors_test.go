package domain

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestPublicNameSharedVectors keeps the server rule and the Android
// pre-check (com.anpfuel.domain.account.PublicName) on one vector file.
func TestPublicNameSharedVectors(t *testing.T) {
	file, err := os.Open(filepath.Join(accountFixturesDir(t), "public-names-v1.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	cases := 0
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, "\t")
		if len(fields) != 3 {
			t.Fatalf("line %d: want 3 tab-separated fields, got %d", line, len(fields))
		}
		input, err := strconv.Unquote(`"` + fields[1] + `"`)
		if err != nil {
			t.Fatalf("line %d: input escape: %v", line, err)
		}
		cases++
		switch fields[0] {
		case "normalize":
			if got := NormalizePublicName(input); got != fields[2] {
				t.Errorf("line %d: NormalizePublicName(%q) = %q, want %q", line, input, got, fields[2])
			}
		case "valid":
			want, err := strconv.ParseBool(fields[2])
			if err != nil {
				t.Fatalf("line %d: expected must be true or false", line)
			}
			if got := ValidPublicName(input); got != want {
				t.Errorf("line %d: ValidPublicName(%q) = %v, want %v", line, input, got, want)
			}
		default:
			t.Fatalf("line %d: unknown kind %q", line, fields[0])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if cases < 30 {
		t.Fatalf("want at least 30 public-name vectors, got %d", cases)
	}
}
