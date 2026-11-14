//go:build integration

package adapters_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// Frozen v1 historical receipt format. This fixture deliberately does not call
// the current command's digest method to manufacture compatibility evidence.
func historicalSettingsDigest(t *testing.T, org string, v app.SettingsWriteInput) string {
	t.Helper()
	p, r := v.Values.Policies, v.Values.Responsibilities
	raw, e := json.Marshal([]any{1, "org.settings.command", org, v.OperationID, v.ExpectedRevision, v.Values.TimeZone, v.Values.ConsumptionDisplay, v.Values.ReviewRequired.Trips, v.Values.ReviewRequired.Fuelings, v.Values.ReviewRequired.Expenses, v.Values.ApprovalLevelsDefault, v.Values.ClosurePeriodDays, p.IntakeWindowDays, p.ClockSkewLimitSeconds, p.DelegationMaximumDays, p.InvitationValidityDays, p.ActivationValidityHours, r.ReviewFallback.PrimaryID, r.ReviewFallback.SubstituteID, r.PlatformNotices.PrimaryID, r.PlatformNotices.SubstituteID, r.DataProtection.PrimaryID, r.DataProtection.SubstituteID, r.Billing.PrimaryID, r.Billing.SubstituteID, v.Reason})
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func seedHistoricalSettingsReceipt(t *testing.T, fx legalFixtureState, store *orgpg.SettingsStore, command app.SettingsWrite) app.SettingsResult {
	t.Helper()
	v, e := command.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	hash := historicalSettingsDigest(t, fx.scope.OrgID().String(), v)
	raw, e := orgpg.EncodeSettingsValues(v.Values)
	if e != nil {
		t.Fatal(e)
	}
	var result app.SettingsResult
	e = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		_, e := store.Audit.RecordChangeTx(t.Context(), tx, fx.scope, audit.Event{OperationID: v.OperationID, ActorID: fx.actor.AccountID, Action: "org.settings.changed", TargetType: "org.settings", TargetID: fx.scope.OrgID().String(), ReasonCode: "configuration", RequestHash: hash}, func(ctx context.Context, tx pgx.Tx) error {
			queries := q.New(tx)
			if e := queries.InsertSettingsRevision(ctx, q.InsertSettingsRevisionParams{OrgID: fx.scope.OrgID().String(), Revision: v.ExpectedRevision + 1, SettingsValues: raw, ActorID: fx.actor.AccountID, MemberID: fx.member, Reason: v.Reason, OperationID: v.OperationID}); e != nil {
				return e
			}
			if n, e := queries.PublishSettingsRevision(ctx, q.PublishSettingsRevisionParams{OrgID: fx.scope.OrgID().String(), ExpectedRevision: v.ExpectedRevision}); e != nil {
				return e
			} else if n != 1 {
				t.Fatal("legacy fixture source revision conflict")
			}
			row, e := queries.GetSettingsRevision(ctx, q.GetSettingsRevisionParams{OrgID: fx.scope.OrgID().String(), Revision: v.ExpectedRevision + 1})
			if e != nil {
				return e
			}
			root, e := queries.SettingsRootRevision(ctx, fx.scope.OrgID().String())
			if e != nil {
				return e
			}
			result = app.SettingsResult{Revision: row.Revision, RootRevision: root, EffectiveAt: row.EffectiveAt.Time.UTC()}
			return queries.InsertSettingsReceipt(ctx, q.InsertSettingsReceiptParams{OrgID: fx.scope.OrgID().String(), OperationID: v.OperationID, ActorID: fx.actor.AccountID, RequestHash: hash, SettingsRevision: row.Revision, ResultRootRevision: root})
		})
		return e
	})
	if e != nil {
		t.Fatal("could not provision a valid immutable v1 source receipt", e)
	}
	return result
}

func TestSettingsLegacyReceiptsPreserveCompleteIntent(t *testing.T) {
	for _, confirmation := range []bool{false, true} {
		name := "false"
		if confirmation {
			name = "true"
		}
		t.Run(name, func(t *testing.T) {
			fx := settingsSchemaFixture(t)
			store := settingsStore(t, fx)
			before := settingsRead(t, fx, store, 0)
			values := before.Values
			values.Policies.RequireInvitationConfirmation = confirmation
			original := settingsCommand(t, 1, values, "")
			ack := seedHistoricalSettingsReceipt(t, fx, store, original)
			got, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, original)
			if e != nil || !replayed || got != ack {
				t.Fatal("same-intent genuine v1 acknowledgement was not preserved", e)
			}
			input, e := original.Snapshot()
			if e != nil {
				t.Fatal(e)
			}
			input.Values.Policies.RequireInvitationConfirmation = !confirmation
			changed, e := app.NewSettingsWrite(input)
			if e != nil {
				t.Fatal(e)
			}
			if _, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, changed); !errors.Is(e, app.ErrOperationReused) || replayed {
				t.Fatal("legacy receipt accepted a different confirmation policy", e)
			}
			input, _ = original.Snapshot()
			input.Reason = "Synthetic changed historical reason"
			changed, e = app.NewSettingsWrite(input)
			if e != nil {
				t.Fatal(e)
			}
			if _, _, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, changed); !errors.Is(e, app.ErrOperationReused) {
				t.Fatal("legacy receipt accepted another immutable command change", e)
			}
			var count int
			if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_settings_commands WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&count); e != nil || count != 1 {
				t.Fatal("legacy compatibility rewrote or duplicated receipts")
			}
			if confirmation {
				role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
				if _, e := fx.f.Admin.Exec(t.Context(), "REVOKE SELECT ON org_settings_versions FROM "+role); e != nil {
					t.Fatal(e)
				}
				got, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, original)
				if !errors.Is(e, app.ErrUnavailable) || replayed || got != (app.SettingsResult{}) {
					t.Fatal("missing immutable source exposed a legacy acknowledgement", e)
				}
				if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_settings_versions TO "+role); e != nil {
					t.Fatal(e)
				}
			} else {
				current := setupRehireOwnerForReceipt(t, fx)
				got, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, current, original)
				if e != nil || !replayed || got != ack {
					t.Fatal("new current membership period invalidated a legitimate historical same-account replay", e)
				}
			}
		})
	}
}

func setupRehireOwnerForReceipt(t *testing.T, fx legalFixtureState) string {
	t.Helper()
	other := transferPersonalActor(t, transferFixtureState{legalFixtureState: fx, actors: map[string]app.CompanyActor{}})
	_, backup := teamDriver(t, fx, other.AccountID)
	org := fx.scope.OrgID().String()
	if e := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic receipt backup owner')`, org, onboardingID(t), backup, fx.member); e != nil {
		t.Fatal(e)
	}
	var employee string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT employee_id::text FROM org_memberships WHERE org_id=$1 AND id=$2`, org, fx.member).Scan(&employee); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, fx.member); e != nil {
		t.Fatal(e)
	}
	current := onboardingID(t)
	if e := unitSchemaExec(t, fx, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, org, current, employee, fx.actor.AccountID); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'OWNER','ORG',$1,clock_timestamp(),$4,'Synthetic current receipt owner')`, org, onboardingID(t), current, backup); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, current); e != nil {
		t.Fatal(e)
	}
	return current
}

func TestSettingsNewReceiptsPinEveryConfirmationChange(t *testing.T) {
	fx := settingsSchemaFixture(t)
	store := settingsStore(t, fx)
	values := settingsRead(t, fx, store, 0).Values
	values.Policies.RequireInvitationConfirmation = true
	command := settingsCommand(t, 1, values, "")
	ack, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, command)
	if e != nil || replayed {
		t.Fatal("new complete settings intent failed", e)
	}
	got, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, command)
	if e != nil || !replayed || got != ack {
		t.Fatal("new same-intent receipt did not preserve original acknowledgement", e)
	}
	input, e := command.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	input.Values.Policies.RequireInvitationConfirmation = false
	changed, e := app.NewSettingsWrite(input)
	if e != nil {
		t.Fatal(e)
	}
	if _, replayed, e := store.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, changed); !errors.Is(e, app.ErrOperationReused) || replayed {
		t.Fatal("v2 receipt accepted changed security policy", e)
	}
	input, _ = command.Snapshot()
	var stored string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT request_hash FROM org_settings_commands WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), input.OperationID).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if stored != command.Digest(fx.scope) || stored == historicalSettingsDigest(t, fx.scope.OrgID().String(), input) {
		t.Fatal("new command persisted the incomplete historical digest")
	}
}
