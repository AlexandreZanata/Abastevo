package application_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
)

var lifecycleAt = time.Date(2026, 11, 14, 10, 30, 0, 123456789, time.UTC)

func lifecycleInput(command fleet.LifecycleCommand) app.AssetLifecycleInput {
	v := app.AssetLifecycleInput{Command: command, OperationID: id(10), AssetID: id(20), ExpectedRevision: 3}
	switch command {
	case fleet.RecordAcquisition:
		v.At = lifecycleAt
	case fleet.Deactivate, fleet.RevertDisposal:
		v.Reason = "Synthetic lifecycle reason"
	case fleet.Dispose:
		v.Reason, v.DisposalKind, v.At = "Synthetic sale to a synthetic buyer", fleet.Sale, lifecycleAt
	}
	return v
}

func TestLifecycleCommandShapeFollowsSpecificationRows(t *testing.T) {
	for _, command := range fleet.LifecycleCommands() {
		if _, e := app.NewAssetLifecycle(lifecycleInput(command)); e != nil {
			t.Fatal("valid lifecycle command refused", command, e)
		}
	}
	for _, kind := range []fleet.DisposalKind{fleet.Sale, fleet.WriteOff, fleet.ReturnToHolder} {
		v := lifecycleInput(fleet.Dispose)
		v.DisposalKind = kind
		if _, e := app.NewAssetLifecycle(v); e != nil {
			t.Fatal("disposal kind refused", kind, e)
		}
	}
	for name, edit := range map[string]func(*app.AssetLifecycleInput){
		"deactivate missing reason": func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Deactivate, "" },
		"dispose missing reason": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind, v.At = fleet.Dispose, "", fleet.Sale, lifecycleAt
		},
		"revert missing reason":  func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.RevertDisposal, "" },
		"blank reason":           func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Deactivate, "  " },
		"long reason":            func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Deactivate, strings.Repeat("a", 281) },
		"control reason":         func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Deactivate, "bad\nreason" },
		"invalid utf8 reason":    func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Deactivate, string([]byte{0xff}) },
		"reactivate with reason": func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Reactivate, "Unrequested reason" },
		"archive with reason":    func(v *app.AssetLifecycleInput) { v.Command, v.Reason = fleet.Archive, "Unrequested reason" },
		"acquisition with reason": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.At = fleet.RecordAcquisition, "Unrequested", lifecycleAt
		},
		"acquisition without instant": func(v *app.AssetLifecycleInput) {
			v.Command, v.At = fleet.RecordAcquisition, time.Time{}
		},
		"dispose without kind": func(v *app.AssetLifecycleInput) {
			v.DisposalKind = ""
			v.Command, v.Reason, v.At = fleet.Dispose, "Reason", lifecycleAt
		},
		"dispose unknown kind": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind, v.At = fleet.Dispose, "Reason", "THEFT", lifecycleAt
		},
		"dispose without instant": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind = fleet.Dispose, "Reason", fleet.Sale
		},
		"dispose year 10000": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind, v.At = fleet.Dispose, "R", fleet.Sale, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		},
		"deactivate with instant": func(v *app.AssetLifecycleInput) { v.Command, v.Reason, v.At = fleet.Deactivate, "Reason", lifecycleAt },
		"deactivate with kind": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind = fleet.Deactivate, "Reason", fleet.Sale
		},
		"revert with kind": func(v *app.AssetLifecycleInput) {
			v.Command, v.Reason, v.DisposalKind = fleet.RevertDisposal, "Reason", fleet.WriteOff
		},
		"archive with instant": func(v *app.AssetLifecycleInput) { v.Command, v.At = fleet.Archive, lifecycleAt },
		"unknown command":      func(v *app.AssetLifecycleInput) { v.Command = "ACTIVATE" },
		"operation":            func(v *app.AssetLifecycleInput) { v.OperationID = "not-a-uuid" },
		"asset":                func(v *app.AssetLifecycleInput) { v.AssetID = strings.ToUpper(id(20)) },
		"zero revision":        func(v *app.AssetLifecycleInput) { v.ExpectedRevision = 0 },
		"exhausted revision":   func(v *app.AssetLifecycleInput) { v.ExpectedRevision = math.MaxInt64 },
		"negative revision":    func(v *app.AssetLifecycleInput) { v.ExpectedRevision = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			v := lifecycleInput(fleet.Archive)
			edit(&v)
			if _, e := app.NewAssetLifecycle(v); !errors.Is(e, fleet.ErrInvalid) {
				t.Fatal("invalid lifecycle command accepted", e)
			}
		})
	}
}

func TestLifecycleDigestBindsExactIntentAndScope(t *testing.T) {
	s := scope(t)
	v := lifecycleInput(fleet.Dispose)
	command, e := app.NewAssetLifecycle(v)
	if e != nil {
		t.Fatal(e)
	}
	digest := command.Digest(s)
	if len(digest) != 64 || strings.Trim(digest, "0123456789abcdef") != "" {
		t.Fatal("lifecycle digest is not a lowercase SHA-256", digest)
	}
	v.Reason = "changed after capture"
	if command.Digest(s) != digest {
		t.Fatal("captured command escaped")
	}
	for name, edit := range map[string]func(*app.AssetLifecycleInput){
		"nanosecond": func(v *app.AssetLifecycleInput) { v.At = v.At.Add(time.Nanosecond) },
		"kind":       func(v *app.AssetLifecycleInput) { v.DisposalKind = fleet.WriteOff },
		"reason":     func(v *app.AssetLifecycleInput) { v.Reason += "." },
		"revision":   func(v *app.AssetLifecycleInput) { v.ExpectedRevision++ },
		"asset":      func(v *app.AssetLifecycleInput) { v.AssetID = id(21) },
		"operation":  func(v *app.AssetLifecycleInput) { v.OperationID = id(11) },
	} {
		changed := lifecycleInput(fleet.Dispose)
		edit(&changed)
		other, e := app.NewAssetLifecycle(changed)
		if e != nil || other.Digest(s) == digest {
			t.Fatal("digest ignored intent field", name, e)
		}
	}
	zoned := lifecycleInput(fleet.Dispose)
	zoned.At = lifecycleAt.In(time.FixedZone("BRT", -3*3600))
	same, e := app.NewAssetLifecycle(zoned)
	if e != nil || same.Digest(s) != digest {
		t.Fatal("the same instant in another zone changed intent", e)
	}
	org, _ := kernel.ParseOrgID(id(99))
	other, _ := kernel.NewOrgScope(org)
	if command.Digest(other) == digest || command.Digest(kernel.OrgScope{}) != "" {
		t.Fatal("digest not scoped")
	}
	acquisition, _ := app.NewAssetLifecycle(lifecycleInput(fleet.RecordAcquisition))
	deactivate, _ := app.NewAssetLifecycle(lifecycleInput(fleet.Deactivate))
	if acquisition.Digest(s) == deactivate.Digest(s) || (app.AssetLifecycle{}).Digest(s) != "" {
		t.Fatal("command kind or zero command not bound")
	}
}

func TestOpenOperationsNamesOnlyKnownDistinctKinds(t *testing.T) {
	kinds := []app.OpenWorkKind{app.OpenTrip, app.OpenWorkOrder}
	e := app.NewOpenOperations(kinds)
	var open *app.OpenOperationsError
	if !errors.As(e, &open) || !errors.Is(e, app.ErrOpenOperations) {
		t.Fatal("open work refusal lost its kinds", e)
	}
	kinds[0] = "SYNTHETIC"
	got := open.Kinds()
	if len(got) != 2 || got[0] != app.OpenTrip || got[1] != app.OpenWorkOrder {
		t.Fatal("open work kinds escaped or reordered", got)
	}
	got[0] = "MUTATED"
	if open.Kinds()[0] != app.OpenTrip {
		t.Fatal("open work kinds mutable")
	}
	for _, bad := range [][]app.OpenWorkKind{nil, {}, {app.OpenTrip, app.OpenTrip}, {"RESERVATION"}, {"trip"}, {app.OpenTrip, app.OpenWorkOrder, "X"}} {
		if e := app.NewOpenOperations(bad); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("unknown or duplicate open work kind accepted", bad, e)
		}
	}
}

type lifecycleRepository struct {
	transitions, histories int
	member                 string
	result                 app.AssetResult
	replayed               bool
	page                   app.LifecyclePage
	err                    error
}

func (r *lifecycleRepository) TransitionAsset(_ context.Context, _ kernel.OrgScope, _ app.FleetActor, member string, _ app.AssetLifecycle) (app.AssetResult, bool, error) {
	r.transitions++
	r.member = member
	return r.result, r.replayed, r.err
}
func (r *lifecycleRepository) ListAssetLifecycle(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.LifecycleFilter) (app.LifecyclePage, error) {
	r.histories++
	return r.page, r.err
}

func TestLifecycleServiceRequiresCurrentManageProofAndExactResults(t *testing.T) {
	command, e := app.NewAssetLifecycle(lifecycleInput(fleet.Dispose))
	if e != nil {
		t.Fatal(e)
	}
	s := scope(t)
	actor := app.FleetActor{AccountID: id(2), SessionID: id(3)}
	if _, _, e := (app.AssetLifecycleService{}).Transition(t.Context(), s, actor, access.Decision{}, command); !errors.Is(e, app.ErrSurfaceDisabled) {
		t.Fatal("default lifecycle surface enabled", e)
	}
	repo := &lifecycleRepository{result: app.AssetResult{AssetID: id(20), State: fleet.Disposed, Revision: 4}}
	if _, _, e := (app.AssetLifecycleService{Enabled: true, Repository: repo}).Transition(t.Context(), s, actor, access.Decision{}, command); !errors.Is(e, app.ErrUnauthenticated) || repo.transitions != 0 {
		t.Fatal("forged decision reached lifecycle storage", e)
	}
	ctx, finish, s, actor, read := decision(t, id(20), false)
	defer finish()
	if _, _, e := (app.AssetLifecycleService{Enabled: true, Repository: repo}).Transition(ctx, s, actor, read, command); !errors.Is(e, app.ErrUnauthenticated) || repo.transitions != 0 {
		t.Fatal("read decision admitted a lifecycle write", e)
	}
	ctx, finish2, s, actor, write := decision(t, id(20), true)
	defer finish2()
	service := app.AssetLifecycleService{Enabled: true, Repository: repo}
	if out, replayed, e := service.Transition(ctx, s, actor, write, command); e != nil || replayed || out.State != fleet.Disposed || out.Revision != 4 || repo.member != id(4) {
		t.Fatal("valid disposal result refused", e)
	}
	for name, bad := range map[string]app.AssetResult{
		"other asset":    {AssetID: id(21), State: fleet.Disposed, Revision: 4},
		"skipped":        {AssetID: id(20), State: fleet.Disposed, Revision: 5},
		"wrong target":   {AssetID: id(20), State: fleet.Inactive, Revision: 4},
		"discarded":      {AssetID: id(20), Revision: 4, Discarded: true},
		"unknown state":  {AssetID: id(20), State: "SOLD", Revision: 4},
		"missing asset":  {State: fleet.Disposed, Revision: 4},
		"zero revision":  {AssetID: id(20), State: fleet.Disposed},
		"archived early": {AssetID: id(20), State: fleet.Archived, Revision: 4},
	} {
		repo.result = bad
		if _, _, e := service.Transition(ctx, s, actor, write, command); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("invented lifecycle result accepted", name, e)
		}
	}
	acquisition, _ := app.NewAssetLifecycle(lifecycleInput(fleet.RecordAcquisition))
	for _, state := range []fleet.State{fleet.Draft, fleet.Active, fleet.Inactive} {
		repo.result = app.AssetResult{AssetID: id(20), State: state, Revision: 4}
		if _, _, e := service.Transition(ctx, s, actor, write, acquisition); e != nil {
			t.Fatal("acquisition keeps its source state", state, e)
		}
	}
	repo.result = app.AssetResult{AssetID: id(20), State: fleet.Disposed, Revision: 4}
	if _, _, e := service.Transition(ctx, s, actor, write, acquisition); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("acquisition result on a disposed asset accepted", e)
	}
	repo.err = app.NewOpenOperations([]app.OpenWorkKind{app.OpenWorkOrder})
	var open *app.OpenOperationsError
	if _, _, e := service.Transition(ctx, s, actor, write, command); !errors.As(e, &open) || open.Kinds()[0] != app.OpenWorkOrder {
		t.Fatal("open work refusal not propagated", e)
	}
	repo.err = app.ErrConflict
	if _, _, e := service.Transition(ctx, s, actor, write, command); !errors.Is(e, app.ErrConflict) {
		t.Fatal("conflict not propagated", e)
	}
	if _, _, e := (app.AssetLifecycleService{Enabled: true}).Transition(ctx, s, actor, write, command); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("missing lifecycle repository", e)
	}
}

func lifecycleEntry(revision int64, command fleet.LifecycleCommand, from, to fleet.State) app.LifecycleEntry {
	entry := app.LifecycleEntry{Revision: revision, Command: command, From: from, To: to, RecordedAt: lifecycleAt.Add(time.Duration(revision) * time.Minute)}
	if fleet.RequiresReason(command) {
		entry.Reason = "Synthetic history reason"
	}
	if command == fleet.Dispose {
		at := lifecycleAt
		entry.DisposalKind, entry.DisposedAt = fleet.WriteOff, &at
	}
	return entry
}

func TestLifecycleHistoryReadValidatesEveryEntryAndKeyset(t *testing.T) {
	ctx, finish, s, actor, read := decision(t, id(20), false)
	defer finish()
	if e := (app.LifecycleFilter{Limit: 0}).Validate(); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("unbounded history filter accepted")
	}
	if e := (app.LifecycleFilter{Limit: 101}).Validate(); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("oversized history page accepted")
	}
	if e := (app.LifecycleFilter{Limit: 2, AfterRevision: -1}).Validate(); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("negative keyset accepted")
	}
	history := []app.LifecycleEntry{lifecycleEntry(3, fleet.Deactivate, fleet.Active, fleet.Inactive), lifecycleEntry(4, fleet.Dispose, fleet.Inactive, fleet.Disposed), lifecycleEntry(5, fleet.RevertDisposal, fleet.Disposed, fleet.Inactive)}
	repo := &lifecycleRepository{page: app.LifecyclePage{Entries: history[:2], NextRevision: 4}}
	service := app.AssetLifecycleService{Enabled: true, Repository: repo}
	page, e := service.History(ctx, s, actor, read, id(20), app.LifecycleFilter{Limit: 2})
	if e != nil || len(page.Entries) != 2 || page.NextRevision != 4 || page.Entries[1].DisposedAt == nil || !page.Entries[1].DisposedAt.Equal(lifecycleAt) {
		t.Fatal("valid lifecycle history refused", e)
	}
	*page.Entries[1].DisposedAt = time.Time{}
	if !history[1].DisposedAt.Equal(lifecycleAt) {
		t.Fatal("history entry escaped the repository copy")
	}
	for name, bad := range map[string]app.LifecyclePage{
		"impossible transition": {Entries: []app.LifecycleEntry{lifecycleEntry(3, fleet.Archive, fleet.Active, fleet.Archived)}},
		"dispose without data":  {Entries: []app.LifecycleEntry{{Revision: 3, Command: fleet.Dispose, From: fleet.Active, To: fleet.Disposed, Reason: "R", RecordedAt: lifecycleAt}}},
		"reason on archive":     {Entries: []app.LifecycleEntry{{Revision: 3, Command: fleet.Archive, From: fleet.Disposed, To: fleet.Archived, Reason: "R", RecordedAt: lifecycleAt}}},
		"descending":            {Entries: []app.LifecycleEntry{history[1], history[0]}},
		"next without full":     {Entries: history[:1], NextRevision: 3},
		"next not last":         {Entries: history[:2], NextRevision: 3},
		"oversized":             {Entries: history},
		"acquisition row":       {Entries: []app.LifecycleEntry{lifecycleEntry(3, fleet.RecordAcquisition, fleet.Active, fleet.Active)}},
		"unset recording":       {Entries: []app.LifecycleEntry{{Revision: 3, Command: fleet.Reactivate, From: fleet.Inactive, To: fleet.Active}}},
		"revision one":          {Entries: []app.LifecycleEntry{lifecycleEntry(1, fleet.Deactivate, fleet.Active, fleet.Inactive)}},
	} {
		repo.page = bad
		if _, e := service.History(ctx, s, actor, read, id(20), app.LifecycleFilter{Limit: 2}); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("invalid lifecycle history accepted", name, e)
		}
	}
	repo.page = app.LifecyclePage{Entries: history[2:]}
	if _, e := service.History(ctx, s, actor, read, id(20), app.LifecycleFilter{Limit: 2, AfterRevision: 5}); !errors.Is(e, app.ErrUnavailable) {
		t.Fatal("entry at or before the keyset accepted")
	}
	if _, e := service.History(ctx, s, actor, read, "bad", app.LifecycleFilter{Limit: 2}); !errors.Is(e, fleet.ErrInvalid) {
		t.Fatal("invalid asset selector reached history")
	}
	before := repo.histories
	if _, e := service.History(ctx, s, actor, read, id(21), app.LifecycleFilter{Limit: 2}); !errors.Is(e, app.ErrUnauthenticated) || repo.histories != before {
		t.Fatal("history decision rebound to another asset", e)
	}
}

func TestAssetViewCarriesCurrentAcquisitionAndDisposalConsistently(t *testing.T) {
	ctx, finish, s, actor, read := decision(t, id(20), false)
	defer finish()
	at := lifecycleAt
	disposed := view(t, s, 20)
	disposed.State, disposed.Revision = fleet.Disposed, 5
	disposed.AcquiredAt = &at
	disposed.Disposal = &app.AssetDisposal{Kind: fleet.ReturnToHolder, At: lifecycleAt.Add(time.Hour), Reason: "Synthetic lease end"}
	repo := &repository{view: disposed}
	service := app.AssetService{Enabled: true, Repository: repo}
	got, e := service.Read(ctx, s, actor, read, id(20))
	if e != nil || got.Disposal == nil || got.Disposal.Kind != fleet.ReturnToHolder || got.AcquiredAt == nil || !got.AcquiredAt.Equal(at) {
		t.Fatal("disposed asset unreadable", e)
	}
	got.Disposal.Reason = "mutated"
	*got.AcquiredAt = time.Time{}
	if disposed.Disposal.Reason != "Synthetic lease end" || !disposed.AcquiredAt.Equal(lifecycleAt) {
		t.Fatal("asset view lifecycle data escaped its clone")
	}
	for name, edit := range map[string]func(*app.AssetView){
		"disposed without disposal": func(v *app.AssetView) { v.Disposal = nil },
		"archived without disposal": func(v *app.AssetView) { v.State, v.Disposal = fleet.Archived, nil },
		"inactive with disposal":    func(v *app.AssetView) { v.State = fleet.Inactive },
		"unknown kind":              func(v *app.AssetView) { v.Disposal = &app.AssetDisposal{Kind: "THEFT", At: lifecycleAt, Reason: "R"} },
		"blank reason":              func(v *app.AssetView) { v.Disposal = &app.AssetDisposal{Kind: fleet.Sale, At: lifecycleAt} },
		"zero instant":              func(v *app.AssetView) { v.Disposal = &app.AssetDisposal{Kind: fleet.Sale, Reason: "R"} },
		"zero acquisition":          func(v *app.AssetView) { zero := time.Time{}; v.AcquiredAt = &zero },
	} {
		bad := view(t, s, 20)
		bad.State, bad.Revision = fleet.Disposed, 5
		bad.Disposal = &app.AssetDisposal{Kind: fleet.Sale, At: lifecycleAt, Reason: "R"}
		edit(&bad)
		repo.view = bad
		if _, e := service.Read(ctx, s, actor, read, id(20)); !errors.Is(e, app.ErrUnavailable) {
			t.Fatal("inconsistent lifecycle view accepted", name, e)
		}
	}
}
