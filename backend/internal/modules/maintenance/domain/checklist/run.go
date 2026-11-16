package checklist

import (
	"slices"
	"time"
)

// EMP-CHECK-002 (B-BR-EMP-440): the server recheck of a completed run.

// VersionStamp is one published version of a template and its instant.
type VersionStamp struct {
	Version     int64
	PublishedAt time.Time
}

// RunFacts are the server's stored facts about the template a run names: the
// named version's target and items, every published version, the archive
// instant and the whole policy history.
type RunFacts struct {
	Context    Context
	Version    int64
	Target     Target
	Items      []Item
	Versions   []VersionStamp
	ArchivedAt *time.Time
	Policies   []PolicyRevision
}

// RunClaim is a completed run as the device declared it.
type RunClaim struct {
	Context                 Context
	AssetID, CategoryID     string
	Version, PolicyRevision int64
	StartedAt, CompletedAt  time.Time
	Responses               []Response
	DeviceOutcome           Outcome
	Acknowledged            bool
}

// Recheck is the server's answer: its evaluation under the policy revision in
// force at the completion instant, the device's outcome as declared and the
// two stored flags.
type Recheck struct {
	Evaluation     Evaluation
	PolicyRevision int64
	DeviceOutcome  Outcome
	Superseded     bool
	Differs        bool
}

// ParseOutcome reads one of the three run outcomes.
func ParseOutcome(raw string) (Outcome, error) {
	switch o := Outcome(raw); o {
	case CompletedOK, CompletedWithIssues, Blocking:
		return o, nil
	}
	return "", ErrInvalid
}

// covers reports whether a canonical target applies to the asset: the
// organization default, the asset itself or the asset's category.
func covers(target Target, assetID, categoryID string) bool {
	switch target.Kind {
	case TargetOrganization:
		return true
	case TargetAssets:
		return slices.Contains(target.IDs, assetID)
	}
	return slices.Contains(target.IDs, categoryID)
}

// RecheckRun recomputes a completed run from the server's facts
// (BR-CHECK-007). Every malformed input is ErrInvalid; the shared answer
// rule's refusals keep their own errors. The policy is the revision in force
// at the declared completion instant, or the first revision when the
// instant precedes it; the device's outcome never governs.
func RecheckRun(facts RunFacts, claim RunClaim) (Recheck, error) {
	if _, err := ParseContext(string(claim.Context)); err != nil || claim.Context != facts.Context || !CanonicalID(claim.AssetID) || !CanonicalID(claim.CategoryID) {
		return Recheck{}, ErrInvalid
	}
	if len(facts.Versions) == 0 || facts.Version < 1 || facts.Version > int64(len(facts.Versions)) || claim.Version != facts.Version {
		return Recheck{}, ErrInvalid
	}
	for i, v := range facts.Versions {
		if v.Version != int64(i+1) || !calendarInstant(v.PublishedAt) || i > 0 && v.PublishedAt.Before(facts.Versions[i-1].PublishedAt) {
			return Recheck{}, ErrInvalid
		}
	}
	target, err := canonicalTarget(facts.Target)
	if err != nil || !covers(target, claim.AssetID, claim.CategoryID) {
		return Recheck{}, ErrInvalid
	}
	if len(facts.Policies) == 0 || claim.PolicyRevision < 1 || claim.PolicyRevision > int64(len(facts.Policies)) {
		return Recheck{}, ErrInvalid
	}
	if claim.CompletedAt.IsZero() || !calendarInstant(claim.StartedAt) || claim.StartedAt.After(claim.CompletedAt) || facts.ArchivedAt != nil && !calendarInstant(*facts.ArchivedAt) {
		return Recheck{}, ErrInvalid
	}
	inForce, found, err := PolicyAt(facts.Policies, claim.CompletedAt)
	if err != nil {
		return Recheck{}, ErrInvalid
	}
	if !found {
		inForce = facts.Policies[0]
	}
	device, err := ParseOutcome(string(claim.DeviceOutcome))
	if err != nil || claim.Acknowledged && device != CompletedWithIssues {
		return Recheck{}, ErrInvalid
	}
	evaluation, err := Evaluate(facts.Items, inForce.Policy, claim.Responses)
	if err != nil {
		return Recheck{}, err
	}
	superseded := facts.ArchivedAt != nil && !facts.ArchivedAt.After(claim.CompletedAt)
	for _, v := range facts.Versions[claim.Version:] {
		superseded = superseded || !v.PublishedAt.After(claim.CompletedAt)
	}
	return Recheck{Evaluation: evaluation, PolicyRevision: inForce.Revision, DeviceOutcome: device, Superseded: superseded, Differs: device != evaluation.Outcome}, nil
}

// ValidNote reports a present item note: 1..140 scalars, nonblank, no control
// character (B-BR-EMP-439).
func ValidNote(raw string) bool { return validText(raw, MaxNoteScalars, true) }
