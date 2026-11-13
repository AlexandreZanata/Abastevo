package protocol_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	sync "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func fields(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, part := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || key == "" {
			t.Fatal("bad vector field")
		}
		if _, duplicate := out[key]; duplicate {
			t.Fatal("duplicate vector field")
		}
		out[key] = value
	}
	return out
}
func rawHex(t *testing.T, raw string) []byte {
	t.Helper()
	value, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal("bad vector encoding")
	}
	return value
}
func errorFields(t *testing.T, err error) map[string]string {
	t.Helper()
	switch {
	case errors.Is(err, sync.ErrInvalid):
		return map[string]string{"error": "INVALID"}
	case errors.Is(err, sync.ErrLimit):
		return map[string]string{"error": "LIMIT"}
	case errors.Is(err, sync.ErrBinding):
		return map[string]string{"error": "BINDING"}
	case errors.Is(err, sync.ErrDigest):
		return map[string]string{"error": "DIGEST"}
	case errors.Is(err, sync.ErrReused):
		return map[string]string{"error": "REUSED"}
	}
	t.Fatal("unknown protocol error")
	return nil
}
func vectors(t *testing.T, path string, count int, execute func(*testing.T, string, map[string]string) map[string]string) {
	t.Helper()
	f, err := os.Open("../../../../../contracts/testdata/enterprise/sync/" + path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("bad header")
	}
	seen := map[string]bool{}
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) != 4 || parts[0] == "" || seen[parts[0]] {
			t.Fatal("bad/duplicate vector")
		}
		seen[parts[0]] = true
		t.Run(parts[0], func(t *testing.T) {
			got := execute(t, parts[1], fields(t, parts[2]))
			want := fields(t, parts[3])
			if !reflect.DeepEqual(got, want) {
				t.Fatal("protocol oracle output differs")
			}
		})
	}
	if scanner.Err() != nil || len(seen) != count {
		t.Fatalf("rows=%d error=%v", len(seen), scanner.Err())
	}
}
func TestCanonicalVectors(t *testing.T) {
	vectors(t, "canonical-v1.psv", 58, func(t *testing.T, operation string, args map[string]string) map[string]string {
		if operation != "CANONICAL" {
			t.Fatal("unknown operation")
		}
		canonical, err := sync.CanonicalJSON(rawHex(t, args["raw_hex"]))
		if err != nil {
			return errorFields(t, err)
		}
		digest := sha256.Sum256(canonical)
		return map[string]string{"canonical_hex": hex.EncodeToString(canonical), "sha256": hex.EncodeToString(digest[:])}
	})
}
