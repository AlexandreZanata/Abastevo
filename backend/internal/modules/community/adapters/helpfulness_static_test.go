package adapters

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// CS06: helpfulness never feeds consensus, confidence, ratings or
// projections, and dispute/vote audit is never swept: the 35-day
// horizon governs ledger/media retention, never fact audit. These
// static pins fail the build the moment a query crosses the boundary.
func communityQuery(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "db", "queries", "community", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return strings.ToUpper(string(raw))
}

func TestHelpfulnessStaysOutOfConsensusReads(t *testing.T) {
	for _, name := range []string{"projection.sql", "observations.sql", "feed.sql", "votes.sql", "signals.sql"} {
		if strings.Contains(communityQuery(t, name), "HELPFULNESS_") {
			t.Errorf("%s references helpfulness tables", name)
		}
	}
}

func TestHelpfulnessWritesTouchNoRatingsOrProjections(t *testing.T) {
	raw := communityQuery(t, "helpfulness.sql")
	for _, other := range []string{"FEEDBACK_", "RATING", "CONSENSUS", "CURRENT_PRICES", "PROJECTION_INPUTS"} {
		if strings.Contains(raw, other) {
			t.Errorf("helpfulness.sql references %s", other)
		}
	}
}

func TestDisputeVoteAuditNeverSwept(t *testing.T) {
	// The 35-day horizon bounds ledger/media retention; dispute and
	// vote facts stay append-only audit with no DELETE path.
	files, err := filepath.Glob(filepath.Join(filepath.Dir(mustCallerFile(t)), "..", "..", "..", "..", "db", "queries", "community", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		upper := strings.ToUpper(string(raw))
		base := filepath.Base(path)
		if base == "photo_captures.sql" {
			continue // 24h media lifecycle owns its deletes.
		}
		for _, verb := range []string{"DELETE FROM COMMUNITY_DISPUTES", "DELETE FROM COMMUNITY_CONFIRMATIONS", "DELETE FROM COMMUNITY_HELPFULNESS_VOTES", "DELETE FROM COMMUNITY_OBSERVATIONS", "TRUNCATE"} {
			if strings.Contains(upper, verb) {
				t.Errorf("%s destroys audit: %s", base, verb)
			}
		}
	}
}

func mustCallerFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return file
}
