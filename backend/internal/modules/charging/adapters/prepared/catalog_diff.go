package prepared

import (
	"errors"
	"maps"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

type NationalCatalogDiff struct {
	PreviousSHA256   string                `json:"previous_sha256,omitempty"`
	CandidateSHA256  string                `json:"candidate_sha256"`
	InventoryRecords int                   `json:"inventory_records"`
	Added            []string              `json:"added,omitempty"`
	Removed          []string              `json:"removed,omitempty"`
	Sources          []CatalogSourceChange `json:"source_changes,omitempty"`
	Moved            []CatalogMove         `json:"moved_over_50m,omitempty"`
	Groups           []CatalogGroupChange  `json:"connector_changes,omitempty"`
	Review           []CatalogDiffReview   `json:"review_required,omitempty"`
}

type CatalogSourceChange struct {
	Before string   `json:"before"`
	After  string   `json:"after"`
	Gained []string `json:"gained,omitempty"`
	Lost   []string `json:"lost,omitempty"`
}

type CatalogMove struct {
	Before    string  `json:"before"`
	After     string  `json:"after"`
	DistanceM float64 `json:"distance_m"`
}

type CatalogGroupChange struct {
	Key    string         `json:"key"`
	Source string         `json:"source"`
	Change string         `json:"change"`
	Before *NationalGroup `json:"before,omitempty"`
	After  *NationalGroup `json:"after,omitempty"`
}

type CatalogDiffReview struct {
	Before string   `json:"before"`
	Reason string   `json:"reason"`
	Keys   []string `json:"keys"`
}

// CompareNationalCatalogs verifies both bundles and compares their projections.
// The caller supplies the complete, independently verified candidate source
// inventory, including admitted assertions outside the published projection.
func CompareNationalCatalogs(previous *NationalImport, next NationalImport, nextInventory NationalSourceInventory) (NationalCatalogDiff, error) {
	if nextInventory.manifestSHA256 != next.ManifestSHA256 || nextInventory.states == nil {
		return NationalCatalogDiff{}, errors.New("prepared: candidate source inventory required")
	}
	candidate, err := admitNationalImport(next)
	if err != nil {
		return NationalCatalogDiff{}, err
	}
	if nextInventory.catalog != (ArtifactDescriptor{Bytes: int64(len(candidate.bundle.Catalog)),
		Records: candidate.bundle.Sites, SHA256: candidate.bundle.OfferSHA256}) {
		return NationalCatalogDiff{}, errors.New("prepared: inventory catalog mismatch")
	}
	for _, place := range candidate.places {
		for _, source := range place.Sources {
			if nextInventory.states[source.Key] != "ACCEPTED" {
				return NationalCatalogDiff{}, errors.New("prepared: candidate source inventory incomplete")
			}
		}
	}
	var baseline admittedNationalImport
	if previous != nil {
		baseline, err = admitNationalImport(*previous)
		if err != nil {
			return NationalCatalogDiff{}, err
		}
	}
	report := compareNationalPlaces(baseline.places, candidate.places, nextInventory.states)
	report.PreviousSHA256 = baseline.plan.BatchSHA256
	report.CandidateSHA256 = candidate.plan.BatchSHA256
	report.InventoryRecords = len(nextInventory.states)
	return report, nil
}

// NationalTransition binds a reviewable candidate to its verified baseline.
type NationalTransition struct {
	previousSHA256  string
	candidateSHA256 string
}

// VerifyNationalTransition returns a publication proof only when the verified
// comparison has no unresolved identity or source-presence items.
func VerifyNationalTransition(previous *NationalImport, next NationalImport, inventory NationalSourceInventory) (NationalCatalogDiff, NationalTransition, error) {
	report, err := CompareNationalCatalogs(previous, next, inventory)
	if err != nil || len(report.Review) != 0 {
		return report, NationalTransition{}, err
	}
	return report, NationalTransition{report.PreviousSHA256, report.CandidateSHA256}, nil
}

func compareNationalPlaces(before, after []NationalPlace, nextInventory map[string]string) NationalCatalogDiff {
	nextByRoot := map[string]*NationalPlace{}
	for i := range after {
		place := &after[i]
		for _, root := range place.Identity.SourceKeys {
			nextByRoot[root] = place
		}
	}

	report := NationalCatalogDiff{}
	matched := map[string]bool{}
	for _, old := range orderedDiffPlaces(before) {
		matches := map[string]*NationalPlace{}
		var omittedRoots []string
		for _, root := range old.Identity.SourceKeys {
			if place, ok := nextByRoot[root]; ok {
				matches[place.Identity.Key] = place
				matched[place.Identity.Key] = true
			} else if _, present := nextInventory[root]; present {
				omittedRoots = append(omittedRoots, root)
			}
		}
		key := old.Identity.Key
		if len(omittedRoots) != 0 {
			slices.Sort(omittedRoots)
			report.Review = append(report.Review, CatalogDiffReview{key, "root-still-in-source-inventory", omittedRoots})
		}
		switch len(matches) {
		case 0:
			if len(omittedRoots) == 0 {
				report.Removed = append(report.Removed, key)
			}
		case 1:
			for _, next := range matches {
				gained := missingRoots(next.Identity.SourceKeys, old.Identity.SourceKeys)
				lost := missingRoots(old.Identity.SourceKeys, next.Identity.SourceKeys)
				if len(gained) != 0 || len(lost) != 0 {
					report.Sources = append(report.Sources, CatalogSourceChange{key, next.Identity.Key, gained, lost})
				}
				if distance := catalogMovementM(old, next); distance > 50 {
					report.Moved = append(report.Moved, CatalogMove{key, next.Identity.Key, distance})
				}
			}
		default:
			report.Review = append(report.Review, CatalogDiffReview{key, "identity-split", slices.Sorted(maps.Keys(matches))})
		}
	}
	for _, place := range orderedDiffPlaces(after) {
		if !matched[place.Identity.Key] {
			report.Added = append(report.Added, place.Identity.Key)
		}
	}
	report.Groups = compareCatalogGroups(before, after)
	return report
}

func orderedDiffPlaces(places []NationalPlace) []*NationalPlace {
	ordered := make([]*NationalPlace, len(places))
	for i := range places {
		ordered[i] = &places[i]
	}
	slices.SortFunc(ordered, func(a, b *NationalPlace) int {
		return strings.Compare(a.Identity.Key, b.Identity.Key)
	})
	return ordered
}

func missingRoots(a, b []string) []string {
	var difference []string
	for _, root := range a {
		if !slices.Contains(b, root) {
			difference = append(difference, root)
		}
	}
	slices.Sort(difference)
	return difference
}

func compareCatalogGroups(before, after []NationalPlace) []CatalogGroupChange {
	previous, candidate := map[string]NationalGroup{}, map[string]NationalGroup{}
	keys := map[string]bool{}
	for _, place := range before {
		for _, group := range place.Groups {
			previous[group.Key], keys[group.Key] = group, true
		}
	}
	for _, place := range after {
		for _, group := range place.Groups {
			candidate[group.Key], keys[group.Key] = group, true
		}
	}
	var changes []CatalogGroupChange
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		old, had := previous[key]
		next, has := candidate[key]
		switch {
		case !had:
			changes = append(changes, CatalogGroupChange{key, next.SourceKey, "added", nil, &next})
		case !has:
			changes = append(changes, CatalogGroupChange{key, old.SourceKey, "removed", &old, nil})
		case !reflect.DeepEqual(old, next):
			changes = append(changes, CatalogGroupChange{key, next.SourceKey, "changed", &old, &next})
		}
	}
	return changes
}

func catalogMovementM(a, b *NationalPlace) float64 {
	const earthRadiusM = 6371000.0
	const radians = math.Pi / 180
	latA, _ := strconv.ParseFloat(a.Latitude, 64)
	lonA, _ := strconv.ParseFloat(a.Longitude, 64)
	latB, _ := strconv.ParseFloat(b.Latitude, 64)
	lonB, _ := strconv.ParseFloat(b.Longitude, 64)
	latTerm := math.Sin((latB - latA) * radians / 2)
	lonTerm := math.Sin((lonB - lonA) * radians / 2)
	half := latTerm*latTerm + math.Cos(latA*radians)*math.Cos(latB*radians)*lonTerm*lonTerm
	return 2 * earthRadiusM * math.Asin(math.Sqrt(min(1, half)))
}
