// Package adapters implements F7 PostgreSQL persistence for station
// agreements, negotiated price terms, bounded fueling authorizations and
// the station's once-only redemption confirmation.
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Authority proves the company session, capability, membership and role for
// one F7 policy in the caller's transaction.
type Authority interface {
	CheckAgreementTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, member string, target string, policy access.Policy) error
}

// Store is the F7 repository.
type Store struct {
	Pool      *pgxpool.Pool
	Authority Authority
	Audit     auditing.TransactionRunner
	NewID     func() (string, error)
	Now       func() time.Time
}

var _ app.Repository = (*Store)(nil)

// AuditDefinitions lists the audited F7 transitions (BR-AUD-001).
func AuditDefinitions() []audit.Definition {
	actions := []struct {
		action, target string
	}{
		{"agree.agreement.proposed", "agreement"},
		{"agree.agreement.decided", "agreement"},
		{"agree.agreement.updated", "agreement"},
		{"agree.authorization.issued", "authorization"},
		{"agree.authorization.updated", "authorization"},
		{"agree.redemption.confirmed", "redemption"},
	}
	out := make([]audit.Definition, len(actions))
	for i, a := range actions {
		out[i] = audit.Definition{Action: a.action, TargetType: a.target}
	}
	return out
}

func (s *Store) ready(write bool) bool {
	return s != nil && s.Pool != nil && s.NewID != nil && (!write || s.Audit != nil)
}

// readyOrg adds the organization-side authority requirement.
func (s *Store) readyOrg(write bool) bool {
	return s.ready(write) && s.Authority != nil
}

func validUUID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func mapPGErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return app.ErrOperationReused
		case "23514", "P0001":
			return app.ErrConflict
		}
	}
	return app.ErrUnavailable
}

func digestCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// commandCheck answers the stored idempotent receipt in org_f7_commands.
func commandCheck(ctx context.Context, tx pgx.Tx, org, scopeKind, opID, reqHash string) (json.RawMessage, bool, error) {
	if !validUUID(opID) {
		return nil, false, app.ErrInvalid
	}
	var existingHash string
	var result json.RawMessage
	err := tx.QueryRow(ctx, `SELECT request_hash, result_json FROM org_f7_commands WHERE org_id=$1::uuid AND scope_kind=$2 AND operation_id=$3::uuid`, org, scopeKind, opID).Scan(&existingHash, &result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, app.ErrUnavailable
	}
	if existingHash != reqHash {
		return nil, false, app.ErrOperationReused
	}
	return result, true, nil
}

func commandSave(ctx context.Context, tx pgx.Tx, org, scopeKind, opID, entityID, reqHash string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return app.ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `INSERT INTO org_f7_commands(org_id,scope_kind,operation_id,entity_id,request_hash,result_json) VALUES($1::uuid,$2,$3::uuid,$4::uuid,$5,$6)`,
		org, scopeKind, opID, entityID, reqHash, raw); err != nil {
		return mapPGErr(err)
	}
	return nil
}

func (s *Store) appendAudit(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, action, targetType, targetID, reason string) error {
	opID, err := s.NewID()
	if err != nil {
		return app.ErrUnavailable
	}
	sum := sha256.Sum256([]byte(action + ":" + targetType + ":" + targetID + ":" + reason))
	ev := audit.Event{
		OperationID: opID,
		ActorID:     actor.AccountID,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		RequestHash: hex.EncodeToString(sum[:]),
	}
	_, err = s.Audit.RecordChangeTx(ctx, tx, scope, ev, func(context.Context, pgx.Tx) error { return nil })
	return err
}

func stationScope(ctx context.Context, stationID string) (string, error) {
	if !validUUID(stationID) {
		return "", app.ErrInvalid
	}
	return stationID, nil
}

// committedRefusal commits the recorded side effects (the bounded attempt
// and its event) while answering the caller's typed refusal: the attempt
// must survive even though the command itself is refused.
type committedRefusal struct{ err error }

func (c committedRefusal) Error() string { return c.err.Error() }
func (c committedRefusal) Unwrap() error { return c.err }

// withStationTx opens the station-side scope: only anpfuel.station_id is
// set, so the permissive station policies expose exactly the station's own
// rows. The organization scope joins after the target resolves. A
// committed refusal keeps the recorded attempt rows and still answers the
// refusal.
func withStationTx(ctx context.Context, pool *pgxpool.Pool, stationID string, work func(pgx.Tx) error) error {
	if pool == nil || stationID == "" {
		return app.ErrUnavailable
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return app.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('anpfuel.org_id','',true),pg_catalog.set_config('anpfuel.station_id',$1,true)`, stationID); err != nil {
		return app.ErrUnavailable
	}
	if err := work(tx); err != nil {
		var refusal committedRefusal
		if errors.As(err, &refusal) {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return app.ErrUnavailable
			}
			return refusal.Unwrap()
		}
		return err
	}
	return tx.Commit(ctx)
}

// joinOrgScope narrows an open station transaction to the resolved
// organization as well, so private organization rows stay inside their
// scope for the remaining writes.
func joinOrgScope(ctx context.Context, tx pgx.Tx, org string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_catalog.set_config('anpfuel.org_id',$1,true)`, org); err != nil {
		return app.ErrUnavailable
	}
	return nil
}

func orgScopeOf(org string) (kernel.OrgScope, error) {
	id, err := kernel.ParseOrgID(org)
	if err != nil {
		return kernel.OrgScope{}, app.ErrInvalid
	}
	return kernel.NewOrgScope(id)
}

func proveStation(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority, scope string) (app.StationGrant, error) {
	if authority == nil || stationID == "" || actor.AccountID == "" {
		return app.StationGrant{}, app.ErrUnauthenticated
	}
	grant, err := authority.StationGrantTx(ctx, stationID, actor.AccountID, scope)
	if err != nil {
		return app.StationGrant{}, err
	}
	for _, granted := range grant.Scopes {
		if granted == scope {
			return grant, nil
		}
	}
	return app.StationGrant{}, app.ErrUnauthenticated
}

func stamp(v time.Time) string { return v.UTC().Format(time.RFC3339Nano) }

// ProposeAgreement records the proposal and its negotiated terms once.
func (s *Store) ProposeAgreement(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.ProposeInput, agreementID, digest string) (app.AgreementView, bool, error) {
	if !s.readyOrg(true) {
		return app.AgreementView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	if strings.TrimSpace(in.OrgLabel) == "" || len([]rune(strings.TrimSpace(in.OrgLabel))) > 80 || len([]rune(strings.TrimSpace(in.Conditions))) > 280 {
		return app.AgreementView{}, false, app.ErrInvalid
	}
	terms := make([]d.Term, 0, len(in.Terms))
	for _, term := range in.Terms {
		terms = append(terms, d.Term{Product: term.Product, Unit: term.Unit, PriceMilliBRL: term.PriceMilliBRL, Conditions: term.Conditions})
	}
	if _, err := d.Propose(in.ValidFrom, in.ValidUntil, terms); err != nil {
		return app.AgreementView{}, false, app.ErrInvalid
	}
	var out app.AgreementView
	var replayed bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, org, app.AgreementPolicy(true)); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "agree.propose", in.OperationID, digest); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var from, until time.Time
		if err := tx.QueryRow(ctx, `
			INSERT INTO org_station_agreements(org_id,id,station_id,org_label,state,valid_from,valid_until,conditions,proposed_by_membership_id)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4,'PROPOSED',$5,$6,$7,$8::uuid)
			RETURNING id::text,station_id::text,org_label,state,revision,valid_from,valid_until,COALESCE(conditions,'')`,
			org, agreementID, in.StationID, strings.TrimSpace(in.OrgLabel), in.ValidFrom, in.ValidUntil, strings.TrimSpace(in.Conditions), member,
		).Scan(&out.ID, &out.StationID, &out.OrgLabel, &out.State, &out.Revision, &from, &until, &out.Conditions); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrInvalid
			}
			return mapPGErr(err)
		}
		out.ValidFrom, out.ValidUntil = stamp(from), stamp(until)
		for _, term := range in.Terms {
			if _, err := tx.Exec(ctx, `INSERT INTO org_agreement_price_terms(org_id,agreement_id,fuel_product,unit,price_milli_brl,conditions) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6)`,
				org, agreementID, term.Product, term.Unit, term.PriceMilliBRL, term.Conditions); err != nil {
				return mapPGErr(err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_station_agreement_transitions(org_id,agreement_id,revision,kind,from_state,to_state,actor_kind,actor_membership_id,operation_id,reason)
			VALUES($1::uuid,$2::uuid,$3,'PROPOSE','PROPOSED','PROPOSED','ORG',$4::uuid,$5::uuid,$6)`,
			org, agreementID, out.Revision, member, in.OperationID, ""); err != nil {
			return mapPGErr(err)
		}
		out.DerivedState = string(d.DerivedState(d.Proposed, s.now(), in.ValidFrom, in.ValidUntil))
		out.Usable = false
		out.Terms = termViews(in.Terms)
		if err := commandSave(ctx, tx, org, "agree.propose", in.OperationID, agreementID, digest, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, "agree.agreement.proposed", "agreement", agreementID, "")
	})
	return out, replayed, err
}

func termViews(terms []app.TermInput) []app.TermView {
	out := make([]app.TermView, 0, len(terms))
	for _, term := range terms {
		out = append(out, app.TermView{Product: term.Product, Unit: term.Unit, PriceMilliBRL: term.PriceMilliBRL, Conditions: term.Conditions})
	}
	return out
}

// ApplyAgreementTransition applies one recorded SUSPEND or END command.
func (s *Store) ApplyAgreementTransition(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.AgreementTransitionInput, kind string) (app.AgreementView, bool, error) {
	if !s.readyOrg(true) || (kind != "SUSPEND" && kind != "END") {
		return app.AgreementView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	digest, err := app.HashRequest(in)
	if err != nil {
		return app.AgreementView{}, false, err
	}
	var out app.AgreementView
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, in.AgreementID, app.AgreementPolicy(true)); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "agree."+strings.ToLower(kind), in.OperationID, digest); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var state, conditions string
		var revision int64
		var from, until time.Time
		if err := tx.QueryRow(ctx, `SELECT state,revision,valid_from,valid_until,COALESCE(conditions,'') FROM org_station_agreements WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`,
			org, in.AgreementID).Scan(&state, &revision, &from, &until, &conditions); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return app.ErrUnavailable
		}
		head := d.AgreementOf(d.State(state), d.WithWindow(from, until), d.WithRevision(revision))
		var next d.Agreement
		if kind == "SUSPEND" {
			next, err = head.Suspend(s.now())
		} else {
			next, err = head.End(s.now())
		}
		if err != nil {
			return app.ErrConflict
		}
		if in.Revision != revision {
			return app.ErrConflict
		}
		var reasonValue any
		if strings.TrimSpace(in.Reason) != "" {
			reasonValue = strings.TrimSpace(in.Reason)
		}
		var endedReason string
		if kind == "END" {
			endedReason = strings.TrimSpace(in.Reason)
			if len([]rune(endedReason)) > 280 {
				return app.ErrInvalid
			}
			if endedReason == "" {
				endedReason = "Encerramento pela organização"
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE org_station_agreements SET state=$4,revision=revision+1,ended_reason=COALESCE($5,ended_reason) WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3 AND state=$6`,
			org, in.AgreementID, revision, string(next.State()), reasonValue, state)
		if err != nil {
			return mapPGErr(err)
		}
		if tag.RowsAffected() != 1 {
			return app.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_station_agreement_transitions(org_id,agreement_id,revision,kind,from_state,to_state,actor_kind,actor_membership_id,operation_id,reason)
			VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,'ORG',$7::uuid,$8::uuid,$9)`,
			org, in.AgreementID, revision+1, kind, state, string(next.State()), member, in.OperationID, strings.TrimSpace(in.Reason)); err != nil {
			return mapPGErr(err)
		}
		out, err = s.readAgreementTx(ctx, tx, org, in.AgreementID)
		if err != nil {
			return err
		}
		if err := commandSave(ctx, tx, org, "agree."+strings.ToLower(kind), in.OperationID, in.AgreementID, digest, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, "agree.agreement.updated", "agreement", in.AgreementID, strings.TrimSpace(in.Reason))
	})
	return out, replayed, err
}

func (s *Store) readAgreementTx(ctx context.Context, tx pgx.Tx, org, agreementID string) (app.AgreementView, error) {
	var out app.AgreementView
	var from, until time.Time
	if err := tx.QueryRow(ctx, `SELECT id::text,station_id::text,org_label,state,revision,valid_from,valid_until,COALESCE(conditions,'') FROM org_station_agreements WHERE org_id=$1::uuid AND id=$2::uuid`,
		org, agreementID).Scan(&out.ID, &out.StationID, &out.OrgLabel, &out.State, &out.Revision, &from, &until, &out.Conditions); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.AgreementView{}, app.ErrNotFound
		}
		return app.AgreementView{}, app.ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT fuel_product,unit,price_milli_brl,COALESCE(conditions,'') FROM org_agreement_price_terms WHERE org_id=$1::uuid AND agreement_id=$2::uuid ORDER BY fuel_product,unit`,
		org, agreementID)
	if err != nil {
		return app.AgreementView{}, app.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var term app.TermView
		if err := rows.Scan(&term.Product, &term.Unit, &term.PriceMilliBRL, &term.Conditions); err != nil {
			return app.AgreementView{}, app.ErrUnavailable
		}
		out.Terms = append(out.Terms, term)
	}
	if rows.Err() != nil {
		return app.AgreementView{}, app.ErrUnavailable
	}
	now := s.now()
	out.DerivedState = string(d.DerivedState(d.State(out.State), now, from, until))
	out.Usable = d.AgreementOf(d.State(out.State), d.WithWindow(from, until)).Usable(now)
	out.ValidFrom, out.ValidUntil = stamp(from), stamp(until)
	return out, nil
}

// ReadAgreement reads one agreement with its negotiated terms.
func (s *Store) ReadAgreement(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, agreementID string) (app.AgreementView, error) {
	if !s.readyOrg(false) {
		return app.AgreementView{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var out app.AgreementView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, agreementID, app.AgreementPolicy(false)); err != nil {
			return err
		}
		var err error
		out, err = s.readAgreementTx(ctx, tx, org, agreementID)
		return err
	})
	return out, err
}

// ListAgreements lists the organization's agreements with derived states.
func (s *Store) ListAgreements(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string) ([]app.AgreementView, error) {
	if !s.readyOrg(false) {
		return nil, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	out := []app.AgreementView{}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, org, app.AgreementPolicy(false)); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM org_station_agreements WHERE org_id=$1::uuid ORDER BY proposed_at DESC LIMIT 100`, org)
		if err != nil {
			return app.ErrUnavailable
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return app.ErrUnavailable
			}
			ids = append(ids, id)
		}
		rows.Close()
		if rows.Err() != nil {
			return app.ErrUnavailable
		}
		for _, id := range ids {
			view, err := s.readAgreementTx(ctx, tx, org, id)
			if err != nil {
				return err
			}
			out = append(out, view)
		}
		return nil
	})
	return out, err
}

// IssueAuthorization issues one bounded single-use voucher under a usable
// agreement with a negotiated term for its product and unit. The code is
// answered exactly once; only its digest is stored.
func (s *Store) IssueAuthorization(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.IssueInput, authorizationID, code, digest string) (app.IssueAcknowledgement, bool, error) {
	if !s.readyOrg(true) {
		return app.IssueAcknowledgement{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var ack app.IssueAcknowledgement
	var replayed bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, in.AgreementID, app.AuthorizationPolicy(true)); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "agree.issue", in.OperationID, digest); err != nil {
			return err
		} else if hit {
			// The one-time code is never repeated: a replay answers the
			// stored authorization view only.
			replayed = true
			var stored app.AuthorizationView
			if err := json.Unmarshal(raw, &stored); err != nil {
				return app.ErrUnavailable
			}
			ack.Authorization = stored
			return nil
		}
		var stationID string
		var state string
		var revision int64
		var from, until time.Time
		if err := tx.QueryRow(ctx, `SELECT station_id::text,state,revision,valid_from,valid_until FROM org_station_agreements WHERE org_id=$1::uuid AND id=$2::uuid FOR UPDATE`,
			org, in.AgreementID).Scan(&stationID, &state, &revision, &from, &until); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return app.ErrUnavailable
		}
		head := d.AgreementOf(d.State(state), d.WithWindow(from, until))
		// Issuance requires the accepted head; the validity window decides
		// at redemption time, so a future-window agreement can pre-authorize.
		if head.State() != d.Active {
			return app.ErrConflict
		}
		var price int64
		if err := tx.QueryRow(ctx, `SELECT price_milli_brl FROM org_agreement_price_terms WHERE org_id=$1::uuid AND agreement_id=$2::uuid AND fuel_product=$3 AND unit=$4`,
			org, in.AgreementID, in.Product, in.Unit).Scan(&price); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrInvalid
			}
			return app.ErrUnavailable
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_authorizations(org_id,id,agreement_id,station_id,employee_id,asset_id,fuel_product,unit,max_quantity_milli,max_centavos,valid_until,code_hash,issued_by_membership_id)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6,$7,$8,$9,$10,$11,$12,$13::uuid)`,
			org, authorizationID, in.AgreementID, stationID, in.EmployeeID, nilString(in.AssetID), in.Product, in.Unit, in.MaxQuantityMilli, in.MaxCentavos, in.ValidUntil, digestCode(code), member); err != nil {
			return mapPGErr(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_authorization_events(org_id,authorization_id,seq,kind,actor_kind,actor_membership_id) VALUES($1::uuid,$2::uuid,1,'ISSUE','ORG',$3::uuid)`,
			org, authorizationID, member); err != nil {
			return mapPGErr(err)
		}
		view, err := s.readAuthorizationTx(ctx, tx, org, authorizationID)
		if err != nil {
			return err
		}
		if err := commandSave(ctx, tx, org, "agree.issue", in.OperationID, authorizationID, digest, view); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, scope, actor, "agree.authorization.issued", "authorization", authorizationID, ""); err != nil {
			return err
		}
		ack.Authorization, ack.Code = view, code
		return nil
	})
	return ack, replayed, err
}

func nilString(raw string) any {
	if raw == "" {
		return nil
	}
	return raw
}

func (s *Store) readAuthorizationTx(ctx context.Context, tx pgx.Tx, org, authorizationID string) (app.AuthorizationView, error) {
	var out app.AuthorizationView
	var asset *string
	var validUntil time.Time
	if err := tx.QueryRow(ctx, `SELECT id::text,agreement_id::text,station_id::text,employee_id::text,asset_id::text,fuel_product,unit,max_quantity_milli,max_centavos,valid_until,state,failed_attempts
		FROM org_fuel_authorizations WHERE org_id=$1::uuid AND id=$2::uuid`,
		org, authorizationID).Scan(&out.ID, &out.AgreementID, &out.StationID, &out.EmployeeID, &asset, &out.Product, &out.Unit, &out.MaxQuantityMilli, &out.MaxCentavos, &validUntil, &out.State, &out.FailedAttempts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.AuthorizationView{}, app.ErrNotFound
		}
		return app.AuthorizationView{}, app.ErrUnavailable
	}
	if asset != nil {
		out.AssetID = *asset
	}
	voucher := d.VoucherOf(d.VoucherState(out.State), d.VoucherValidUntil(validUntil))
	out.DerivedState = string(voucher.DerivedState(s.now()))
	out.ValidUntil = stamp(validUntil)
	return out, nil
}

// ReadAuthorization reads one voucher; the code never repeats.
func (s *Store) ReadAuthorization(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member, authorizationID string) (app.AuthorizationView, error) {
	if !s.readyOrg(false) {
		return app.AuthorizationView{}, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	var out app.AuthorizationView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, authorizationID, app.AuthorizationPolicy(false)); err != nil {
			return err
		}
		var err error
		out, err = s.readAuthorizationTx(ctx, tx, org, authorizationID)
		return err
	})
	return out, err
}

// ListAuthorizations lists the organization's vouchers with derived states.
func (s *Store) ListAuthorizations(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string) ([]app.AuthorizationView, error) {
	if !s.readyOrg(false) {
		return nil, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	out := []app.AuthorizationView{}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, org, app.AuthorizationPolicy(false)); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM org_fuel_authorizations WHERE org_id=$1::uuid ORDER BY issued_at DESC LIMIT 100`, org)
		if err != nil {
			return app.ErrUnavailable
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return app.ErrUnavailable
			}
			ids = append(ids, id)
		}
		rows.Close()
		if rows.Err() != nil {
			return app.ErrUnavailable
		}
		for _, id := range ids {
			view, err := s.readAuthorizationTx(ctx, tx, org, id)
			if err != nil {
				return err
			}
			out = append(out, view)
		}
		return nil
	})
	return out, err
}

// CancelAuthorization cancels one issued voucher.
func (s *Store) CancelAuthorization(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string, in app.CancelInput) (app.AuthorizationView, bool, error) {
	if !s.readyOrg(true) {
		return app.AuthorizationView{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	digest, err := app.HashRequest(in)
	if err != nil {
		return app.AuthorizationView{}, false, err
	}
	var out app.AuthorizationView
	var replayed bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, in.AuthorizationID, app.AuthorizationPolicy(true)); err != nil {
			return err
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "agree.cancel", in.OperationID, digest); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		tag, err := tx.Exec(ctx, `UPDATE org_fuel_authorizations SET state='CANCELLED',cancelled_reason=$3 WHERE org_id=$1::uuid AND id=$2::uuid AND state='ISSUED'`,
			org, in.AuthorizationID, strings.TrimSpace(in.Reason))
		if err != nil {
			return mapPGErr(err)
		}
		if tag.RowsAffected() != 1 {
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_fuel_authorizations WHERE org_id=$1::uuid AND id=$2::uuid AND state='REDEEMED')`, org, in.AuthorizationID).Scan(new(bool)); err == nil {
				return app.ErrConflict
			}
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_fuel_authorizations WHERE org_id=$1::uuid AND id=$2::uuid)`, org, in.AuthorizationID).Scan(new(bool)); err == nil {
				return app.ErrNotFound
			}
			return app.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_authorization_events(org_id,authorization_id,seq,kind,actor_kind,actor_membership_id,detail)
			SELECT $1::uuid,$2::uuid,(SELECT COALESCE(MAX(seq),0)+1 FROM org_fuel_authorization_events WHERE org_id=$1::uuid AND authorization_id=$2::uuid),'CANCEL','ORG',$3::uuid,$4`,
			org, in.AuthorizationID, member, strings.TrimSpace(in.Reason)); err != nil {
			return mapPGErr(err)
		}
		var err2 error
		out, err2 = s.readAuthorizationTx(ctx, tx, org, in.AuthorizationID)
		if err2 != nil {
			return err2
		}
		if err := commandSave(ctx, tx, org, "agree.cancel", in.OperationID, in.AuthorizationID, digest, out); err != nil {
			return err
		}
		return s.appendAudit(ctx, tx, scope, actor, "agree.authorization.updated", "authorization", in.AuthorizationID, strings.TrimSpace(in.Reason))
	})
	return out, replayed, err
}

// ListRedemptions lists the station-confirmed settlements in scope.
func (s *Store) ListRedemptions(ctx context.Context, scope kernel.OrgScope, actor app.Actor, member string) ([]app.RedemptionView, error) {
	if !s.readyOrg(false) {
		return nil, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	out := []app.RedemptionView{}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckAgreementTx(ctx, tx, scope, actor, member, org, app.AgreementPolicy(false)); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id::text,agreement_id::text,authorization_id::text,station_id::text,employee_id::text,asset_id::text,fuel_product,unit,quantity_milli,price_milli_brl,total_centavos,confirmed_at
			FROM org_fuel_redemptions WHERE org_id=$1::uuid ORDER BY confirmed_at DESC LIMIT 100`, org)
		if err != nil {
			return app.ErrUnavailable
		}
		defer rows.Close()
		for rows.Next() {
			var view app.RedemptionView
			var asset *string
			var confirmedAt time.Time
			if err := rows.Scan(&view.ID, &view.AgreementID, &view.AuthorizationID, &view.StationID, &view.EmployeeID, &asset, &view.Product, &view.Unit, &view.QuantityMilli, &view.PriceMilliBRL, &view.TotalCentavos, &confirmedAt); err != nil {
				return app.ErrUnavailable
			}
			if asset != nil {
				view.AssetID = *asset
			}
			view.ConfirmedAt = stamp(confirmedAt)
			out = append(out, view)
		}
		return rows.Err()
	})
	return out, err
}

// stationAgreementTx reads the station-side allowlisted projection of one
// agreement under the station scope.
func stationAgreementTx(ctx context.Context, tx pgx.Tx, agreementID string) (app.StationAgreementView, bool, error) {
	var out app.StationAgreementView
	var from, until time.Time
	err := tx.QueryRow(ctx, `SELECT id::text,org_label,state,valid_from,valid_until,COALESCE(conditions,'') FROM org_station_agreements WHERE id=$1::uuid`,
		agreementID).Scan(&out.ID, &out.OrgLabel, &out.State, &from, &until, &out.Conditions)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.StationAgreementView{}, false, nil
	}
	if err != nil {
		return app.StationAgreementView{}, false, app.ErrUnavailable
	}
	rows, err := tx.Query(ctx, `SELECT fuel_product,unit,price_milli_brl,COALESCE(conditions,'') FROM org_agreement_price_terms WHERE agreement_id=$1::uuid ORDER BY fuel_product,unit`,
		agreementID)
	if err != nil {
		return app.StationAgreementView{}, false, app.ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var term app.TermView
		if err := rows.Scan(&term.Product, &term.Unit, &term.PriceMilliBRL, &term.Conditions); err != nil {
			return app.StationAgreementView{}, false, app.ErrUnavailable
		}
		out.Terms = append(out.Terms, term)
	}
	if rows.Err() != nil {
		return app.StationAgreementView{}, false, app.ErrUnavailable
	}
	now := time.Now().UTC()
	out.DerivedState = string(d.DerivedState(d.State(out.State), now, from, until))
	out.ValidFrom, out.ValidUntil = stamp(from), stamp(until)
	return out, true, nil
}

// StationAgreements lists the station's agreements across organizations
// with only the allowlisted fields (BR-AGREE-001).
func (s *Store) StationAgreements(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority) ([]app.StationAgreementView, error) {
	if !s.ready(false) {
		return nil, app.ErrUnavailable
	}
	if _, err := stationScope(ctx, stationID); err != nil {
		return nil, err
	}
	if _, err := proveStation(ctx, stationID, actor, authority, "agreement.manage"); err != nil {
		return nil, err
	}
	out := []app.StationAgreementView{}
	err := withStationTx(ctx, s.Pool, stationID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text FROM org_station_agreements WHERE station_id=$1::uuid ORDER BY proposed_at DESC LIMIT 100`, stationID)
		if err != nil {
			return app.ErrUnavailable
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return app.ErrUnavailable
			}
			ids = append(ids, id)
		}
		rows.Close()
		if rows.Err() != nil {
			return app.ErrUnavailable
		}
		for _, id := range ids {
			view, found, err := stationAgreementTx(ctx, tx, id)
			if err != nil {
				return err
			}
			if found {
				out = append(out, view)
			}
		}
		return nil
	})
	return out, err
}

// stationDecide applies the station party's explicit ACCEPT or DECLINE. A
// repeated command is refused by the recorded state machine and the
// transition row keeps the minted audited reference.
func (s *Store) stationDecide(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority, in app.StationDecisionInput, accept bool) (app.StationAgreementView, bool, error) {
	kind := "DECLINE"
	if accept {
		kind = "ACCEPT"
	}
	if !s.ready(true) || !validUUID(in.AgreementID) {
		return app.StationAgreementView{}, false, app.ErrInvalid
	}
	if _, err := proveStation(ctx, stationID, actor, authority, "agreement.manage"); err != nil {
		return app.StationAgreementView{}, false, err
	}
	if len([]rune(strings.TrimSpace(in.Reason))) > 280 {
		return app.StationAgreementView{}, false, app.ErrInvalid
	}
	operationID, err := s.NewID()
	if err != nil {
		return app.StationAgreementView{}, false, app.ErrUnavailable
	}
	var out app.StationAgreementView
	err = withStationTx(ctx, s.Pool, stationID, func(tx pgx.Tx) error {
		var org string
		var state string
		var revision int64
		var from, until time.Time
		if err := tx.QueryRow(ctx, `SELECT org_id::text,state,revision,valid_from,valid_until FROM org_station_agreements WHERE id=$1::uuid AND station_id=$2::uuid FOR UPDATE`,
			in.AgreementID, stationID).Scan(&org, &state, &revision, &from, &until); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return app.ErrUnavailable
		}
		if err := joinOrgScope(ctx, tx, org); err != nil {
			return err
		}
		scope, err := orgScopeOf(org)
		if err != nil {
			return app.ErrUnavailable
		}
		head := d.AgreementOf(d.State(state), d.WithWindow(from, until), d.WithRevision(revision))
		var next d.Agreement
		if accept {
			next, err = head.Accept(s.now())
		} else {
			next, err = head.Decline(s.now())
		}
		if err != nil {
			return app.ErrConflict
		}
		decidedAt := s.now()
		tag, err := tx.Exec(ctx, `UPDATE org_station_agreements SET state=$4,revision=revision+1,decided_by_account_id=$5,decided_at=$6 WHERE org_id=$1::uuid AND id=$2::uuid AND revision=$3 AND state='PROPOSED'`,
			org, in.AgreementID, revision, string(next.State()), actor.AccountID, decidedAt)
		if err != nil {
			return mapPGErr(err)
		}
		if tag.RowsAffected() != 1 {
			return app.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_station_agreement_transitions(org_id,agreement_id,revision,kind,from_state,to_state,actor_kind,actor_account_id,operation_id,reason)
			VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,'STATION',$7::uuid,$8::uuid,$9)`,
			org, in.AgreementID, revision+1, kind, state, string(next.State()), actor.AccountID, operationID, strings.TrimSpace(in.Reason)); err != nil {
			return mapPGErr(err)
		}
		view, found, err := stationAgreementTx(ctx, tx, in.AgreementID)
		if err != nil || !found {
			if err == nil {
				err = app.ErrUnavailable
			}
			return err
		}
		out = view
		orgActor := app.Actor{AccountID: actor.AccountID}
		return s.appendAudit(ctx, tx, scope, orgActor, "agree.agreement.decided", "agreement", in.AgreementID, strings.TrimSpace(in.Reason))
	})
	return out, false, err
}

// StationAccept is the station party's explicit acceptance.
func (s *Store) StationAccept(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority, in app.StationDecisionInput) (app.StationAgreementView, bool, error) {
	return s.stationDecide(ctx, stationID, actor, authority, in, true)
}

// StationDecline is the station party's explicit decline.
func (s *Store) StationDecline(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority, in app.StationDecisionInput) (app.StationAgreementView, bool, error) {
	return s.stationDecide(ctx, stationID, actor, authority, in, false)
}

// StationConfirmRedemption consumes the voucher once under the agreement's
// negotiated price and records the settlement. A matching retry answers
// the stored receipt; competing use cannot consume it twice.
func (s *Store) StationConfirmRedemption(ctx context.Context, stationID string, actor app.StationActor, authority app.StationAuthority, in app.ConfirmInput, redemptionID string) (app.RedemptionView, bool, error) {
	if !s.ready(true) {
		return app.RedemptionView{}, false, app.ErrUnavailable
	}
	if _, err := proveStation(ctx, stationID, actor, authority, "redemption.confirm"); err != nil {
		return app.RedemptionView{}, false, err
	}
	var out app.RedemptionView
	var replayed bool
	err := withStationTx(ctx, s.Pool, stationID, func(tx pgx.Tx) error {
		var org, authorizationID, agreementID, employeeID, voucherStation string
		var asset *string
		var product, unit, state string
		var maxQuantity, maxCentavos int64
		var validUntil time.Time
		var attempts int
		err := tx.QueryRow(ctx, `SELECT org_id::text,id::text,agreement_id::text,station_id::text,employee_id::text,asset_id::text,fuel_product,unit,max_quantity_milli,max_centavos,valid_until,state,failed_attempts
			FROM org_fuel_authorizations WHERE code_hash=$1 FOR UPDATE`, digestCode(in.Code)).Scan(&org, &authorizationID, &agreementID, &voucherStation, &employeeID, &asset, &product, &unit, &maxQuantity, &maxCentavos, &validUntil, &state, &attempts)
		if errors.Is(err, pgx.ErrNoRows) {
			// An unknown code reveals nothing (V-PRIV).
			return app.ErrNotFound
		}
		if err != nil {
			return app.ErrUnavailable
		}
		if err := joinOrgScope(ctx, tx, org); err != nil {
			return err
		}
		// The once-only fact answers every matching retry identically
		// (BR-AGREE-002), before any new command is considered.
		if state == "REDEEMED" {
			var confirmedAt time.Time
			var storedAsset *string
			err := tx.QueryRow(ctx, `SELECT id::text,agreement_id::text,authorization_id::text,station_id::text,employee_id::text,asset_id::text,fuel_product,unit,quantity_milli,price_milli_brl,total_centavos,confirmed_at
				FROM org_fuel_redemptions WHERE org_id=$1::uuid AND authorization_id=$2::uuid AND station_id=$3::uuid`,
				org, authorizationID, stationID).Scan(&out.ID, &out.AgreementID, &out.AuthorizationID, &out.StationID, &out.EmployeeID, &storedAsset, &out.Product, &out.Unit, &out.QuantityMilli, &out.PriceMilliBRL, &out.TotalCentavos, &confirmedAt)
			if err != nil {
				return app.ErrConflict
			}
			if storedAsset != nil {
				out.AssetID = *storedAsset
			}
			out.ConfirmedAt = stamp(confirmedAt)
			replayed = true
			return nil
		}
		if voucherStation != stationID {
			// A foreign voucher is a bounded invalid attempt, never data:
			// the recorded attempt commits with the refusal.
			if err := recordFailedAttempt(ctx, tx, org, authorizationID, actor, "wrong station"); err != nil {
				return err
			}
			return committedRefusal{app.ErrStationMismatch}
		}
		if raw, hit, err := commandCheck(ctx, tx, org, "agree.station.redeem", in.OperationID, digestCode(in.OperationID+in.Code)); err != nil {
			return err
		} else if hit {
			replayed = true
			return json.Unmarshal(raw, &out)
		}
		var agreementState string
		var revision int64
		var from, until time.Time
		if err := tx.QueryRow(ctx, `SELECT state,revision,valid_from,valid_until FROM org_station_agreements WHERE org_id=$1::uuid AND id=$2::uuid`,
			org, agreementID).Scan(&agreementState, &revision, &from, &until); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return app.ErrNotFound
			}
			return app.ErrUnavailable
		}
		if !d.AgreementOf(d.State(agreementState), d.WithWindow(from, until)).Usable(s.now()) {
			return app.ErrConflict
		}
		var price int64
		if err := tx.QueryRow(ctx, `SELECT price_milli_brl FROM org_agreement_price_terms WHERE org_id=$1::uuid AND agreement_id=$2::uuid AND fuel_product=$3 AND unit=$4`,
			org, agreementID, product, unit).Scan(&price); err != nil {
			return app.ErrConflict
		}
		total := d.TotalCentavos(in.QuantityMilli, price)
		voucher := d.VoucherOf(d.VoucherState(state), d.VoucherProduct(product, unit), d.VoucherLimits(maxQuantity, maxCentavos), d.VoucherValidUntil(validUntil), d.VoucherAttempts(attempts))
		if err := voucher.Consume(in.Product, in.Unit, in.QuantityMilli, total, s.now()); err != nil {
			switch {
			case errors.Is(err, d.ErrAuthorizationProductMismatch), errors.Is(err, d.ErrAuthorizationQuantityExceeded), errors.Is(err, d.ErrAuthorizationTotalExceeded):
				// The bounded attempt commits with its typed refusal.
				if err := recordFailedAttempt(ctx, tx, org, authorizationID, actor, d.ErrorCode(err)); err != nil {
					return err
				}
				return committedRefusal{err}
			}
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE org_fuel_authorizations SET state='REDEEMED' WHERE org_id=$1::uuid AND id=$2::uuid AND state='ISSUED'`,
			org, authorizationID)
		if err != nil {
			return mapPGErr(err)
		}
		if tag.RowsAffected() != 1 {
			return app.ErrConflict
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_redemptions(org_id,id,authorization_id,agreement_id,station_id,employee_id,asset_id,fuel_product,unit,quantity_milli,price_milli_brl,total_centavos,operation_id,confirmed_by_account_id)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7,$8,$9,$10,$11,$12,$13::uuid,$14::uuid)`,
			org, redemptionID, authorizationID, agreementID, stationID, employeeID, asset, product, unit, in.QuantityMilli, price, total, in.OperationID, actor.AccountID); err != nil {
			return mapPGErr(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_authorization_events(org_id,authorization_id,seq,kind,actor_kind,actor_account_id,detail)
			SELECT $1::uuid,$2::uuid,(SELECT COALESCE(MAX(seq),0)+1 FROM org_fuel_authorization_events WHERE org_id=$1::uuid AND authorization_id=$2::uuid),'REDEEM','STATION',$3::uuid,''`,
			org, authorizationID, actor.AccountID); err != nil {
			return mapPGErr(err)
		}
		var confirmedAt time.Time
		var storedAsset *string
		if err := tx.QueryRow(ctx, `SELECT id::text,agreement_id::text,authorization_id::text,station_id::text,employee_id::text,asset_id::text,fuel_product,unit,quantity_milli,price_milli_brl,total_centavos,confirmed_at
			FROM org_fuel_redemptions WHERE org_id=$1::uuid AND id=$2::uuid`, org, redemptionID).Scan(&out.ID, &out.AgreementID, &out.AuthorizationID, &out.StationID, &out.EmployeeID, &storedAsset, &out.Product, &out.Unit, &out.QuantityMilli, &out.PriceMilliBRL, &out.TotalCentavos, &confirmedAt); err != nil {
			return app.ErrUnavailable
		}
		if storedAsset != nil {
			out.AssetID = *storedAsset
		}
		out.ConfirmedAt = stamp(confirmedAt)
		if err := commandSave(ctx, tx, org, "agree.station.redeem", in.OperationID, redemptionID, digestCode(in.OperationID+in.Code), out); err != nil {
			return err
		}
		orgActor := app.Actor{AccountID: actor.AccountID}
		return s.appendAudit(ctx, tx, orgScopeOfMust(org), orgActor, "agree.redemption.confirmed", "redemption", redemptionID, "")
	})
	return out, replayed, err
}

func orgScopeOfMust(org string) kernel.OrgScope {
	id, err := kernel.ParseOrgID(org)
	if err != nil {
		return kernel.OrgScope{}
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}
	}
	return scope
}

func recordFailedAttempt(ctx context.Context, tx pgx.Tx, org, authorizationID string, actor app.StationActor, detail string) error {
	tag, err := tx.Exec(ctx, `UPDATE org_fuel_authorizations SET failed_attempts=failed_attempts+1,state=CASE WHEN failed_attempts+1>=5 THEN 'LOCKED' ELSE state END WHERE org_id=$1::uuid AND id=$2::uuid AND state='ISSUED'`,
		org, authorizationID)
	if err != nil {
		return mapPGErr(err)
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `INSERT INTO org_fuel_authorization_events(org_id,authorization_id,seq,kind,actor_kind,actor_account_id,detail)
			SELECT $1::uuid,$2::uuid,(SELECT COALESCE(MAX(seq),0)+1 FROM org_fuel_authorization_events WHERE org_id=$1::uuid AND authorization_id=$2::uuid),'FAILED_ATTEMPT','STATION',$3::uuid,$4`,
			org, authorizationID, actor.AccountID, detail); err != nil {
			return mapPGErr(err)
		}
	}
	return nil
}
