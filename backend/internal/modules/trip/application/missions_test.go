package application_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func respond(t *testing.T, response string, reason any, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return operation(t, app.MissionRespondOperationType, map[string]any{"response": response, "reason": reason}, func(e, p map[string]any) {
		e["entity"] = map[string]any{"type": app.MissionEntityType, "id": id(60), "expected_revision": 2}
		if change != nil {
			change(e, p)
		}
	})
}

// EDR-229: the response envelope owns the mission and its expected
// revision; the payload is exactly the response and its reason.
func TestParseRespondIsExactAndMalformedIsFinal(t *testing.T) {
	accept, err := app.ParseRespond(respond(t, "ACCEPT", nil, nil))
	v, _ := accept.Snapshot()
	if err != nil || v.MissionID != id(60) || v.ExpectedRevision != 2 || v.Response != trip.ResponseAccept || v.Reason != nil || v.AuthorMembershipID != id(3) {
		t.Fatal("accept", err)
	}
	decline, err := app.ParseRespond(respond(t, "DECLINE", "Veiculo em manutencao", nil))
	v, _ = decline.Snapshot()
	if err != nil || v.Response != trip.ResponseDecline || v.Reason == nil || *v.Reason != "Veiculo em manutencao" {
		t.Fatal("decline", err)
	}
	*v.Reason = "changed"
	if again, _ := decline.Snapshot(); *again.Reason != "Veiculo em manutencao" {
		t.Fatal("snapshot shares its reason")
	}
	for name, op := range map[string]protocol.Operation{
		"decline-without-reason": respond(t, "DECLINE", nil, nil),
		"blank-reason":           respond(t, "DECLINE", "   ", nil),
		"accept-with-reason":     respond(t, "ACCEPT", "Motivo", nil),
		"unknown-response":       respond(t, "ACCEPTED", nil, nil),
		"lowercase-response":     respond(t, "accept", nil, nil),
		"extra-key":              respond(t, "ACCEPT", nil, func(_, p map[string]any) { p["trip_id"] = id(40) }),
		"missing-reason-key":     respond(t, "ACCEPT", nil, func(_, p map[string]any) { delete(p, "reason") }),
		"no-expected-revision": respond(t, "ACCEPT", nil, func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.MissionEntityType, "id": id(60), "expected_revision": nil}
		}),
		"other-entity": respond(t, "ACCEPT", nil, func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "trip", "id": id(60), "expected_revision": 2}
		}),
		"dependency": respond(t, "ACCEPT", nil, func(e, _ map[string]any) { e["depends_on"] = []any{id(19)} }),
		"other-type": operation(t, app.StartOperationType, map[string]any{"response": "ACCEPT", "reason": nil}, nil),
	} {
		if _, err := app.ParseRespond(op); !errors.Is(err, app.ErrMalformed) {
			t.Fatal("malformed response admitted", name, err)
		}
	}
	if _, err := (app.RespondCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command", err)
	}
}

// EDR-231: fixed policies; the response and the own list are trip.trip.start.
func TestMissionPoliciesAreFixed(t *testing.T) {
	for action, want := range map[app.MissionAction]access.Policy{
		app.MissionRead:    {Permission: "trip.mission.manage", Capability: "organizations", Effect: access.Read},
		app.MissionManage:  {Permission: "trip.mission.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true},
		app.MissionOwnRead: {Permission: "trip.trip.start", Capability: "organizations", Effect: access.Read},
		app.MissionReceipt: {Permission: "trip.trip.start", Capability: "organizations", Effect: access.Read},
		app.MissionRespond: {Permission: "trip.trip.start", Capability: "organizations", Effect: access.OperationalWrite},
	} {
		if got := app.MissionPolicy(action); got != want || got.Validate() != nil {
			t.Fatal("mission policy drift", action, got)
		}
	}
	if app.MissionPolicy(0) != (access.Policy{}) {
		t.Fatal("unknown action has a policy")
	}
}

// B-BR-EMP-353/354: each command carries exactly its own fields, validated
// by the pure rules; the digest binds every field and the organization.
func TestMissionCommandsAreExactPerKind(t *testing.T) {
	start := time.Date(2026, 11, 20, 11, 0, 0, 0, time.UTC)
	create := app.MissionCommandInput{Kind: app.MissionCreate, OperationID: id(1), UnitID: id(2), AssetID: id(3), Title: "Entrega", StartsAt: start, EndsAt: start.Add(time.Hour)}
	assign := app.MissionCommandInput{Kind: app.MissionAssignCommand, OperationID: id(1), MissionID: id(4), ExpectedRevision: 1, AssigneeEmployeeID: id(5)}
	cancel := app.MissionCommandInput{Kind: app.MissionCancelCommand, OperationID: id(1), MissionID: id(4), ExpectedRevision: 2, Reason: "Cliente cancelou"}
	for name, in := range map[string]app.MissionCommandInput{"create": create, "assign": assign, "cancel": cancel} {
		if _, err := app.NewMissionCommand(in); err != nil {
			t.Fatal("valid command refused", name, err)
		}
	}
	bad := map[string]app.MissionCommandInput{}
	edit := func(name string, base app.MissionCommandInput, change func(*app.MissionCommandInput)) {
		change(&base)
		bad[name] = base
	}
	edit("create-with-revision", create, func(v *app.MissionCommandInput) { v.ExpectedRevision = 1 })
	edit("create-with-mission", create, func(v *app.MissionCommandInput) { v.MissionID = id(4) })
	edit("create-without-unit", create, func(v *app.MissionCommandInput) { v.UnitID = "" })
	edit("create-blank-title", create, func(v *app.MissionCommandInput) { v.Title = " " })
	edit("create-reversed-window", create, func(v *app.MissionCommandInput) { v.EndsAt = start.Add(-time.Hour) })
	edit("create-sub-microsecond", create, func(v *app.MissionCommandInput) { v.StartsAt = start.Add(time.Nanosecond) })
	edit("assign-without-employee", assign, func(v *app.MissionCommandInput) { v.AssigneeEmployeeID = "" })
	edit("assign-revision-zero", assign, func(v *app.MissionCommandInput) { v.ExpectedRevision = 0 })
	edit("assign-with-reason", assign, func(v *app.MissionCommandInput) { v.Reason = "x" })
	edit("assign-with-title", assign, func(v *app.MissionCommandInput) { v.Title = "x" })
	edit("cancel-without-reason", cancel, func(v *app.MissionCommandInput) { v.Reason = "" })
	edit("cancel-with-employee", cancel, func(v *app.MissionCommandInput) { v.AssigneeEmployeeID = id(5) })
	edit("bad-operation", cancel, func(v *app.MissionCommandInput) { v.OperationID = "x" })
	edit("unknown-kind", cancel, func(v *app.MissionCommandInput) { v.Kind = "PLAN" })
	for name, in := range bad {
		if _, err := app.NewMissionCommand(in); !errors.Is(err, trip.ErrInvalid) {
			t.Fatal("invalid command admitted", name, err)
		}
	}
	scope := missionScope(t, 1)
	c, _ := app.NewMissionCommand(cancel)
	other := cancel
	other.Reason = "Outro motivo"
	o, _ := app.NewMissionCommand(other)
	if d := c.Digest(scope); len(d) != 64 || d == o.Digest(scope) || d != c.Digest(scope) || d == c.Digest(missionScope(t, 2)) {
		t.Fatal("digest does not bind the command and the organization")
	}
}

func missionScope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(n))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// EMP-TRIP-009 (B-BR-EMP-382, EDR-249): PLAN carries exactly its plan; the
// command repeats the domain plan profile without a scope; its digest binds
// every plan field while CREATE, ASSIGN and CANCEL keep the EMP-TRIP-008
// digest form, so receipts stored before 000231 replay unchanged.
func TestMissionPlanCommandIsExactAndDigestStable(t *testing.T) {
	text := func(v string) *string { return &v }
	count := func(v int) *int { return &v }
	plan := app.MissionPlanFields{DestinationText: text("Centro de distribuição Norte"), Reason: "Entrega de peças", CostCentreID: id(6), Load: "CARGO", CargoDescription: text("Peças")}
	base := app.MissionCommandInput{Kind: app.MissionPlanCommand, OperationID: id(1), MissionID: id(4), ExpectedRevision: 2, Plan: &plan}
	valid := map[string]app.MissionPlanFields{
		"text-cargo":      plan,
		"unit-passengers": {DestinationUnitID: id(7), Reason: "Transporte da equipe", Load: "PASSENGERS", Passengers: count(12)},
		"none":            {DestinationText: text("Pátio"), Reason: "Vistoria", Load: "NONE"},
		"999":             {DestinationText: text("Pátio"), Reason: "Evento", Load: "PASSENGERS", Passengers: count(999)},
	}
	for name, p := range valid {
		in := base
		in.Plan = &p
		if _, err := app.NewMissionCommand(in); err != nil {
			t.Fatal("valid plan refused", name, err)
		}
	}
	bad := map[string]func(*app.MissionCommandInput){
		"no-plan":              func(v *app.MissionCommandInput) { v.Plan = nil },
		"plan-on-cancel":       func(v *app.MissionCommandInput) { v.Kind, v.Reason = app.MissionCancelCommand, "Motivo" },
		"revision-zero":        func(v *app.MissionCommandInput) { v.ExpectedRevision = 0 },
		"with-unit":            func(v *app.MissionCommandInput) { v.UnitID = id(2) },
		"with-assignee":        func(v *app.MissionCommandInput) { v.AssigneeEmployeeID = id(5) },
		"with-reason":          func(v *app.MissionCommandInput) { v.Reason = "x" },
		"both-destinations":    func(v *app.MissionCommandInput) { v.Plan.DestinationUnitID = id(7) },
		"no-destination":       func(v *app.MissionCommandInput) { v.Plan.DestinationText = nil },
		"non-canonical-unit":   func(v *app.MissionCommandInput) { v.Plan.DestinationText, v.Plan.DestinationUnitID = nil, "X" },
		"non-canonical-centre": func(v *app.MissionCommandInput) { v.Plan.CostCentreID = "centre" },
		"cpf-destination":      func(v *app.MissionCommandInput) { v.Plan.DestinationText = text("Casa 12345678909") },
		"cpf-reason":           func(v *app.MissionCommandInput) { v.Plan.Reason = "Cliente 123.456.789-09" },
		"cpf-cargo":            func(v *app.MissionCommandInput) { v.Plan.CargoDescription = text("Pacote 12345678909") },
		"blank-reason":         func(v *app.MissionCommandInput) { v.Plan.Reason = " " },
		"unknown-load":         func(v *app.MissionCommandInput) { v.Plan.Load = "BOTH" },
		"cargo-without-text":   func(v *app.MissionCommandInput) { v.Plan.CargoDescription = nil },
		"cargo-with-count":     func(v *app.MissionCommandInput) { v.Plan.Passengers = count(2) },
		"none-with-text":       func(v *app.MissionCommandInput) { v.Plan.Load = "NONE" },
		"passengers-zero": func(v *app.MissionCommandInput) {
			v.Plan.Load, v.Plan.CargoDescription, v.Plan.Passengers = "PASSENGERS", nil, count(0)
		},
		"passengers-1000": func(v *app.MissionCommandInput) {
			v.Plan.Load, v.Plan.CargoDescription, v.Plan.Passengers = "PASSENGERS", nil, count(1000)
		},
		"passengers-description":   func(v *app.MissionCommandInput) { v.Plan.Load, v.Plan.Passengers = "PASSENGERS", count(2) },
		"passengers-without-count": func(v *app.MissionCommandInput) { v.Plan.Load, v.Plan.CargoDescription = "PASSENGERS", nil },
		"unknown-kind":             func(v *app.MissionCommandInput) { v.Kind = "START" },
	}
	for name, change := range bad {
		in := base
		p := plan
		in.Plan = &p
		change(&in)
		if _, err := app.NewMissionCommand(in); !errors.Is(err, trip.ErrInvalid) {
			t.Fatal("invalid plan command admitted", name, err)
		}
	}
	scope := missionScope(t, 1)
	c, err := app.NewMissionCommand(base)
	if err != nil {
		t.Fatal(err)
	}
	digests := map[string]bool{c.Digest(scope): true}
	for name, change := range map[string]func(*app.MissionPlanFields){
		"destination": func(p *app.MissionPlanFields) { p.DestinationText = text("Outro destino") },
		"unit":        func(p *app.MissionPlanFields) { p.DestinationText, p.DestinationUnitID = nil, id(7) },
		"reason":      func(p *app.MissionPlanFields) { p.Reason = "Outro motivo" },
		"centre":      func(p *app.MissionPlanFields) { p.CostCentreID = "" },
		"cargo":       func(p *app.MissionPlanFields) { p.CargoDescription = text("Outra carga") },
		"passengers":  func(p *app.MissionPlanFields) { p.Load, p.CargoDescription, p.Passengers = "PASSENGERS", nil, count(3) },
	} {
		p := plan
		change(&p)
		in := base
		in.Plan = &p
		other, err := app.NewMissionCommand(in)
		if err != nil {
			t.Fatal(name, err)
		}
		if d := other.Digest(scope); len(d) != 64 || digests[d] {
			t.Fatal("plan digest does not bind", name)
		}
		digests[other.Digest(scope)] = true
	}
	if c.Digest(missionScope(t, 2)) == c.Digest(scope) {
		t.Fatal("plan digest does not bind the organization")
	}
	plan.Reason = "changed after validation"
	if again, _ := c.Snapshot(); again.Plan.Reason != "Entrega de peças" {
		t.Fatal("the command shares the caller's plan")
	}
	cancel := app.MissionCommandInput{Kind: app.MissionCancelCommand, OperationID: id(1), MissionID: id(4), ExpectedRevision: 2, Reason: "Cliente cancelou"}
	cc, _ := app.NewMissionCommand(cancel)
	raw, _ := json.Marshal([]any{"trip.mission.command.v1", scope.OrgID().String(), app.MissionCancelCommand, id(1), id(4), int64(2), "", "", "", "", "", "", "Cliente cancelou"})
	if sum := sha256.Sum256(raw); cc.Digest(scope) != hex.EncodeToString(sum[:]) {
		t.Fatal("the EMP-TRIP-008 digest form changed")
	}
}
