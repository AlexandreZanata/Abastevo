// Package domain defines the public inputs and rights a Data dataset can use.
package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalidDataset = errors.New("orgdata: invalid dataset")
	ErrRightsPending  = errors.New("orgdata: current source rights unavailable")
	ErrSourceBinding  = errors.New("orgdata: official export binding mismatch")
	keyPattern        = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	digestPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

const ANPAttribution = "Fonte: ANP"

type Source string

const (
	PublicRegistry Source = "S-ESP-01"
	OfficialPrices Source = "S-ESP-02"
)

type SourcePin struct {
	Source         Source
	Digest         string
	ReviewRevision int64
}

type RightsState string

const (
	RightsPending RightsState = "RIGHTS_PENDING"
	Approved      RightsState = "APPROVED"
	Withdrawn     RightsState = "WITHDRAWN"
)

type Review struct {
	Pin         SourcePin
	State       RightsState
	LicenseRef  string
	Attribution string
}

type Definition struct {
	Key              string
	DisplayName      string
	Revision         int64
	MarketState      string
	MinimumStations  int
	Method           string
	Snapshot         string
	OfficialRevision string
	Registry         SourcePin
	Prices           SourcePin
}

func (d Definition) Validate() error {
	official, err := kernel.ParseOrgID(d.OfficialRevision)
	if err != nil || official.String() != d.OfficialRevision || !ValidDatasetKey(d.Key) ||
		!publicText(d.DisplayName, 120) || d.Revision < 1 || !marketState(d.MarketState) ||
		d.MinimumStations < 5 || d.Method != analytics.MetricsVersion || !digestPattern.MatchString(d.Snapshot) ||
		!d.Registry.matches(PublicRegistry) || !d.Prices.matches(OfficialPrices) {
		return ErrInvalidDataset
	}
	return nil
}

// CheckAdmission expects the latest review for each exact source digest.
// An older approval cannot override a more recent decision.
func (d Definition) CheckAdmission(reviews []Review) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if len(reviews) != 2 {
		return ErrRightsPending
	}
	registry, prices := false, false
	for _, review := range reviews {
		if review.State != Approved || !publicText(review.LicenseRef, 300) || !publicText(review.Attribution, 300) {
			return ErrRightsPending
		}
		switch review.Pin {
		case d.Registry:
			registry = true
		case d.Prices:
			prices = true
		default:
			return ErrRightsPending
		}
	}
	if !registry || !prices {
		return ErrRightsPending
	}
	return nil
}

// MatchManifest checks the declared export identity and the official source
// pins. An input reader must also verify the actual artifact bytes.
func (d Definition) MatchManifest(manifest enterpriseexport.Manifest) error {
	if err := d.Validate(); err != nil {
		return err
	}
	if manifest.SchemaVersion != enterpriseexport.SchemaVersion || manifest.ContentVersion != enterpriseexport.ContentVersion ||
		manifest.PublicFieldPolicy != enterpriseexport.PublicFieldPolicy || manifest.ContentSHA256 != d.Snapshot ||
		manifest.Sources.Directory.Source != string(PublicRegistry) || manifest.Sources.Directory.Kind != "directory" ||
		manifest.Sources.Forum != nil || manifest.Counts.ForumPosts != 0 || len(manifest.Files) != 3 {
		return ErrSourceBinding
	}
	boundFiles := make(map[string]bool, 3)
	for _, file := range manifest.Files {
		switch file.Name {
		case "stations.jsonl", "revisions.jsonl", "prices.jsonl":
		default:
			return ErrSourceBinding
		}
		if boundFiles[file.Name] || !digestPattern.MatchString(file.SHA256) ||
			(file.Name == "stations.jsonl" && file.SHA256 != d.Registry.Digest) {
			return ErrSourceBinding
		}
		boundFiles[file.Name] = true
	}
	matching := 0
	for _, official := range manifest.Sources.Official {
		if official.RevisionID == d.OfficialRevision {
			if official.SourceChecksum != d.Prices.Digest {
				return ErrSourceBinding
			}
			matching++
		}
	}
	if matching != 1 {
		return ErrSourceBinding
	}
	return nil
}

func (pin SourcePin) matches(source Source) bool {
	return pin.Source == source && pin.ReviewRevision > 0 && digestPattern.MatchString(pin.Digest)
}

func ValidDatasetKey(value string) bool {
	return len(value) <= 64 && keyPattern.MatchString(value)
}

func publicText(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}

func marketState(value string) bool {
	switch value {
	case "AC", "AL", "AP", "AM", "BA", "CE", "DF", "ES", "GO", "MA", "MT", "MS", "MG", "PA",
		"PB", "PR", "PE", "PI", "RJ", "RN", "RS", "RO", "RR", "SC", "SP", "SE", "TO":
		return true
	default:
		return false
	}
}
