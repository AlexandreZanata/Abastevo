package domain_test

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

const ownID = "7b0e2a54-0000-4000-8000-000000000001"
const foreignID = "7b0e2a54-0000-4000-8000-000000000002"

func snapshot(t *testing.T, o org.Organization, s kernel.OrgScope) org.Snapshot {
	t.Helper()
	v, err := o.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func pending(t *testing.T) (org.Organization, kernel.OrgScope) {
	t.Helper()
	s := scope(t, ownID)
	o, err := org.NewPending(s, org.Profile{Name: "Synthetic private company", TimeZone: "America/Cuiaba", Kinds: []org.Kind{org.Farm}})
	if err != nil {
		t.Fatal(err)
	}
	return o, s
}
func ready() org.Conditions {
	return org.Conditions{ActiveUnits: 1, ActiveOwners: 1, TermsRecorded: true}
}
func transition(t *testing.T, o org.Organization, s kernel.OrgScope, to org.State, facts org.Conditions) org.Organization {
	t.Helper()
	n, err := o.Transition(s, snapshot(t, o, s).Revision, to, facts)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func TestPendingCreationAndConfigurationDoNotActivateOrInferRelatedEntities(t *testing.T) {
	s := scope(t, ownID)
	o, err := org.NewPending(s, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	first := snapshot(t, o, s)
	if first.State != org.PendingSetup || first.Revision != 1 || first.ID != s.OrgID() || len(first.Profile.Kinds) != 0 {
		t.Fatal("invalid pending root")
	}
	profile := org.Profile{Name: "EX.EMP.L01/0001-70", TimeZone: "America/Cuiaba", Kinds: []org.Kind{org.Farm, org.Carrier, org.Farm}}
	configured, err := o.ConfigurePending(s, 1, profile)
	if err != nil {
		t.Fatal(err)
	}
	profile.Kinds[0] = org.Other
	after := snapshot(t, configured, s)
	if after.State != org.PendingSetup || after.Revision != 2 || !reflect.DeepEqual(after.Profile.Kinds, []org.Kind{org.Carrier, org.Farm}) {
		t.Fatal("configuration inferred state/default")
	}
	// A second organization may have identical descriptions without sharing an ID or state.
	other := scope(t, foreignID)
	duplicate, err := org.NewPending(other, after.Profile)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot(t, duplicate, other).ID == after.ID {
		t.Fatal("description became identity")
	}
	if !reflect.DeepEqual(first, snapshot(t, o, s)) {
		t.Fatal("configuration mutated its input")
	}
	active := transition(t, configured, s, org.Active, ready())
	if _, err := active.ConfigurePending(s, 3, after.Profile); !errors.Is(err, org.ErrTransition) {
		t.Fatal(err)
	}
}

func TestScopesZeroValuesAndStaleRevisionsStopBeforeReturningDataOrChanges(t *testing.T) {
	o, s := pending(t)
	foreign := scope(t, foreignID)
	before := snapshot(t, o, s)
	for _, wrong := range []kernel.OrgScope{{}, foreign} {
		if _, err := o.Snapshot(wrong); !errors.Is(err, org.ErrScope) {
			t.Fatal(err)
		}
		if _, err := o.Transition(wrong, 1, org.Active, ready()); !errors.Is(err, org.ErrScope) {
			t.Fatal(err)
		}
		if _, err := o.ConfigurePending(wrong, 1, before.Profile); !errors.Is(err, org.ErrScope) {
			t.Fatal(err)
		}
		if _, err := org.Rehydrate(wrong, before); !errors.Is(err, org.ErrScope) {
			t.Fatal(err)
		}
	}
	if _, err := org.NewPending(kernel.OrgScope{}, before.Profile); !errors.Is(err, org.ErrScope) {
		t.Fatal(err)
	}
	if _, err := (org.Organization{}).Snapshot(s); !errors.Is(err, org.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := (org.Organization{}).Transition(s, 1, org.Active, ready()); !errors.Is(err, org.ErrInvalid) {
		t.Fatal(err)
	}
	for _, rev := range []int64{-1, 0, 2, math.MaxInt64} {
		if _, err := o.Transition(s, rev, org.Active, ready()); !errors.Is(err, org.ErrConflict) {
			t.Fatal(err)
		}
		if _, err := o.ConfigurePending(s, rev, before.Profile); !errors.Is(err, org.ErrConflict) {
			t.Fatal(err)
		}
	}
	max := before
	max.Revision = math.MaxInt64
	terminalRevision, err := org.Rehydrate(s, max)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := terminalRevision.ConfigurePending(s, math.MaxInt64, before.Profile); !errors.Is(err, org.ErrRevision) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, o, s)) {
		t.Fatal("failure changed input")
	}
}

func TestDefaultEvidenceCannotPerformAnyLifecycleEdgeAndClosureNeverResurrects(t *testing.T) {
	o, s := pending(t)
	if _, err := o.Transition(s, 1, org.Closed, org.Conditions{}); !errors.Is(err, org.ErrConditions) {
		t.Fatal(err)
	}
	active := transition(t, o, s, org.Active, ready())
	for _, target := range []org.State{org.Suspended, org.Archived} {
		if _, err := active.Transition(s, 2, target, org.Conditions{}); !errors.Is(err, org.ErrConditions) {
			t.Fatal(err)
		}
	}
	suspended := transition(t, active, s, org.Suspended, org.Conditions{SuspensionReason: org.Billing})
	if _, err := suspended.Transition(s, 3, org.Active, org.Conditions{}); !errors.Is(err, org.ErrConditions) {
		t.Fatal(err)
	}
	if _, err := suspended.Transition(s, 3, org.Archived, org.Conditions{}); !errors.Is(err, org.ErrConditions) {
		t.Fatal(err)
	}
	archived := transition(t, suspended, s, org.Archived, org.Conditions{ArchiveClear: true})
	for _, target := range []org.State{org.Active, org.Suspended, org.Closed} {
		if _, err := archived.Transition(s, 4, target, org.Conditions{}); !errors.Is(err, org.ErrConditions) {
			t.Fatal(err)
		}
	}
	closed := transition(t, archived, s, org.Closed, org.Conditions{Closure: org.ClosureElapsed})
	before := snapshot(t, closed, s)
	for _, target := range []org.State{org.PendingSetup, org.Active, org.Suspended, org.Archived, org.Closed, "UNKNOWN"} {
		if _, err := closed.Transition(s, 5, target, ready()); !errors.Is(err, org.ErrClosed) {
			t.Fatal(err)
		}
	}
	if _, err := closed.ConfigurePending(s, 5, before.Profile); !errors.Is(err, org.ErrClosed) {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, snapshot(t, closed, s)) {
		t.Fatal("closed mutated")
	}
}

func TestSnapshotProfileAndChecklistCopiesCannotMutateAggregateOrError(t *testing.T) {
	p := org.Profile{Name: "Synthetic", TimeZone: "UTC", Kinds: []org.Kind{org.Farm, org.Delivery}}
	s := scope(t, ownID)
	o, err := org.NewPending(s, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Kinds[0] = org.Other
	exposed := snapshot(t, o, s)
	exposed.Profile.Kinds[0] = org.Other
	again := snapshot(t, o, s)
	if !reflect.DeepEqual(again.Profile.Kinds, []org.Kind{org.Delivery, org.Farm}) {
		t.Fatal("snapshot alias")
	}
	restored, err := org.Rehydrate(s, again)
	if err != nil {
		t.Fatal(err)
	}
	again.Profile.Kinds[0] = org.Other
	if !reflect.DeepEqual(snapshot(t, restored, s).Profile.Kinds, []org.Kind{org.Delivery, org.Farm}) {
		t.Fatal("hydrate alias")
	}
	empty, err := org.NewPending(s, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = empty.Transition(s, 1, org.Active, org.Conditions{})
	var setup *org.SetupError
	if !errors.As(err, &setup) || !errors.Is(err, org.ErrSetup) {
		t.Fatal(err)
	}
	codes := setup.Missing()
	codes[0] = "UNTRUSTED"
	if setup.Missing()[0] != org.MissingName {
		t.Fatal("error alias")
	}
	if setup.Error() != org.ErrSetup.Error() {
		t.Fatal("unbounded error")
	}
}

func TestInvalidProfilesAndHydratedStatesCannotBypassActivation(t *testing.T) {
	o, s := pending(t)
	original := snapshot(t, o, s)
	for _, profile := range []org.Profile{
		{Name: string([]byte{255}), TimeZone: "UTC"},
		{Name: "Synthetic", TimeZone: "Mars/Olympus"},
		{Name: "Synthetic", TimeZone: "UTC", Kinds: []org.Kind{"UNKNOWN"}},
	} {
		if _, err := org.NewPending(s, profile); !errors.Is(err, org.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := o.ConfigurePending(s, 1, profile); !errors.Is(err, org.ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, state := range []org.State{org.Active, org.Suspended, org.Archived} {
		invalid := original
		invalid.State = state
		invalid.Profile = org.Profile{}
		if state == org.Suspended {
			invalid.SuspensionReason = org.Security
		}
		if _, err := org.Rehydrate(s, invalid); !errors.Is(err, org.ErrInvalid) {
			t.Fatal(err)
		}
	}
	// Cancelled/incomplete setup may be closed; CLOSED remains readable and terminal.
	invalid := original
	invalid.State = org.Closed
	invalid.Profile = org.Profile{}
	if _, err := org.Rehydrate(s, invalid); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentValueDecisionsAreDeterministicAndPrivateFormattingIsRedacted(t *testing.T) {
	o, s := pending(t)
	before := snapshot(t, o, s)
	want := snapshot(t, transition(t, o, s, org.Active, ready()), s)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			changed, err := o.Transition(s, 1, org.Active, ready())
			if err != nil {
				t.Error(err)
				return
			}
			got, err := changed.Snapshot(s)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Error("nondeterministic decision", err)
			}
			got.Profile.Kinds[0] = org.Other
		})
	}
	wg.Wait()
	if !reflect.DeepEqual(before, snapshot(t, o, s)) {
		t.Fatal("concurrent mutation")
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	logger.Info("domain", "organization", o, "snapshot", before, "profile", before.Profile)
	formatted := fmt.Sprintf("%v %+v %#v %v %#v %v %#v", o, o, o, before, before, before.Profile, before.Profile)
	for _, secret := range []string{ownID, before.Profile.Name, before.Profile.TimeZone} {
		if strings.Contains(logs.String()+formatted, secret) {
			t.Fatal("private value leaked")
		}
	}
}
