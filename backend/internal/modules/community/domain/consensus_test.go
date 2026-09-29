package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// Golden fixtures in contracts/testdata/consensus drive the required
// spec cases: each file carries inputs plus the expected verdict, and
// the harness additionally replays every case with reversed input order
// to prove permutation invariance.
type consensusFixtureVote struct {
	Voter            string `json:"voter"`
	ObservationID    string `json:"observation_id"`
	ConfirmationID   string `json:"confirmation_id"`
	EventID          string `json:"event_id"`
	Author           string `json:"author"`
	AmountMilli      int64  `json:"amount_milli"`
	ReceivedAt       string `json:"received_at"`
	AnchorReceivedAt string `json:"anchor_received_at"`
	Trust            string `json:"trust"`
	PhotoValidated   bool   `json:"photo_validated"`
	PhotoProximityOK bool   `json:"photo_proximity_ok"`
	MediaKey         string `json:"media_key"`
	StationID        string `json:"station_id"`
	Product          string `json:"fuel_product"`
	Unit             string `json:"unit"`
	ConditionKind    string `json:"condition_kind"`
	Qualifier        string `json:"qualifier_id"`
}

type consensusFixture struct {
	ID               string                 `json:"id"`
	PriceKey         PriceKey               `json:"price_key"`
	Now              string                 `json:"now"`
	ModerationOpen   bool                   `json:"moderation_open"`
	ConsensusVersion string                 `json:"consensus_version"`
	Votes            []consensusFixtureVote `json:"votes"`
	Expect           struct {
		Verdict       string   `json:"verdict"`
		AmountMilli   *int64   `json:"amount_milli"`
		Confidence    *string  `json:"confidence"`
		Freshness     *string  `json:"freshness"`
		ExpiresAt     *string  `json:"expires_at"`
		Supporters    *int     `json:"supporters"`
		Confirmations *int     `json:"confirmations"`
		Reasons       []string `json:"reasons"`
		Error         bool     `json:"error"`
	} `json:"expect"`
}

func loadFixtures(t *testing.T) []consensusFixture {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "..", "contracts", "testdata", "consensus", "*.json"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no consensus fixtures found: %v", err)
	}
	sort.Strings(matches)
	out := make([]consensusFixture, 0, len(matches))
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var f consensusFixture
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse %s: %v", filepath.Base(path), err)
		}
		if f.ID == "" {
			t.Fatalf("%s: fixture needs an id", filepath.Base(path))
		}
		out = append(out, f)
	}
	return out
}

func mustTime(t *testing.T, raw string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("bad time %q: %v", raw, err)
	}
	return ts
}

func fixtureVotes(t *testing.T, f consensusFixture) []Vote {
	t.Helper()
	out := make([]Vote, 0, len(f.Votes))
	for _, v := range f.Votes {
		out = append(out, Vote{
			VoterRef: v.Voter, ObservationID: v.ObservationID,
			ConfirmationID: v.ConfirmationID, EventID: v.EventID, AuthorRef: v.Author,
			AmountMilli: v.AmountMilli,
			ReceivedAt:  mustTime(t, v.ReceivedAt), AnchorReceivedAt: mustTime(t, v.AnchorReceivedAt),
			TrustTier: v.Trust, PhotoValidated: v.PhotoValidated,
			PhotoProximityOK: v.PhotoProximityOK, MediaKey: v.MediaKey,
			StationID: v.StationID, Product: v.Product, Unit: v.Unit,
			ConditionKind: v.ConditionKind, Qualifier: v.Qualifier,
		})
	}
	return out
}

func checkExpect(t *testing.T, f consensusFixture, got Result) {
	t.Helper()
	exp := f.Expect
	if got.Verdict != exp.Verdict {
		t.Errorf("verdict = %q, want %q", got.Verdict, exp.Verdict)
	}
	if exp.AmountMilli != nil && got.AmountMilli != *exp.AmountMilli {
		t.Errorf("amount = %d, want %d", got.AmountMilli, *exp.AmountMilli)
	}
	if exp.Confidence != nil && got.Confidence != *exp.Confidence {
		t.Errorf("confidence = %q, want %q", got.Confidence, *exp.Confidence)
	}
	if exp.Freshness != nil && got.Freshness != *exp.Freshness {
		t.Errorf("freshness = %q, want %q", got.Freshness, *exp.Freshness)
	}
	if exp.ExpiresAt != nil && got.ExpiresAt.Format(time.RFC3339) != *exp.ExpiresAt {
		t.Errorf("expires = %q, want %q", got.ExpiresAt.Format(time.RFC3339), *exp.ExpiresAt)
	}
	if exp.Supporters != nil && got.Supporters != *exp.Supporters {
		t.Errorf("supporters = %d, want %d", got.Supporters, *exp.Supporters)
	}
	if exp.Confirmations != nil && got.Confirmations != *exp.Confirmations {
		t.Errorf("confirmations = %d, want %d", got.Confirmations, *exp.Confirmations)
	}
	for _, want := range exp.Reasons {
		found := false
		for _, r := range got.Reasons {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("reasons = %v, want %q", got.Reasons, want)
		}
	}
	if got.AlgorithmVersion != ConsensusV1 {
		t.Errorf("algorithm = %q, want %q", got.AlgorithmVersion, ConsensusV1)
	}
}

func TestConsensusGoldenCases(t *testing.T) {
	for _, f := range loadFixtures(t) {
		t.Run(f.ID, func(t *testing.T) {
			now := mustTime(t, f.Now)
			opts := Options{Version: f.ConsensusVersion, ModerationOpen: f.ModerationOpen}
			got, err := Compute(f.PriceKey, fixtureVotes(t, f), now, opts)
			if f.Expect.Error {
				if err == nil {
					t.Error("expected error, got success")
				}
				return
			}
			if err != nil {
				t.Fatalf("compute: %v", err)
			}
			checkExpect(t, f, got)
			// Permutation invariance: reversed inputs decide identically.
			rev := fixtureVotes(t, f)
			for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
				rev[i], rev[j] = rev[j], rev[i]
			}
			again, err := Compute(f.PriceKey, rev, now, opts)
			if err != nil {
				t.Fatalf("reversed compute: %v", err)
			}
			if !reflect.DeepEqual(got, again) {
				t.Errorf("permutation changed outcome:\n%+v\n%+v", got, again)
			}
		})
	}
}

func TestComputeRejectsBadInput(t *testing.T) {
	now := time.Now()
	key := PriceKey{StationID: "s", Product: "GASOLINE_REGULAR", Unit: "L", ConditionKind: "STANDARD", Qualifier: "STANDARD"}
	good := Vote{
		VoterRef: "c1", ObservationID: "o1", EventID: "e1", AuthorRef: "c1",
		AmountMilli: 5999, ReceivedAt: now, AnchorReceivedAt: now, TrustTier: "NEW",
	}
	bad := func(mut func(*Vote)) Vote {
		v := good
		mut(&v)
		return v
	}
	cases := map[string][]Vote{
		"empty voter":       {bad(func(v *Vote) { v.VoterRef = "" })},
		"empty observation": {bad(func(v *Vote) { v.ObservationID = "" })},
		"empty event":       {bad(func(v *Vote) { v.EventID = "" })},
		"zero amount":       {bad(func(v *Vote) { v.AmountMilli = 0 })},
		"zero time":         {bad(func(v *Vote) { v.ReceivedAt = time.Time{} })},
		"unknown trust":     {bad(func(v *Vote) { v.TrustTier = "GOLD" })},
	}
	for name, votes := range cases {
		if _, err := Compute(key, votes, now, Options{Version: ConsensusV1}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := Compute(key, []Vote{good}, now, Options{Version: "consensus-v0"}); err == nil {
		t.Error("unknown version accepted")
	}
	emptyKey := PriceKey{}
	if _, err := Compute(emptyKey, []Vote{good}, now, Options{Version: ConsensusV1}); err == nil {
		t.Error("empty price key accepted")
	}
}

func TestRecencyWeightBoundaries(t *testing.T) {
	weight := func(age time.Duration, trust string) int64 {
		return recencyFactor(age) * trustFactor(trust)
	}
	if weight(6*time.Hour, "NEW") != 4 || weight(6*time.Hour+time.Second, "NEW") != 2 {
		t.Error("6h boundary wrong")
	}
	if weight(24*time.Hour, "NEW") != 2 || weight(24*time.Hour+time.Second, "NEW") != 1 {
		t.Error("24h boundary wrong")
	}
	if weight(time.Hour, "ESTABLISHED") != 8 || weight(time.Hour, "NEW") != 4 {
		t.Error("trust factors wrong")
	}
}

func TestNoPaidWeightByConstruction(t *testing.T) {
	// Votes and keys carry amounts, identities and media only: payment,
	// entitlement, ANP and device fields cannot weigh what does not exist.
	forbidden := []string{"pay", "entitle", "premium", "anp", "device", "email", "weight", "score"}
	for _, typ := range []reflect.Type{reflect.TypeOf(Vote{}), reflect.TypeOf(PriceKey{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, bad := range forbidden {
				if strings.Contains(name, bad) {
					t.Errorf("%s.%s invites %q influence", typ.Name(), typ.Field(i).Name, bad)
				}
			}
		}
	}
}

func TestWinnerIDsFeedRestrictedAudit(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	key := PriceKey{StationID: "s", Product: "GASOLINE_REGULAR", Unit: "L", ConditionKind: "STANDARD", Qualifier: "STANDARD"}
	mk := func(voter, obs, conf, event string) Vote {
		return Vote{
			VoterRef: voter, ObservationID: obs, ConfirmationID: conf, EventID: event,
			AuthorRef: "c1", AmountMilli: 5999,
			ReceivedAt: now.Add(-time.Hour), AnchorReceivedAt: now.Add(-time.Hour),
			TrustTier: "NEW", StationID: "s", Product: "GASOLINE_REGULAR",
			Unit: "L", ConditionKind: "STANDARD", Qualifier: "STANDARD",
		}
	}
	got, err := Compute(key, []Vote{
		mk("c1", "o9", "", "e1"),
		mk("c2", "o9", "k2", "e2"),
		mk("c3", "o1", "", "e3"),
	}, now, Options{Version: ConsensusV1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != VerdictPrice || len(got.WinnerObservationIDs) != 2 || len(got.WinnerConfirmationIDs) != 1 {
		t.Fatalf("result = %+v", got)
	}
	if got.WinnerObservationIDs[0] != "o1" || got.WinnerObservationIDs[1] != "o9" {
		t.Errorf("anchors not stably sorted: %v", got.WinnerObservationIDs)
	}
	if got.WinnerConfirmationIDs[0] != "k2" {
		t.Errorf("confirmations = %v", got.WinnerConfirmationIDs)
	}
}
