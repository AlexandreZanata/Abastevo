package application_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/incident"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
)

func uid(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

const origin = "https://api.example.invalid"

func reportPayload() map[string]any {
	return map[string]any{"report_type": "DAMAGE", "incident_kind": nil, "asset_id": uid(10), "trip_id": nil, "description": "Lanterna trincada", "location": nil, "blocks_use": false}
}

func reportOperation(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	body := reportPayload()
	envelope := map[string]any{"schema_version": 1, "operation_id": uid(20), "owner_account_id": uid(1), "api_origin": origin, "org_id": uid(65), "membership_id": uid(3),
		"enrollment_id": uid(4), "type": app.ReportOperationType, "operation_schema": 1, "entity": map[string]any{"type": app.ReportEntityType, "id": uid(40), "expected_revision": nil},
		"occurred_at": "2026-11-15T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{},
		"soft_references": []any{}, "attachments": []any{}, "payload": body}
	if change != nil {
		change(envelope, body)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(uid(1), origin, uid(65))
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("invalid unit operation fixture", err)
	}
	return op
}

// B-BR-EMP-369: the envelope owns identity, author and declaration; the
// payload carries exactly the bounded report.
func TestParseReportKeepsTheExactFact(t *testing.T) {
	op := reportOperation(t, func(e, p map[string]any) {
		p["report_type"], p["incident_kind"], p["trip_id"], p["location"], p["blocks_use"] = "INCIDENT", "THEFT", uid(41), "Patio da base", true
		e["depends_on"] = []any{uid(19)}
	})
	command, err := app.ParseReport(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.ReportID != uid(40) || v.AssetID != uid(10) || v.TripID != uid(41) || v.AuthorMembershipID != uid(3) || v.OwnerAccountID != uid(1) || v.OperationID != op.ID() ||
		v.PayloadDigest != op.PayloadDigest() || v.Report.Type != incident.Incident || v.Report.Kind != incident.Theft || !v.Report.BlocksUse || v.Report.Location == nil ||
		*v.Report.Location != "Patio da base" || v.Report.State != incident.Reported || v.Report.Revision != 1 || v.Declared.OccurredAt.Nanosecond()%1000 != 123 ||
		v.Declared.EntryZone != "America/Cuiaba" || len(v.DependsOn) != 1 {
		t.Fatal("parsed report lost its exact fact", err)
	}
	*v.Report.Location = "changed"
	v.DependsOn[0] = "changed"
	if again, _ := command.Snapshot(); *again.Report.Location != "Patio da base" || again.DependsOn[0] != uid(19) {
		t.Fatal("snapshot shares state")
	}
	if _, err := (app.ReportCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("zero command", err)
	}
}

// B-BR-EMP-369/370: shapes are final MALFORMED; an attachment declaration
// and a soft reference stay unavailable (EDR-238/239).
func TestParseReportRefusesMalformedAndKeepsAttachmentsUnavailable(t *testing.T) {
	for name, change := range map[string]func(e, p map[string]any){
		"damage-kind":       func(_, p map[string]any) { p["incident_kind"] = "ACCIDENT" },
		"incident-no-kind":  func(_, p map[string]any) { p["report_type"] = "INCIDENT" },
		"empty-kind":        func(_, p map[string]any) { p["report_type"], p["incident_kind"] = "INCIDENT", "" },
		"kind-not-string":   func(_, p map[string]any) { p["report_type"], p["incident_kind"] = "INCIDENT", 1 },
		"type-null":         func(_, p map[string]any) { p["report_type"] = nil },
		"unknown-type":      func(_, p map[string]any) { p["report_type"] = "FIRE" },
		"blank":             func(_, p map[string]any) { p["description"] = " " },
		"too-long":          func(_, p map[string]any) { p["description"] = strings.Repeat("a", 1001) },
		"description-null":  func(_, p map[string]any) { p["description"] = nil },
		"empty-location":    func(_, p map[string]any) { p["location"] = "" },
		"blocks-null":       func(_, p map[string]any) { p["blocks_use"] = nil },
		"blocks-string":     func(_, p map[string]any) { p["blocks_use"] = "false" },
		"asset-uppercase":   func(_, p map[string]any) { p["asset_id"] = strings.ToUpper(uid(10)) },
		"trip-not-uuid":     func(_, p map[string]any) { p["trip_id"] = "trip" },
		"extra-key":         func(_, p map[string]any) { p["amount_centavos"] = "1" },
		"missing-key":       func(_, p map[string]any) { delete(p, "location") },
		"entity-type":       func(e, _ map[string]any) { e["entity"].(map[string]any)["type"] = "incident" },
		"expected-revision": func(e, _ map[string]any) { e["entity"].(map[string]any)["expected_revision"] = 1 },
		"two-dependencies":  func(e, _ map[string]any) { e["depends_on"] = []any{uid(18), uid(19)} },
		"other-type":        func(e, _ map[string]any) { e["type"] = "check.damage.report" },
	} {
		if _, err := app.ParseReport(reportOperation(t, change)); !errors.Is(err, app.ErrMalformed) {
			t.Fatal("malformed report accepted", name, err)
		}
	}
	for name, change := range map[string]func(e, p map[string]any){
		"attachment": func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": uid(55), "sha256": strings.Repeat("a", 64), "bytes": 10, "mime": "image/jpeg"}}
		},
		"soft-reference": func(e, _ map[string]any) { e["soft_references"] = []any{map[string]any{"type": "trip", "id": uid(41)}} },
	} {
		if _, err := app.ParseReport(reportOperation(t, change)); !errors.Is(err, app.ErrUnsupported) {
			t.Fatal("recognised future part not kept unavailable", name, err)
		}
	}
}

// B-BR-EMP-371: the trip window, asset and author.
func TestTripWindowChecksAssetAuthorAndInstant(t *testing.T) {
	op := reportOperation(t, func(_, p map[string]any) { p["trip_id"] = uid(41) })
	command, err := app.ParseReport(op)
	if err != nil {
		t.Fatal(err)
	}
	in, _ := command.Snapshot()
	start := in.Declared.OccurredAt.Add(-time.Hour)
	record := tripapp.Record{ID: uid(41), AssetID: uid(10), AuthorMembershipID: uid(3), Declared: orgtime.Declaration{OccurredAt: start}}
	if err := app.TripWindow(record, in); err != nil {
		t.Fatal("in-window report", err)
	}
	for name, c := range map[string]struct {
		edit func(*tripapp.Record)
		want error
	}{
		"other-asset":  {func(r *tripapp.Record) { r.AssetID = uid(11) }, app.ErrMalformed},
		"other-author": {func(r *tripapp.Record) { r.AuthorMembershipID = uid(5) }, app.ErrForbiddenTrip},
		"before-start": {func(r *tripapp.Record) { r.Declared.OccurredAt = in.Declared.OccurredAt.Add(time.Nanosecond) }, app.ErrMalformed},
		"after-finish": {func(r *tripapp.Record) {
			r.Finish = &tripapp.FinishRecord{Declared: orgtime.Declaration{OccurredAt: in.Declared.OccurredAt.Add(-time.Nanosecond)}}
		}, app.ErrMalformed},
		"wrong-record": {func(r *tripapp.Record) { r.ID = uid(42) }, app.ErrUnavailable},
	} {
		r := record
		c.edit(&r)
		if err := app.TripWindow(r, in); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	at := record
	at.Finish = &tripapp.FinishRecord{Declared: orgtime.Declaration{OccurredAt: in.Declared.OccurredAt}}
	if err := app.TripWindow(at, in); err != nil {
		t.Fatal("a report at the finish instant", err)
	}
}

// EDR-241: the fixed policies.
func TestIncidentPoliciesAreFixed(t *testing.T) {
	want := map[app.IncidentAction]access.Policy{
		app.IncidentQueue:  {Permission: "check.incident.manage", Capability: "organizations", Effect: access.Read},
		app.IncidentRead:   {Permission: "check.incident.manage", Capability: "organizations", Effect: access.Read},
		app.IncidentDecide: {Permission: "check.incident.manage", Capability: "organizations", Effect: access.ConfigurationWrite, Fresh: true},
		app.IncidentOwn:    {Permission: "check.incident.report", Capability: "organizations", Effect: access.Read},
	}
	for action, policy := range want {
		if app.IncidentPolicy(action) != policy {
			t.Fatal("policy drift", action)
		}
	}
	if app.IncidentPolicy(0) != (access.Policy{}) || app.ReportPolicy(0) != (access.Policy{}) {
		t.Fatal("unknown action has a policy")
	}
	if app.ReportPolicy(app.ReportWrite) != (access.Policy{Permission: "check.incident.report", Capability: "organizations", Effect: access.OperationalWrite}) ||
		app.ReportPolicy(app.ReportReceipt) != (access.Policy{Permission: "check.incident.report", Capability: "organizations", Effect: access.Read}) {
		t.Fatal("intake policy drift")
	}
}

// B-BR-EMP-373: command shapes validate before any repository call; the
// digest binds every field, including the note.
func TestDecisionCommandValidatesAndDigestsEveryField(t *testing.T) {
	note := "Lanterna trocada"
	base := app.DecisionInput{OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Resolve, Note: &note}
	command, err := app.NewDecisionCommand(base)
	if err != nil {
		t.Fatal(err)
	}
	org, err := kernel.ParseOrgID(uid(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digest := command.Digest(scope)
	if len(digest) != 64 {
		t.Fatal("digest", digest)
	}
	other := "Outra nota"
	for name, edit := range map[string]func(*app.DecisionInput){
		"note":      func(v *app.DecisionInput) { v.Note = &other },
		"revision":  func(v *app.DecisionInput) { v.ExpectedRevision = 2 },
		"action":    func(v *app.DecisionInput) { v.Action = incident.Dismiss },
		"duplicate": func(v *app.DecisionInput) { v.Action, v.DuplicateOf = incident.Dismiss, uid(41) },
		"report":    func(v *app.DecisionInput) { v.ReportID = uid(42) },
	} {
		v := base
		edit(&v)
		c, err := app.NewDecisionCommand(v)
		if err != nil || c.Digest(scope) == digest {
			t.Fatal("digest does not bind", name, err)
		}
	}
	for name, v := range map[string]app.DecisionInput{
		"bad operation":      {OperationID: "x", ReportID: uid(40), ExpectedRevision: 1, Action: incident.Acknowledge},
		"bad report":         {OperationID: uid(70), ReportID: "x", ExpectedRevision: 1, Action: incident.Acknowledge},
		"zero revision":      {OperationID: uid(70), ReportID: uid(40), Action: incident.Acknowledge},
		"unknown action":     {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: "OPEN"},
		"note on ack":        {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Acknowledge, Note: &note},
		"no note":            {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Treat},
		"duplicate resolve":  {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Resolve, Note: &note, DuplicateOf: uid(41)},
		"duplicate itself":   {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Dismiss, Note: &note, DuplicateOf: uid(40)},
		"duplicate not uuid": {OperationID: uid(70), ReportID: uid(40), ExpectedRevision: 1, Action: incident.Dismiss, Note: &note, DuplicateOf: "x"},
	} {
		if _, err := app.NewDecisionCommand(v); !errors.Is(err, incident.ErrInvalid) {
			t.Fatal("invalid command accepted", name, err)
		}
	}
	if (app.DecisionCommand{}).Digest(scope) != "" {
		t.Fatal("zero command digest")
	}
}

// BR-PRIV-008: report text and handler notes never reach a log or JSON.
func TestIncidentValuesAreRedacted(t *testing.T) {
	const secret = "PRIVATE-INCIDENT-TEXT-4410"
	location := secret
	values := []any{app.ReportInput{AssetID: secret, Report: incident.Report{Description: secret, Location: &location}}, app.ReportView{Description: secret},
		app.OwnReport{Description: secret}, app.DecisionInput{Note: &location}, app.TransitionView{Note: &location}, app.IncidentActor{AccountID: secret}}
	var buffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		return a
	}}))
	for _, v := range values {
		logger.Info("value", "v", v)
		encoded, _ := json.Marshal(v)
		for _, out := range []string{fmt.Sprint(v), fmt.Sprintf("%+v %#v", v, v), string(encoded), buffer.String()} {
			if strings.Contains(out, secret) {
				t.Fatalf("private incident value leaked through %T", v)
			}
		}
	}
}
