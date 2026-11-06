package domain

import (
	"errors"
	"strings"
	"testing"
)

func bulletinInput() BulletinInput {
	return BulletinInput{
		UF:                         "SP",
		Fuel:                       "GASOLINE_REGULAR",
		Unit:                       "L",
		CurrentRevisionID:          "11111111-1111-4111-8111-111111111111",
		CurrentSurveyStart:         "2026-10-26",
		CurrentSurveyEnd:           "2026-11-01",
		CurrentPublishedAt:         "2026-11-02T12:00:00Z",
		CurrentSourceURL:           "https://www.gov.br/anp/synthetic-current.xlsx",
		CurrentSourceChecksum:      "sha256:current-1111",
		CurrentParserVersion:       "synthetic-parser-v1",
		CurrentSumMilliBrl:         601800,
		CurrentObservationCount:    100,
		CurrentSampledStations:     100,
		CurrentSourceStations:      120,
		CurrentSourceObservations:  105,
		PreviousRevisionID:         "22222222-2222-4222-8222-222222222222",
		PreviousSurveyStart:        "2026-10-19",
		PreviousSurveyEnd:          "2026-10-25",
		PreviousPublishedAt:        "2026-10-27T12:00:00Z",
		PreviousSourceURL:          "https://www.gov.br/anp/synthetic-previous.xlsx",
		PreviousSourceChecksum:     "sha256:previous-2222",
		PreviousParserVersion:      "synthetic-parser-v1",
		PreviousSumMilliBrl:        590000,
		PreviousObservationCount:   100,
		PreviousSampledStations:    100,
		PreviousSourceStations:     118,
		PreviousSourceObservations: 100,
	}
}

func TestBuildBulletinDraftFreezesTitleExcerptAndProvenance(t *testing.T) {
	draft, err := BuildBulletinDraft(bulletinInput())
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	if draft.Key != "forum-bulletin-v1:11111111-1111-4111-8111-111111111111:22222222-2222-4222-8222-222222222222:SP:GASOLINE_REGULAR:L" {
		t.Fatalf("unexpected key %q", draft.Key)
	}
	if draft.Title != "Gasolina comum em SP: R$ 6,018/L" {
		t.Fatalf("unexpected title %q", draft.Title)
	}
	wantExcerpt := "Média amostral de 100 postos: R$ 6,018/L, contra R$ 5,900/L na semana anterior (+0,118/L, +2,0%)."
	if draft.Excerpt != wantExcerpt {
		t.Fatalf("unexpected excerpt %q", draft.Excerpt)
	}
	body := draft.BodyText
	for _, fragment := range []string{
		"média amostral do preço da gasolina comum (L) no estado de SP foi de R$ 6,018/L",
		"na semana de 2026-10-26 a 2026-11-01",
		"Na semana anterior (2026-10-19 a 2026-10-25), a média foi de R$ 5,900/L",
		"variação de +0,118/L (+2,0%)",
		"observação mais recente por posto",
		"Não é a média publicada pela ANP",
		"não mede a variação do preço no mesmo posto",
		"não representa a frota nacional",
		"Revisão atual: 11111111-1111-4111-8111-111111111111",
		"checksum sha256:current-1111",
		"https://www.gov.br/anp/synthetic-current.xlsx",
		"Revisão anterior: 22222222-2222-4222-8222-222222222222",
		"checksum sha256:previous-2222",
		"100 postos amostrados na revisão atual e 100 na anterior",
		"120 postos com qualquer preço oficial na UF na revisão atual; 118 na anterior",
		"Metodologia forum-official-mean-v1",
		"Template forum-bulletin-v1",
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("body missing %q:\n%s", fragment, body)
		}
	}
	if got := len([]rune(draft.Title)); got > MaxTopicTitleRunes {
		t.Fatalf("title has %d runes", got)
	}
	if got := len([]rune(draft.Excerpt)); got > 400 {
		t.Fatalf("excerpt has %d runes", got)
	}
	if got := len([]rune(draft.BodyText)); got > MaxTopicBodyRunes {
		t.Fatalf("body has %d runes", got)
	}
}

func TestBuildBulletinDraftIsByteDeterministic(t *testing.T) {
	first, err := BuildBulletinDraft(bulletinInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildBulletinDraft(bulletinInput())
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.Title != second.Title || first.Excerpt != second.Excerpt || first.BodyText != second.BodyText {
		t.Fatalf("draft output is not deterministic:\n%+v\n%+v", first, second)
	}
}

func TestBuildBulletinDraftRendersSignedDeltasAndPercentTies(t *testing.T) {
	tests := []struct {
		name       string
		current    int64
		previous   int64
		wantDelta  string
		wantChange string
	}{
		{name: "positive", current: 601800, previous: 590000, wantDelta: "+0,118", wantChange: "+2,0"},
		{name: "negative", current: 580000, previous: 590000, wantDelta: "-0,100", wantChange: "-1,7"},
		{name: "zero", current: 590000, previous: 590000, wantDelta: "0,000", wantChange: "0,0"},
		{name: "half away positive", current: 590500, previous: 590000, wantDelta: "+0,005", wantChange: "+0,1"},
		{name: "half away negative", current: 589500, previous: 590000, wantDelta: "-0,005", wantChange: "-0,1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := bulletinInput()
			in.CurrentSumMilliBrl = tc.current
			draft, err := BuildBulletinDraft(in)
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			if !strings.Contains(draft.Excerpt, "("+tc.wantDelta+"/L, "+tc.wantChange+"%)") {
				t.Fatalf("excerpt %q missing delta %s/%s", draft.Excerpt, tc.wantDelta, tc.wantChange)
			}
		})
	}
}

func TestBulletinRoundingHalfAwayFromZero(t *testing.T) {
	tests := []struct {
		num, den int64
		want     int64
	}{
		{1, 2, 1}, {-1, 2, -1}, {3, 2, 2}, {-3, 2, -2},
		{5, 2, 3}, {-5, 2, -3}, {4, 3, 1}, {-4, 3, -1},
	}
	for _, tc := range tests {
		if got := roundHalfAway(tc.num, tc.den); got != tc.want {
			t.Errorf("roundHalfAway(%d,%d) = %d, want %d", tc.num, tc.den, got, tc.want)
		}
	}
}

func TestBulletinCorrectionIdentity(t *testing.T) {
	original, err := BuildBulletinDraft(bulletinInput())
	if err != nil {
		t.Fatal(err)
	}
	correctedInput := bulletinInput()
	correctedInput.CurrentRevisionID = "33333333-3333-4333-8333-333333333333"
	correctedInput.CurrentSourceChecksum = "sha256:current-3333"
	correctedInput.CurrentPublishedAt = "2026-11-03T12:00:00Z"
	corrected, err := BuildBulletinDraft(correctedInput)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Key == original.Key {
		t.Fatal("corrected revision must have a new key")
	}
	if !corrected.Identity.IsCorrectionOf(original.Identity) {
		t.Fatalf("same-window correction not identified: %+v -> %+v", original.Identity, corrected.Identity)
	}
	if original.Identity.IsCorrectionOf(corrected.Identity) {
		t.Fatal("correction direction must be explicit")
	}
	if original.Identity.IsCorrectionOf(original.Identity) {
		t.Fatal("same key must be a no-op, not a correction")
	}

	nextWeek := bulletinInput()
	nextWeek.PreviousRevisionID = "33333333-3333-4333-8333-333333333333"
	nextWeek.PreviousSourceChecksum = "sha256:current-3333"
	nextWeek.PreviousSurveyStart = "2026-10-26"
	nextWeek.PreviousSurveyEnd = "2026-11-01"
	nextWeek.CurrentRevisionID = "44444444-4444-4444-8444-444444444444"
	nextWeek.CurrentSurveyStart = "2026-11-02"
	nextWeek.CurrentSurveyEnd = "2026-11-08"
	nextDraft, err := BuildBulletinDraft(nextWeek)
	if err != nil {
		t.Fatal(err)
	}
	if nextDraft.Identity.IsCorrectionOf(original.Identity) {
		t.Fatal("a new survey window is not a correction")
	}
}

func TestBuildBulletinDraftRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*BulletinInput)
	}{
		{name: "unknown uf", mutate: func(in *BulletinInput) { in.UF = "XX" }},
		{name: "unknown fuel", mutate: func(in *BulletinInput) { in.Fuel = "DIESEL" }},
		{name: "wrong unit", mutate: func(in *BulletinInput) { in.Unit = "M3" }},
		{name: "missing current revision", mutate: func(in *BulletinInput) { in.CurrentRevisionID = "" }},
		{name: "same revision", mutate: func(in *BulletinInput) { in.PreviousRevisionID = in.CurrentRevisionID }},
		{name: "below minimum", mutate: func(in *BulletinInput) { in.CurrentSampledStations = 29 }},
		{name: "sampled mismatch", mutate: func(in *BulletinInput) { in.CurrentObservationCount = 99 }},
		{name: "source stations below sample", mutate: func(in *BulletinInput) { in.CurrentSourceStations = 99 }},
		{name: "source observations below sample", mutate: func(in *BulletinInput) { in.CurrentSourceObservations = 99 }},
		{name: "zero sum", mutate: func(in *BulletinInput) { in.CurrentSumMilliBrl = 0 }},
		{name: "negative sum", mutate: func(in *BulletinInput) { in.PreviousSumMilliBrl = -1 }},
		{name: "overlapping periods", mutate: func(in *BulletinInput) { in.PreviousSurveyEnd = "2026-10-26" }},
		{name: "reversed current period", mutate: func(in *BulletinInput) { in.CurrentSurveyStart = "2026-11-02" }},
		{name: "missing source url", mutate: func(in *BulletinInput) { in.CurrentSourceURL = "" }},
		{name: "non-https source url", mutate: func(in *BulletinInput) { in.CurrentSourceURL = "http://example.com/source.xlsx" }},
		{name: "control character in checksum", mutate: func(in *BulletinInput) { in.PreviousSourceChecksum = "sha256:bad\nvalue" }},
		{name: "missing checksum", mutate: func(in *BulletinInput) { in.PreviousSourceChecksum = "" }},
		{name: "content too long", mutate: func(in *BulletinInput) { in.CurrentSourceURL = "https://example.com/" + strings.Repeat("a", 5000) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := bulletinInput()
			tc.mutate(&in)
			if _, err := BuildBulletinDraft(in); err == nil {
				t.Fatal("expected refusal")
			}
		})
	}
}

func TestBuildBulletinDraftRejectsUnsupportedPercentRange(t *testing.T) {
	in := bulletinInput()
	in.PreviousSumMilliBrl = 1
	in.PreviousObservationCount = 1_000_000
	in.PreviousSampledStations = 1_000_000
	in.PreviousSourceStations = 1_000_000
	in.PreviousSourceObservations = 1_000_000
	in.CurrentSumMilliBrl = 9223372036854775807
	if _, err := BuildBulletinDraft(in); !errors.Is(err, ErrBulletinArithmetic) {
		t.Fatalf("expected ErrBulletinArithmetic, got %v", err)
	}
}

func TestBuildBulletinDraftSourceChecksumChange(t *testing.T) {
	original, err := BuildBulletinDraft(bulletinInput())
	if err != nil {
		t.Fatal(err)
	}
	changed := bulletinInput()
	changed.CurrentSourceChecksum = "sha256:current-9999"
	draft, err := BuildBulletinDraft(changed)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Key != original.Key {
		t.Fatal("same revision pair must keep the identity key")
	}
	if !strings.Contains(draft.BodyText, "sha256:current-9999") || strings.Contains(draft.BodyText, "sha256:current-1111") {
		t.Fatalf("checksum provenance was not rendered: %s", draft.BodyText)
	}
}

func TestBuildBulletinDraftRejectsOversizedProvenance(t *testing.T) {
	in := bulletinInput()
	in.CurrentSourceURL = "https://example.com/" + strings.Repeat("a", 5000)
	_, err := BuildBulletinDraft(in)
	if !errors.Is(err, ErrBulletinContentTooLong) {
		t.Fatalf("expected ErrBulletinContentTooLong, got %v", err)
	}
}
