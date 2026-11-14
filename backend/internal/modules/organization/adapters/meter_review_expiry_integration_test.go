//go:build integration

package adapters_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// review-expiry-v1 (EDR-119/B-BR-EMP-168) is elapsed 60x86400 seconds.
const reviewExpiryElapsed = 60 * 86400 * time.Second

// posixJulian is the POSIX TZ "Jn" day: 1..365 with February 29 never counted.
func posixJulian(t time.Time) int {
	day := t.YearDay()
	if y := t.Year(); (y%4 == 0 && y%100 != 0 || y%400 == 0) && t.Month() > time.February {
		day--
	}
	return day
}

// dstSessionZone returns a POSIX session TimeZone whose DST rule starts ten
// days after the actual receipt, so the review window always spans a real
// session-zone transition regardless of the date this test runs. No tzdata
// zone has a transition inside every possible 60-day window (none does after
// 2026-11-14, for example), so a named zone alone cannot prove the rule.
func dstSessionZone(received time.Time) string {
	return fmt.Sprintf("<-03>3<-02>,J%d/2,J%d/2", posixJulian(received.Add(10*24*time.Hour)), posixJulian(received.Add(200*24*time.Hour)))
}

func reviewIDsAsOf(t *testing.T, fx syncFixtureState, pool *pgxpool.Pool, state string, at time.Time) []string {
	t.Helper()
	var ids []string
	err := database.WithOrgTx(t.Context(), pool, fx.scope, func(tx pgx.Tx) error {
		var err error
		ids, err = q.New(tx).ListMeterReviewIDs(t.Context(), q.ListMeterReviewIDsParams{OrgID: fx.scope.OrgID().String(), State: state, At: pgtype.Timestamptz{Time: at, Valid: true}, RowLimit: 50})
		return err
	})
	if err != nil {
		t.Fatal("actual as-of review listing unavailable", err)
	}
	return ids
}

func TestMeterReviewExpiryIsElapsedSecondsUnderNonUTCSessionDST(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	_, open, openAck, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour), 7))
	if err != nil {
		t.Fatal(err)
	}
	_, resolved, resolvedAck, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, time.Now().UTC().Add(-30*time.Minute), 7))
	if err != nil {
		t.Fatal(err)
	}
	received, err := orgtime.ParseInstant(openAck.Snapshot().ReceivedAt)
	if err != nil {
		t.Fatal(err)
	}
	resolvedReceived, err := orgtime.ParseInstant(resolvedAck.Snapshot().ReceivedAt)
	if err != nil {
		t.Fatal(err)
	}
	deadline, resolvedDeadline := received.Add(reviewExpiryElapsed), resolvedReceived.Add(reviewExpiryElapsed)
	rule := dstSessionZone(received)
	var shifts bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT extract(epoch FROM($1::timestamptz AT TIME ZONE $3)-($1::timestamptz AT TIME ZONE 'UTC'))<>extract(epoch FROM($2::timestamptz AT TIME ZONE $3)-($2::timestamptz AT TIME ZONE 'UTC'))`, received, deadline, rule).Scan(&shifts); err != nil || !shifts {
		t.Fatal("session zone does not cross an actual DST transition inside the review window", err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	t.Cleanup(func() { _, _ = fx.f.Admin.Exec(context.Background(), "ALTER ROLE "+role+" RESET timezone") })
	decided := false
	for _, zone := range []string{"UTC", "Europe/Berlin", "America/Sao_Paulo", rule} {
		t.Run(zone, func(t *testing.T) {
			// A role-level default is a deployment condition, not a new setting:
			// every fresh ordinary runtime session below uses this TimeZone.
			var statement string
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT format('ALTER ROLE %s SET timezone TO %L',$1::text,$2::text)`, role, zone).Scan(&statement); err != nil {
				t.Fatal(err)
			}
			if _, err := fx.f.Admin.Exec(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
			pool := fx.f.RuntimePool(t, 2)
			var session string
			if err := pool.QueryRow(t.Context(), `SHOW timezone`).Scan(&session); err != nil || session != zone {
				t.Fatal("runtime session did not use the selected TimeZone", session, err)
			}
			zoned := *store
			zoned.Pool = pool
			view, err := zoned.ReadMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, open.ReviewItemID)
			if err != nil || view.State != app.MeterReviewOpen || !view.ReceivedAt.Equal(received) || !view.ExpiresAt.Equal(deadline) {
				t.Errorf("review deadline depends on session TimeZone: got %s want %s (%v)", view.ExpiresAt.Format(time.RFC3339Nano), deadline.Format(time.RFC3339Nano), err)
			}
			before := deadline.Add(-time.Microsecond)
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewOpen, before); !slices.Contains(ids, open.ReviewItemID) {
				t.Error("review expired before its exact elapsed deadline")
			}
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewExpired, before); slices.Contains(ids, open.ReviewItemID) {
				t.Error("review expired before its exact elapsed deadline")
			}
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewExpired, deadline); !slices.Contains(ids, open.ReviewItemID) {
				t.Error("unresolved review was not expired at its exact deadline")
			}
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewOpen, deadline); slices.Contains(ids, open.ReviewItemID) {
				t.Error("unresolved review stayed open at its exact deadline")
			}
			if zone != rule {
				return
			}
			// The decision path in the DST session never consults expiry; the
			// immutable resolution outranks EXPIRED_UNRESOLVED at later as-of times.
			decision, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: resolved.ReviewItemID, ExpectedRevision: 1, Decision: app.MeterReviewAccept})
			if err != nil {
				t.Fatal(err)
			}
			if result, replay, err := zoned.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, decision); err != nil || replay || result.Snapshot().Outcome != protocol.ReviewAccepted {
				t.Fatal("current reviewer cannot resolve under a DST session", err)
			}
			decided = true
			view, err = zoned.ReadMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, resolved.ReviewItemID)
			if err != nil || view.State != app.MeterReviewAccepted || !view.ExpiresAt.Equal(resolvedDeadline) {
				t.Fatal("resolved review lost its exact deadline or outcome", err)
			}
			late := resolvedDeadline.Add(24 * time.Hour)
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewAccepted, late); !slices.Contains(ids, resolved.ReviewItemID) {
				t.Fatal("resolution was hidden after the deadline")
			}
			if ids := reviewIDsAsOf(t, fx, pool, app.MeterReviewExpired, late); slices.Contains(ids, resolved.ReviewItemID) || !slices.Contains(ids, open.ReviewItemID) {
				t.Fatal("expiry outranked the immutable resolution")
			}
		})
	}
	if !decided {
		t.Fatal("DST-session decision was not exercised")
	}
}
