package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
)

const testOfficialRevision = "df12742d-7361-4308-ad01-0167408e778a"

func TestDatasetReviewsAreExactAndCurrent(t *testing.T) {
	registry := domain.SourcePin{Source: domain.PublicRegistry, Digest: "sha256:" + strings.Repeat("a", 64), ReviewRevision: 3}
	prices := domain.SourcePin{Source: domain.OfficialPrices, Digest: "sha256:" + strings.Repeat("b", 64), ReviewRevision: 1}
	definition := domain.Definition{
		Key: "regional-prices", DisplayName: "Synthetic regional dataset", Revision: 1,
		MarketState: "MT", MinimumStations: 5, OfficialRevision: testOfficialRevision,
		Method: analytics.MetricsVersion, Snapshot: "sha256:" + strings.Repeat("c", 64),
		Registry: registry, Prices: prices,
	}
	review := func(pin domain.SourcePin) domain.Review {
		return domain.Review{Pin: pin, State: domain.Approved, LicenseRef: "synthetic-approved-license", Attribution: "Synthetic official dataset"}
	}
	reviews := []domain.Review{review(registry), review(prices)}
	if err := definition.CheckAdmission(reviews); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func([]domain.Review){
		func(r []domain.Review) { r[1].State = domain.RightsPending },
		func(r []domain.Review) { r[0].State = domain.Withdrawn },
		func(r []domain.Review) { r[0].Pin.ReviewRevision++ },
		func(r []domain.Review) { r[0].Pin.Digest = r[1].Pin.Digest },
		func(r []domain.Review) { r[0].Attribution = "" },
		func(r []domain.Review) { r[0] = r[1] },
	} {
		input := append([]domain.Review(nil), reviews...)
		change(input)
		if err := definition.CheckAdmission(input); !errors.Is(err, domain.ErrRightsPending) {
			t.Fatalf("unreviewed or mismatched source entered a commercial dataset: %v", err)
		}
	}
	unsafe := definition
	unsafe.MinimumStations = 1
	if err := unsafe.CheckAdmission(reviews); !errors.Is(err, domain.ErrInvalidDataset) {
		t.Fatalf("unreviewed small-cell threshold was accepted: %v", err)
	}
}

func TestDatasetBindsOnlyOfficialExportInputs(t *testing.T) {
	digest := func(seed string) string { return "sha256:" + strings.Repeat(seed, 64) }
	definition := domain.Definition{
		Key: "regional-prices", DisplayName: "Synthetic regional dataset", Revision: 1,
		MarketState: "MT", MinimumStations: 5, OfficialRevision: testOfficialRevision,
		Method: analytics.MetricsVersion, Snapshot: digest("1"),
		Registry: domain.SourcePin{Source: domain.PublicRegistry, Digest: digest("2"), ReviewRevision: 1},
		Prices:   domain.SourcePin{Source: domain.OfficialPrices, Digest: digest("3"), ReviewRevision: 1},
	}
	manifest := enterpriseexport.Manifest{
		SchemaVersion: enterpriseexport.SchemaVersion, ContentVersion: enterpriseexport.ContentVersion,
		PublicFieldPolicy: enterpriseexport.PublicFieldPolicy, ContentSHA256: definition.Snapshot,
		Sources: enterpriseexport.Sources{
			Directory: enterpriseexport.DirectoryPin{Source: string(domain.PublicRegistry), Kind: "directory"},
			Official:  []enterpriseexport.RevisionPin{{RevisionID: testOfficialRevision, SourceChecksum: definition.Prices.Digest}},
		},
		Files: []enterpriseexport.FileDigest{{Name: "stations.jsonl", SHA256: definition.Registry.Digest},
			{Name: "revisions.jsonl", SHA256: digest("4")}, {Name: "prices.jsonl", SHA256: digest("5")}},
	}
	if err := definition.MatchManifest(manifest); err != nil {
		t.Fatal(err)
	}
	mixed := manifest
	mixed.Sources.Forum = &enterpriseexport.ForumPin{Kind: enterpriseexport.ForumKind}
	if err := definition.MatchManifest(mixed); !errors.Is(err, domain.ErrSourceBinding) {
		t.Fatalf("mixed editorial source admitted: %v", err)
	}
	foreign := manifest
	foreign.ContentSHA256 = digest("6")
	if err := definition.MatchManifest(foreign); !errors.Is(err, domain.ErrSourceBinding) {
		t.Fatalf("foreign snapshot admitted: %v", err)
	}
}
