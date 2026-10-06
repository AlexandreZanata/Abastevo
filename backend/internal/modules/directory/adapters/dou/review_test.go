package dou

import (
	"testing"
	"time"
)

// P26-T03 review, catch-up and acceptance: weekday backfill computation
// with checkpoint dedup, correction-chain linking (supersession), and
// republication convergence. Review concurrency/role denial reuses the
// existing moderation ports (no new authority invented here); G26 stays
// BLOCKED on live access per plan.

func TestMissingDatesSkipsWeekendsAndSeen(t *testing.T) {
	seen := map[string]bool{"2026-10-05": true}
	got := MissingDates("2026-10-02", "2026-10-06", func(date string) bool { return seen[date] })
	// Fri 10-02, Mon 10-05 (seen), Tue 10-06. Sat/Sun never listed.
	want := []string{"2026-10-02", "2026-10-06"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("missing = %v, want %v", got, want)
	}
}

func TestMissingDatesRejectsBadRange(t *testing.T) {
	if len(MissingDates("2026-10-06", "2026-10-02", func(string) bool { return false })) != 0 {
		t.Fatal("reversed range must be empty")
	}
	if len(MissingDates("not-a-date", "2026-10-06", func(string) bool { return false })) != 0 {
		t.Fatal("bad start must be empty")
	}
}

func TestLinkCorrectionChainsSupersession(t *testing.T) {
	prior := CorrectionLink{AssertionID: "a-prior", ActID: "ANP-2026-0001"}
	next := CorrectionLink{AssertionID: "a-next", ActID: "ANP-2026-0009", CorrectsActID: "ANP-2026-0001"}
	linked, err := LinkCorrection(prior, next)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if linked.SupersededBy != "a-prior" {
		t.Fatalf("linked = %+v", linked)
	}
	if _, err := LinkCorrection(prior, CorrectionLink{AssertionID: "a-x", ActID: "A-X"}); err == nil {
		t.Fatal("unrelated correction must not link")
	}
	if _, err := LinkCorrection(prior, CorrectionLink{AssertionID: "a-prior", ActID: "ANP-2026-0001", CorrectsActID: "ANP-2026-0001"}); err == nil {
		t.Fatal("self-link must not link")
	}
}

func TestRepublicationConvergesByChecksum(t *testing.T) {
	editionA := Edition{Date: "2026-10-01", Checksum: "ed-a"}
	editionB := Edition{Date: "2026-10-05", Checksum: "ed-b"}
	act := Act{ID: "ANP-2026-0001", Text: "autoriza o posto ALFA, revenda varejista, CNPJ 04218406000104"}
	first, skippedA := StageActs(editionA, []Act{act})
	second, skippedB := StageActs(editionB, []Act{act})
	if skippedA != 0 || skippedB != 0 || len(first) != 1 || len(second) != 1 {
		t.Fatalf("first = %+v, second = %+v", first, second)
	}
	if first[0].Checksum == second[0].Checksum {
		t.Fatal("republication must leave a traceable distinct row")
	}
	if first[0].SourceKey != second[0].SourceKey {
		t.Fatal("republication must converge on the same identity")
	}
	_ = time.Now
}
