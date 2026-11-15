package domain_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
)

// BR-FLEET-033 (B-BR-EMP-325/328): only an IMPLEMENT attaches, only to a
// VEHICLE or MACHINE, and an IMPLEMENT is never a trip's main asset (AC4).
func TestAttachmentKindsAndMainAssetEligibility(t *testing.T) {
	kinds := []fleet.Kind{fleet.Vehicle, fleet.Machine, fleet.Implement}
	for _, implement := range kinds {
		for _, powered := range kinds {
			err := fleet.ValidAttachmentKinds(implement, powered)
			ok := implement == fleet.Implement && powered != fleet.Implement
			if ok != (err == nil) || !ok && !errors.Is(err, fleet.ErrInvalid) {
				t.Fatal("attachment kinds", implement, powered, err)
			}
		}
	}
	if err := fleet.ValidAttachmentKinds(fleet.Implement, "TRAILER"); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("unknown powered kind", err)
	}
	for kind, want := range map[fleet.Kind]error{fleet.Vehicle: nil, fleet.Machine: nil, fleet.Implement: fleet.ErrImplementMainAsset, "": fleet.ErrInvalid, "TRAILER": fleet.ErrInvalid} {
		if err := fleet.MainAssetAllowed(kind); !errors.Is(err, want) && !(want == nil && err == nil) {
			t.Fatal("main asset eligibility", kind, err)
		}
	}
}

// B-BR-EMP-326: creation and detach periods (detach before attach refused).
func TestAttachmentPeriodsAndDetach(t *testing.T) {
	now := resAt(0)
	open, _ := fleet.NewAssignmentPeriod(resAt(0), nil)
	if err := fleet.ValidNewAttachment(open, now); err != nil {
		t.Fatal("attachment from now", err)
	}
	if err := fleet.ValidNewAttachment(open, resAt(1)); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("retroactive attachment", err)
	}
	empty, _ := fleet.NewAssignmentPeriod(resAt(10), ptrAt(10))
	if err := fleet.ValidNewAttachment(empty, now); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("empty attachment", err)
	}
	started, _ := fleet.NewAssignmentPeriod(resAt(-30), nil)
	ended, err := fleet.DetachAt(started, nil, now.Add(1500*time.Nanosecond))
	if err != nil || ended.To == nil || !ended.To.Equal(now.Add(time.Microsecond)) {
		t.Fatal("detach now floors to the microsecond", ended, err)
	}
	if ended, err := fleet.DetachAt(started, ptrAt(20), now); err != nil || !ended.To.Equal(resAt(20)) {
		t.Fatal("planned detach", err)
	}
	scheduled, _ := fleet.NewAssignmentPeriod(resAt(30), ptrAt(90))
	for name, end := range map[string]*time.Time{"before the attach instant": ptrAt(29), "in the past": ptrAt(-1), "after the planned end": ptrAt(91), "inexact": ptrPlus(resAt(40), time.Nanosecond)} {
		if _, err := fleet.DetachAt(scheduled, end, now); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("detach", name, err)
		}
	}
	if never, err := fleet.DetachAt(scheduled, ptrAt(30), now); err != nil || !never.Empty() {
		t.Fatal("detach at the start of a scheduled attachment", err)
	}
	if never, err := fleet.DetachAt(scheduled, nil, now); err != nil || !never.Empty() {
		t.Fatal("detach a scheduled attachment now", err)
	}
	finished, _ := fleet.NewAssignmentPeriod(resAt(-30), ptrAt(-10))
	if _, err := fleet.DetachAt(finished, nil, now); !errors.Is(err, fleet.ErrAttachmentEnded) {
		t.Fatal("detach an ended attachment", err)
	}
}

// AC3 (B-BR-EMP-327): an implement is IN_USE through its powered asset's trip,
// with the powered asset named on the cause.
func TestAttachedImplementFollowsItsPoweredAsset(t *testing.T) {
	at := resAt(20)
	facts := fleet.AvailabilityFacts{Lifecycle: fleet.Active, Uses: []fleet.UseFact{{ID: "trip-1", From: resAt(1), Via: "tractor-1"}}}
	got, err := fleet.Assess(facts, at, at)
	if err != nil || got.Availability != fleet.InUse || len(got.Causes) != 1 || got.Causes[0].Via != "tractor-1" || got.Causes[0].ID != "trip-1" {
		t.Fatal("implement through its powered asset", got, err)
	}
	facts.Uses[0].Via = " bad"
	if _, err := fleet.Assess(facts, at, at); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("malformed powered asset", err)
	}
}

func ptrAt(minutes int) *time.Time { v := resAt(minutes); return &v }
func ptrPlus(at time.Time, d time.Duration) *time.Time {
	v := at.Add(d)
	return &v
}
