//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbstationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	agreeadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/adapters"
	agreeweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/adapters/http"
	agreeapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/agreement/application"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	orgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	profileadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/adapters"
	profileapplication "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	orgtest "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// agreementStation seeds one directory station with its operator revision,
// an approved claim path and an active representation grant carrying the
// F7 scopes for the given account (BR-AGREE-001: the station party).
func agreementStation(t *testing.T, pool *pgxpool.Pool, accountID, scopes string) string {
	t.Helper()
	return agreementStationForOperator(t, pool, accountID, "04218406000104", scopes)
}

func agreementStationForOperator(t *testing.T, pool *pgxpool.Pool, accountID, cnpj, scopes string) string {
	t.Helper()
	ctx := context.Background()
	stationID := onboardingID(t)
	if _, err := pool.Exec(ctx, "INSERT INTO directory_stations (id, display_name) VALUES ($1, 'Posto Sintético F7')", stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_operator_revisions (id, station_id, cnpj, source) VALUES (gen_random_uuid(), $1, $2, 'registry')", stationID, cnpj); err != nil {
		t.Fatalf("seed operator: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO station_profiles (station_id, policy_version) VALUES ($1, 'profile-v1') ON CONFLICT DO NOTHING", stationID); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	claimID, decisionID, grantID := onboardingID(t), onboardingID(t), onboardingID(t)
	if _, err := pool.Exec(ctx, "INSERT INTO profile_claims (id, account_id, station_id, role, policy_version, state) VALUES ($1, $2, $3, 'administrator', 'profile-v1', 'approved')", claimID, accountID, stationID); err != nil {
		t.Fatalf("seed claim: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO claim_decisions (id, claim_id, reviewer, decision, reason, policy_version, operator_cnpj, scopes) VALUES ($1, $2, 'op-f7', 'approved', 'seed', 'profile-v1', $3, $4)", decisionID, claimID, cnpj, scopes); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO representation_grants (id, account_id, station_id, operator_cnpj, role, scopes, claim_id, decision_id) VALUES ($1, $2, $3, $4, 'administrator', $5, $6, $7)", grantID, accountID, stationID, cnpj, scopes, claimID, decisionID); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	return stationID
}

type f7StationSession struct {
	account string
	family  string
	access  string
}

func f7StationLogin(t *testing.T, accounts *account.Service) f7StationSession {
	t.Helper()
	key, err := accounts.CreateKeyAccount(t.Context(), "synf7station"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	proof, err := accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	return f7StationSession{account: proof.Account.ID, family: proof.Session.FamilyID, access: proof.Session.AccessToken}
}

func f7StationCall(t *testing.T, router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, session f7StationSession, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	if body == nil {
		body = map[string]any{}
	}
	body["family_id"] = session.family
	body["access_token"] = session.access
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		out = map[string]any{}
	}
	return w.Code, out
}

// TestAgreementF7EndToEndFlow drives the whole Phase F7 boundary through
// the actual composed company routes and the station-session surface:
// proposal, explicit station acceptance, bounded issuance, the once-only
// redemption with its matching-retry receipt, the recorded lifecycle and
// the negative cases (foreign station, expired, over-limit, wrong product,
// capability guard and bounded attempts), all under the ordinary runtime
// role with real PostGIS (EMP-AGREE-001..006, BR-AGREE-001/002).
func TestAgreementF7EndToEndFlow(t *testing.T) {
	fx := devicesFixture(t)
	grantEmployeeTables(t, fx.legalFixtureState)
	grantF7AgreementTables(t, fx.f)
	grantStationReads(t, fx.f)
	router := chi.NewRouter()
	runtimeAccounts := *fx.accounts
	runtimeAccounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{59}, 32), Pool: fx.f.Runtime, Accounts: &runtimeAccounts}); err != nil {
		t.Fatal("composed registration failed", err)
	}
	// The station party's surface on the same router: its session proof and
	// live station authority are the same production ports.
	agreementCatalog, err := orgaudit.NewCatalog(agreeadapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	stationStore := &agreeadapters.Store{Pool: fx.f.Runtime, Audit: auditpg.NewStore(fx.f.Runtime, agreementCatalog), NewID: platformjobs.NewUUIDv4, Now: time.Now}
	accountStore := accountpg.NewPGStore(fx.f.Runtime)
	stationAccounts := &account.Service{Clock: onboardingClock{time.Now().UTC().Truncate(time.Second)}, Store: accountStore, Hasher: accountdomain.SHA256Hasher{}, IDGen: platformjobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	profileQueries := dbstationprofile.New(fx.f.Runtime)
	operatorStore := profileadapters.Store{Q: profileQueries}
	agreeweb.StationHandler{
		Service: agreeapp.Service{Enabled: true, Repository: stationStore, Now: time.Now, NewID: platformjobs.NewUUIDv4, NewCode: platformjobs.NewVoucherCode},
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return stationAccounts.ValidateAccess(ctx, familyID, accessToken)
		},
		Authority: profileapplication.StationAuthority{
			Grants: profileadapters.ReviewDecisions{Pool: fx.f.Runtime},
			OperatorOf: func(ctx context.Context, stationID string) (string, bool, error) {
				operator, found, err := operatorStore.CurrentOperator(ctx, stationID)
				if err != nil || !found {
					return "", false, err
				}
				return operator.CNPJ, true, nil
			},
			AccountLive: func(ctx context.Context, accountID string) (bool, error) {
				acc, found, err := accountStore.GetAccount(ctx, accountID)
				if err != nil || !found {
					return false, err
				}
				return acc.Active(), nil
			},
		},
	}.RegisterStationRoutes(router)

	owner := matrixNewPerson(t, fx, router)
	matrixAssign(t, fx, owner, "OWNER", "ORG", "ACTIVE")
	org := fx.scope.OrgID().String()
	stationSession := f7StationLogin(t, fx.accounts)
	stationID := agreementStation(t, fx.f.Admin, stationSession.account, "profile.edit,reply.official,price.publish,agreement.manage,redemption.confirm")

	// UC-AGREE-01: eligible representatives propose and explicitly accept.
	// The window starts immediately so the redemption path exercises the
	// live validity, and the bounded expiry stays in the future.
	from := time.Now().UTC().Truncate(time.Second)
	until := from.Add(30 * 24 * time.Hour)
	proposeBody := func(operation string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": operation, "station_id": stationID, "org_label": "Transportes Vale",
			"valid_from": from.Format(time.RFC3339Nano), "valid_until": until.Format(time.RFC3339Nano),
			"conditions": "Faturamento mensal mediante nota fiscal",
			"terms":      []map[string]any{{"fuel_product": "DIESEL_S10", "unit": "L", "price_milli_brl": 5999}}}
	}
	operation := onboardingID(t)
	proposed := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/propose", proposeBody(operation), 200, "", "", nil)
	agreement, _ := proposed["agreement"].(map[string]any)
	if agreement == nil || agreement["state"] != "PROPOSED" || agreement["revision"] != float64(1) || proposed["replayed"] != false {
		t.Fatal("proposal receipt", proposed)
	}
	agreementID, _ := agreement["id"].(string)
	// The idempotent command receipt replays the same proposal.
	if replay := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/propose", proposeBody(operation), 200, "", "", nil); replay["replayed"] != true {
		t.Fatal("proposal replay", replay)
	}
	// The same identifier with a different request is refused.
	reused := proposeBody(operation)
	reused["org_label"] = "Transportes Dois"
	if code := matrixCallStatus(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/propose", reused); code != http.StatusConflict {
		t.Fatal("expected operation reuse refusal, got", code)
	}

	// The station party sees only the allowlisted fields (BR-AGREE-001):
	// no organization identity, membership, fleet or document fields.
	code, stationList := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/agreements/list", nil)
	if code != http.StatusOK {
		t.Fatal("station agreements list", code)
	}
	rows, _ := stationList["agreements"].([]any)
	if len(rows) != 1 {
		t.Fatal("station agreements count", len(rows))
	}
	view, _ := rows[0].(map[string]any)
	if view["id"] != agreementID || view["org_label"] != "Transportes Vale" || view["state"] != "PROPOSED" {
		t.Fatal("station agreement view", view)
	}
	for _, forbidden := range []string{"org_id", "proposed_by_membership_id", "employee_id", "station_id"} {
		if _, present := view[forbidden]; present {
			t.Fatal("station view leaked a private field", forbidden)
		}
	}

	// The station party accepts explicitly; a repeated decision is refused
	// by the recorded state machine.
	if code, accepted := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/agreements/"+agreementID+"/accept", nil); code != http.StatusOK || accepted["agreement"].(map[string]any)["state"] != "ACTIVE" {
		t.Fatal("station acceptance", code)
	}
	if code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/agreements/"+agreementID+"/accept", nil); code != http.StatusConflict {
		t.Fatal("repeated acceptance must conflict")
	}

	// EMP-AGREE-004: bounded, expiring, single-use authorization. The code
	// is answered exactly once; only its digest is stored.
	driverEmployee := matrixNewPerson(t, fx, router)
	issueBody := func(operation string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": operation, "agreement_id": agreementID,
			"employee_id": driverEmployee.employee, "fuel_product": "DIESEL_S10", "unit": "L",
			"max_quantity_milli": 200000, "max_centavos": 300000, "valid_until": until.Format(time.RFC3339Nano)}
	}
	issueOperation := onboardingID(t)
	issued := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(issueOperation), 200, "", "", nil)
	ack, _ := issued["authorization"].(map[string]any)
	if ack == nil || ack["state"] != "ISSUED" {
		t.Fatal("issue receipt", issued)
	}
	voucherCode, _ := issued["code"].(string)
	if len(voucherCode) != 32 {
		t.Fatal("one-time code missing", voucherCode)
	}
	authorizationID, _ := ack["id"].(string)
	// The organization reads the voucher; the one-time code never repeats.
	read := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/"+authorizationID+"/read", map[string]any{}, 200, "", "", nil)
	if read["authorization"].(map[string]any)["id"] != authorizationID || read["code"] != nil {
		t.Fatal("authorization read", read)
	}
	// A replay answers the stored authorization view and never the code.
	if replay := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(issueOperation), 200, "", "", nil); replay["replayed"] != true || replay["code"] != nil {
		t.Fatal("issue replay leaked the one-time code", replay)
	}
	// A product outside the negotiated terms is refused.
	outside := issueBody(onboardingID(t))
	outside["fuel_product"] = "GASOLINE_REGULAR"
	if code := matrixCallStatus(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", outside); code != http.StatusUnprocessableEntity {
		t.Fatal("expected term refusal, got", code)
	}

	// EMP-AGREE-005: the once-only redemption. A matching retry answers the
	// stored receipt identically and the database keeps exactly one record.
	type f7Call struct {
		code int
		body map[string]any
	}
	confirm := func(operation string) f7Call {
		code, body := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
			"operation_id": operation, "code": voucherCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 50000})
		return f7Call{code: code, body: body}
	}
	firstOperation := onboardingID(t)
	first, second := confirm(firstOperation), confirm(onboardingID(t))
	if first.code != http.StatusOK || second.code != http.StatusOK {
		t.Fatal("matching confirmations", first.code, second.code)
	}
	firstRedemption, _ := first.body["redemption"].(map[string]any)
	secondRedemption, _ := second.body["redemption"].(map[string]any)

	// BR-AGREE-001 / VAL-34: the station sees only settlement facts,
	// both on the original response and on a matching retry.
	stationFields := []string{"id", "agreement_id", "authorization_id", "station_id",
		"fuel_product", "unit", "quantity_milli", "price_milli_brl", "total_centavos", "confirmed_at"}
	for _, receipt := range []map[string]any{firstRedemption, secondRedemption} {
		if len(receipt) != len(stationFields) {
			t.Fatal("station settlement contains fields outside its allowlist")
		}
		for _, field := range stationFields {
			if receipt[field] == nil {
				t.Fatalf("station settlement is missing %s", field)
			}
		}
	}

	if firstRedemption["id"] != secondRedemption["id"] {
		t.Fatal("competing use settled twice", firstRedemption["id"], secondRedemption["id"])
	}
	if firstRedemption["total_centavos"] != float64(299950) || firstRedemption["price_milli_brl"] != float64(5999) {
		t.Fatal("server-derived settlement", firstRedemption)
	}
	if first.body["replayed"] == second.body["replayed"] {
		t.Fatal("exactly one confirmation is the original", first.body["replayed"], second.body["replayed"])
	}
	var settled int
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM org_fuel_redemptions WHERE org_id=$1", org).Scan(&settled); err != nil || settled != 1 {
		t.Fatalf("redemption count: got %d (%v)", settled, err)
	}

	// A changed settlement must never receive an acknowledgement for the
	// original amount, even if the caller keeps the original operation ID.
	for _, mismatch := range []struct {
		product, unit string
		quantity      int
	}{
		{"DIESEL_S10", "L", 51000},
		{"GASOLINE_REGULAR", "L", 50000},
		{"DIESEL_S10", "M3", 50000},
	} {
		status, body := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
			"operation_id": firstOperation, "code": voucherCode, "fuel_product": mismatch.product, "unit": mismatch.unit, "quantity_milli": mismatch.quantity,
		})
		if status != http.StatusConflict || body["redemption"] != nil {
			t.Fatalf("changed settlement confirmed: %+v returned %d", mismatch, status)
		}
	}

	// Competing use cannot consume a fresh voucher twice: two concurrent
	// confirmations converge on the single stored receipt.
	raceIssue := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(onboardingID(t)), 200, "", "", nil)
	raceCode, _ := raceIssue["code"].(string)

	if status, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
		"operation_id": firstOperation, "code": raceCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 10000,
	}); status != http.StatusConflict {
		t.Fatalf("operation reuse for a different voucher returned %d", status)
	}

	raceOperations := []string{onboardingID(t), onboardingID(t)}
	raceResults := make(chan f7Call, 2)
	for _, operation := range raceOperations {
		go func(operation string) {
			code, body := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
				"operation_id": operation, "code": raceCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 10000})
			raceResults <- f7Call{code: code, body: body}
		}(operation)
	}
	raceA, raceB := <-raceResults, <-raceResults
	if raceA.code != http.StatusOK || raceB.code != http.StatusOK {
		t.Fatal("concurrent confirmation refused", raceA.code, raceB.code)
	}
	raceFirst, _ := raceA.body["redemption"].(map[string]any)
	raceSecond, _ := raceB.body["redemption"].(map[string]any)
	if raceFirst["id"] != raceSecond["id"] || raceA.body["replayed"] == raceB.body["replayed"] {
		t.Fatal("concurrent use settled twice", raceFirst["id"], raceSecond["id"])
	}
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM org_fuel_redemptions WHERE org_id=$1", org).Scan(&settled); err != nil || settled != 2 {
		t.Fatalf("redemption count after race: got %d (%v)", settled, err)
	}
	// Matching retries answer the stored receipt identically.
	if receipt := confirm(onboardingID(t)); receipt.code != http.StatusOK || receipt.body["replayed"] != true || receipt.body["redemption"].(map[string]any)["id"] != firstRedemption["id"] {
		t.Fatal("matching retry", receipt)
	}

	// The organization reads both settlements under the agreement scope.
	redemptions := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-redemptions/list", map[string]any{}, 200, "", "", nil)
	fleetRows, _ := redemptions["redemptions"].([]any)
	if len(fleetRows) != 2 {
		t.Fatal("organization redemption list", redemptions)
	}
	for _, row := range fleetRows {
		if row.(map[string]any)["employee_id"] != driverEmployee.employee {
			t.Fatal("fleet-side settlement lost its employee reference")
		}
	}

	// Negative cases: unknown code reveals nothing; wrong product and
	// over-limit tries are bounded attempts.
	if code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
		"operation_id": onboardingID(t), "code": "ffffffffffffffffffffffffffffffff", "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 1000}); code != http.StatusNotFound {
		t.Fatal("unknown code must reveal nothing", code)
	}
	// A second voucher exercises the attempt bounds and expiry.
	secondIssue := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(onboardingID(t)), 200, "", "", nil)
	secondCode, _ := secondIssue["code"].(string)
	// Five bounded invalid attempts lock the voucher (BR-TEN-005): the answers
	// still carry each attempt's own refusal, and the next confirm — even
	// still carries the attempt's own refusal, and the next confirm — even
	// with the matching product — answers the lock.
	for range 5 {
		code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
			"operation_id": onboardingID(t), "code": secondCode, "fuel_product": "GASOLINE_REGULAR", "unit": "L", "quantity_milli": 1000})
		if code != http.StatusUnprocessableEntity {
			t.Fatal("mismatched product must be a bounded refusal")
		}
	}
	if code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
		"operation_id": onboardingID(t), "code": secondCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 1000}); code != http.StatusConflict {
		t.Fatal("bounded attempts must lock the voucher", code)
	}
	var failedAttempts int
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT failed_attempts FROM org_fuel_authorizations WHERE org_id=$1 AND id=$2", org, secondIssue["authorization"].(map[string]any)["id"]).Scan(&failedAttempts); err != nil || failedAttempts != 5 {
		t.Fatalf("bounded attempts: got %d (%v)", failedAttempts, err)
	}

	// The organization cancels an issued voucher.
	thirdIssue := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(onboardingID(t)), 200, "", "", nil)
	thirdAuthorization, _ := thirdIssue["authorization"].(map[string]any)
	thirdCode, _ := thirdIssue["code"].(string)
	cancelBody := func(operation string) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": operation, "reason": "Motorista realocado"}
	}
	cancelled := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/"+thirdAuthorization["id"].(string)+"/cancel", cancelBody(onboardingID(t)), 200, "", "", nil)
	if cancelled["authorization"].(map[string]any)["state"] != "CANCELLED" {
		t.Fatal("cancellation receipt", cancelled)
	}
	if code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
		"operation_id": onboardingID(t), "code": thirdCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 1000}); code != http.StatusConflict {
		t.Fatal("cancelled voucher must be refused", code)
	}

	// The recorded lifecycle: suspension stops both new authorizations and
	// new redemptions under the recorded policy (UC-AGREE-01).
	suspendBody := func(operation string, revision float64) map[string]any {
		return map[string]any{"schema_version": 1, "operation_id": operation, "revision": revision, "reason": "Revisão contratual em andamento"}
	}
	fourthIssue := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(onboardingID(t)), 200, "", "", nil)
	fourthCode, _ := fourthIssue["code"].(string)
	suspended := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/"+agreementID+"/suspend", suspendBody(onboardingID(t), 2), 200, "", "", nil)
	if suspended["agreement"].(map[string]any)["state"] != "SUSPENDED" {
		t.Fatal("suspension receipt", suspended)
	}
	if code, _ := f7StationCall(t, router, stationSession, "/v1/stations/"+stationID+"/redemptions/confirm", map[string]any{
		"operation_id": onboardingID(t), "code": fourthCode, "fuel_product": "DIESEL_S10", "unit": "L", "quantity_milli": 1000}); code != http.StatusConflict {
		t.Fatal("suspended agreement must stop redemptions", code)
	}
	if code := matrixCallStatus(t, fx, router, owner, "/v1/orgs/"+org+"/fueling-authorizations/issue", issueBody(onboardingID(t))); code != http.StatusConflict {
		t.Fatal("suspended agreement must stop new authorizations", code)
	}
	// Ending keeps the settlement history for reconciliation.
	ended := matrixCall(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/"+agreementID+"/end", suspendBody(onboardingID(t), 3), 200, "", "", nil)
	if ended["agreement"].(map[string]any)["state"] != "ENDED" {
		t.Fatal("ending receipt", ended)
	}

	// The capability guard still gates the F7 operational writes: with the
	// organizations capability disabled, a new proposal is refused and no
	// existing settlement changes.
	if _, err := fx.f.Admin.Exec(t.Context(), "UPDATE org_capabilities SET enabled=false,revision=revision+1 WHERE org_id=$1 AND capability='organizations'", org); err != nil {
		t.Fatal(err)
	}
	if code := matrixCallStatus(t, fx, router, owner, "/v1/orgs/"+org+"/station-agreements/propose", proposeBody(onboardingID(t))); code != http.StatusForbidden {
		t.Fatal("disabled capability must gate new proposals", code)
	}
	var settlements int
	if err := fx.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM org_fuel_redemptions WHERE org_id=$1", org).Scan(&settlements); err != nil || settlements != 2 {
		t.Fatalf("settlements changed: got %d (%v)", settlements, err)
	}
}

// matrixCallStatus performs one composed call and answers only the status.
func matrixCallStatus(t *testing.T, fx deviceFixtureState, router interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, actor matrixPerson, path string, body map[string]any) int {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+path, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", deviceTestOrigin)
	if actor.cookie != nil {
		r.AddCookie(actor.cookie)
		r.Header.Set("X-CSRF", actor.csrf)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w.Code
}

// grantF7AgreementTables mirrors the production runtime grants for the
// Phase F7 relations (runtime-role.sql regenerates them from usage).
func grantF7AgreementTables(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	stmt := `GRANT SELECT,INSERT ON org_f7_commands, org_station_agreement_transitions, org_agreement_price_terms, org_fuel_redemptions, org_fuel_authorization_events TO ` + role + `;
	GRANT SELECT,INSERT,UPDATE ON org_station_agreements, org_fuel_authorizations TO ` + role + `;
	GRANT SELECT,INSERT ON org_audit_events TO ` + role + `;
	GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO ` + role
	if _, err := f.Admin.Exec(t.Context(), stmt); err != nil {
		t.Fatal("grant f7 tables:", err)
	}
}

// grantStationReads gives the fixture runtime the station representation
// reads the station-side authority rechecks (grants, claims, decisions,
// operator revisions and profiles).
func grantStationReads(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	stmt := `GRANT SELECT ON representation_grants, profile_claims, claim_decisions, station_operator_revisions, station_profiles, station_announced_prices, station_promotions TO ` + role
	if _, err := f.Admin.Exec(t.Context(), stmt); err != nil {
		t.Fatal("grant station reads:", err)
	}
}
