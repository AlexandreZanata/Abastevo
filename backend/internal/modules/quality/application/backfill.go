package application

import (
	"sort"
	"strings"
)

// Plan actions. Only import_new and correct enqueue import jobs; aliases
// and stable replays converge without new rows, and missing resources
// skip quietly until the source publishes them.
type PlanAction string

const (
	PlanImportNew    PlanAction = "import_new"
	PlanReplayStable PlanAction = "replay_stable"
	PlanCorrect      PlanAction = "correct"
	PlanAlias        PlanAction = "alias"
	PlanSkipMissing  PlanAction = "skip_missing"
)

// InventoryEntry is one observed listing row: a label, its resource URL
// (empty while unpublished) and the observed content hash (empty while
// unfetched). Labels are zero-padded so lexical order is chronological.
type InventoryEntry struct {
	Label  string
	URL    string
	SHA256 string
	Bytes  uint64
}

// RecordedEdition is one ledger-known edition with its resource hashes.
type RecordedEdition struct {
	Label          string
	ManifestHash   string
	ResourceHashes []string
}

// Planned is one decided edition with its reason. AliasOf names the
// recorded label whose bytes the alias shares; it is empty otherwise.
type Planned struct {
	Label   string
	URL     string
	Action  PlanAction
	Reason  string
	AliasOf string
}

// PlanBackfill decides one bounded chunk, oldest labels first. Rules:
//   - empty URL → skip_missing (unpublished yet, retried by later runs);
//   - unknown label → import_new (late resources included: never skipped);
//   - same label, same hash → replay_stable (publisher converges, no job);
//   - same label, new hash → correct (superseding revision, not a rewrite);
//   - known hash under another recorded label → alias (one import covers
//     both labels, e.g. the duplicated February/July monthly resource).
func PlanBackfill(entries []InventoryEntry, recorded []RecordedEdition) []Planned {
	byLabel := map[string]RecordedEdition{}
	hashLabels := map[string]string{}
	for _, r := range recorded {
		byLabel[r.Label] = r
		for _, h := range r.ResourceHashes {
			if h == "" {
				continue
			}
			if _, ok := hashLabels[strings.ToLower(h)]; !ok {
				hashLabels[strings.ToLower(h)] = r.Label
			}
		}
	}
	ordered := append([]InventoryEntry(nil), entries...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Label < ordered[j].Label })
	planned := make([]Planned, 0, len(ordered))
	for _, e := range ordered {
		label := strings.TrimSpace(e.Label)
		if strings.TrimSpace(e.URL) == "" {
			planned = append(planned, Planned{
				Label: label, Action: PlanSkipMissing,
				Reason: "resource unpublished; later runs retry",
			})
			continue
		}
		rec, known := byLabel[label]
		hash := strings.ToLower(strings.TrimSpace(e.SHA256))
		switch {
		case !known:
			if owner, dup := hashLabels[hash]; dup && hash != "" {
				planned = append(planned, Planned{
					Label: label, URL: e.URL, Action: PlanAlias,
					Reason:  "bytes already imported under another label; no double import",
					AliasOf: owner,
				})
			} else {
				planned = append(planned, Planned{
					Label: label, URL: e.URL, Action: PlanImportNew,
					Reason: "label unrecorded; late resources import, never skip",
				})
			}
		case hash == "" || sameHash(rec.ResourceHashes, e.SHA256):
			planned = append(planned, Planned{
				Label: label, URL: e.URL, Action: PlanReplayStable,
				Reason: "content matches the recorded edition; publisher converges",
			})
		default:
			if owner, dup := hashLabels[hash]; dup {
				planned = append(planned, Planned{
					Label: label, URL: e.URL, Action: PlanAlias,
					Reason:  "republished bytes match another recorded edition",
					AliasOf: owner,
				})
			} else {
				planned = append(planned, Planned{
					Label: label, URL: e.URL, Action: PlanCorrect,
					Reason: "label republished with new bytes; superseding revision",
				})
			}
		}
	}
	return planned
}

func sameHash(hashes []string, want string) bool {
	for _, h := range hashes {
		if h != "" && strings.EqualFold(h, want) {
			return true
		}
	}
	return false
}
