//go:build integration

package prepared

import (
	"context"
	"errors"
	"fmt"
	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
	"time"
)

const opOwner = "aaaaaaaa-1111-4111-8111-000000000001"
const opOther = "aaaaaaaa-1111-4111-8111-000000000002"
const opPlace = "bbbbbbbb-1111-4111-8111-000000000001"
const opGroup = "cccccccc-1111-4111-8111-000000000001"

func operationalPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	p := testPool(t)
	ctx := context.Background()
	_, err := p.Exec(ctx, `INSERT INTO accounts(id,alias,status) VALUES($1,'op-owner','active'),($2,'op-other','active')`, opOwner, opOther)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Exec(ctx, `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count) VALUES('dddddddd-1111-4111-8111-000000000001','charging-prepared-v1','synthetic-op-test','test',repeat('a',64),1,1);
 INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points) VALUES('bbbbbbbb-1111-4111-8111-000000000001','dddddddd-1111-4111-8111-000000000001',repeat('b',64),'synthetic-place','Synthetic charging place','BR','0','0','UNKNOWN',1);
 INSERT INTO charging_connection_groups(id,batch_id,place_id,entity_key,external_id,connector_code,raw_connector_code,current,reported_quantity) VALUES('cccccccc-1111-4111-8111-000000000001','dddddddd-1111-4111-8111-000000000001','bbbbbbbb-1111-4111-8111-000000000001',repeat('c',64),'synthetic-group','TYPE2','TYPE2','AC',1);`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func opDraft(id string) chargingdomain.Observation {
	place := opPlace
	return chargingdomain.Observation{ID: id, PlaceID: &place, ConnectorCode: "TYPE2", Condition: "UNKNOWN", ObservedDay: time.Now().UTC().Format("2006-01-02"), Note: "Synthetic dated report", Operational: &chargingdomain.OperationalEvent{Event: "WORKED", TargetScope: "SITE", ObservedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}}
}
func TestOperationalEventsPersistAndValidateActualTarget(t *testing.T) {
	p := operationalPool(t)
	s := ObservationStore{Pool: p}
	ctx := context.Background()
	sample := opDraft("eeeeeeee-1111-4111-8111-000000000001")
	if _, err := s.Submit(ctx, opOwner, sample); err != nil {
		t.Fatal(err)
	}
	var event, scope pgtype.Text
	if err := p.QueryRow(ctx, "SELECT operational_event,operational_scope FROM charging_observations WHERE id=$1", sample.ID).Scan(&event, &scope); err != nil {
		t.Fatal(err)
	}
	if !event.Valid || event.String != "WORKED" || scope.String != "SITE" {
		t.Fatal("dated event was discarded", event, scope)
	}
	group := opGroup
	sample.ID = "eeeeeeee-1111-4111-8111-000000000002"
	sample.Operational.TargetScope = "EQUIPMENT"
	sample.Operational.ConnectionGroupID = &group
	if _, err := s.Submit(ctx, opOwner, sample); err != nil {
		t.Fatal("known one-unit source reference refused", err)
	}
	_, err := p.Exec(ctx, "UPDATE charging_connection_groups SET reported_quantity=2 WHERE id=$1", group)
	if err != nil {
		t.Fatal(err)
	}
	sample.ID = "eeeeeeee-1111-4111-8111-000000000003"
	if _, err := s.Submit(ctx, opOwner, sample); !errors.Is(err, chargingdomain.ErrInvalidObservation) {
		t.Fatal("multiple units falsely identify one equipment", err)
	}
	sample.Operational.TargetScope = "CONNECTOR"
	if _, err := s.Submit(ctx, opOwner, sample); err != nil {
		t.Fatal("actual connector group refused", err)
	}
	sample.ID = "eeeeeeee-1111-4111-8111-000000000004"
	sample.ConnectorCode = "CCS2"
	if _, err := s.Submit(ctx, opOwner, sample); !errors.Is(err, chargingdomain.ErrInvalidObservation) {
		t.Fatal("foreign connector class admitted", err)
	}
	sample.ConnectorCode = "TYPE2"
	sample.Operational.TargetScope = "SITE"
	sample.Operational.ConnectionGroupID = nil
	sample.Operational.ObservedAt = time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	if _, err := s.Submit(ctx, opOwner, sample); !errors.Is(err, chargingdomain.ErrInvalidObservation) {
		t.Fatal("future report admitted", err)
	}
}
func TestChargingSubmitReplayBindsBodyAndOwner(t *testing.T) {
	p := operationalPool(t)
	s := ObservationStore{Pool: p}
	ctx := context.Background()
	sample := opDraft("eeeeeeee-1111-4111-8111-000000000010")
	if _, err := s.Submit(ctx, opOwner, sample); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, opOwner, sample); err != nil {
		t.Fatal("identical replay refused", err)
	}
	if _, err := s.Submit(ctx, opOther, sample); err == nil {
		t.Fatal("foreign owner collision accepted")
	}
	sample.Note = strings.Repeat("z", 8)
	if _, err := s.Submit(ctx, opOwner, sample); !errors.Is(err, chargingdomain.ErrConflict) {
		t.Fatal("divergent replay accepted", err)
	}
	var n int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM charging_observations").Scan(&n); err != nil || n != 1 {
		t.Fatal("replay changed census", n, err)
	}
}

func TestChargingCommandReceiptsSurviveLostReplyAndModeration(t *testing.T) {
	p := operationalPool(t)
	s := ObservationStore{Pool: p}
	ctx := context.Background()
	draft := opDraft("eeeeeeee-1111-4111-8111-000000000020")
	if _, err := s.Submit(ctx, opOwner, draft); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Exec(ctx, "UPDATE charging_observations SET hidden_at=now() WHERE id=$1", draft.ID); err != nil {
		t.Fatal(err)
	}
	draft.Note = "Corrected dated report"
	command := "ffffffff-1111-4111-8111-000000000001"
	for i := 0; i < 2; i++ {
		if rev, err := s.CorrectCommand(ctx, opOwner, draft, 1, command); err != nil || rev != 2 {
			t.Fatal("lost correction reply", rev, err)
		}
	}
	draft.Note = "different"
	if _, err := s.CorrectCommand(ctx, opOwner, draft, 1, command); !errors.Is(err, chargingdomain.ErrConflict) {
		t.Fatal("key reused with changed body", err)
	}
	var hidden bool
	if err := p.QueryRow(ctx, "SELECT hidden_at IS NOT NULL FROM charging_observations WHERE id=$1", draft.ID).Scan(&hidden); err != nil || !hidden {
		t.Fatal("moderation lost", err)
	}
	withdraw := "ffffffff-1111-4111-8111-000000000002"
	for i := 0; i < 2; i++ {
		if rev, err := s.WithdrawCommand(ctx, opOwner, draft.ID, 2, withdraw); err != nil || rev != 3 {
			t.Fatal("lost withdraw reply", rev, err)
		}
	}
	if _, err := s.CorrectCommand(ctx, opOwner, draft, 3, "ffffffff-1111-4111-8111-000000000003"); err == nil {
		t.Fatal("resurrected tombstone")
	}
	var receipts int
	if err := p.QueryRow(ctx, "SELECT count(*) FROM charging_observation_commands").Scan(&receipts); err != nil || receipts != 3 {
		t.Fatal("failed command persisted", receipts, err)
	}
}

func TestChargingCommandConcurrencyAndCancellation(t *testing.T) {
	p := operationalPool(t)
	s := ObservationStore{Pool: p}
	ctx := context.Background()
	draft := opDraft("eeeeeeee-1111-4111-8111-000000000030")
	barrier := make(chan struct{})
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { <-barrier; _, err := s.Submit(ctx, opOwner, draft); errs <- err }()
	}
	close(barrier)
	for i := 0; i < 8; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	other := opDraft("eeeeeeee-1111-4111-8111-000000000031")
	if _, err := s.Submit(cancelled, opOwner, other); err == nil {
		t.Fatal("cancelled write accepted")
	}
	var rows, receipts int
	if err := p.QueryRow(ctx, "SELECT (SELECT count(*) FROM charging_observations),(SELECT count(*) FROM charging_observation_commands)").Scan(&rows, &receipts); err != nil || rows != 1 || receipts != 1 {
		t.Fatal("non atomic command", rows, receipts, err)
	}
}

func TestOperationalQuotaConcurrentDistinctCommands(t *testing.T) {
	p := operationalPool(t)
	ctx := context.Background()
	s := ObservationStore{Pool: p}
	_, err := p.Exec(ctx, `INSERT INTO charging_observations(id,account_id,connector_code,condition,observed_day,note)
 SELECT md5('synthetic-quota-'||n::text)::uuid,$1,'TYPE2','UNKNOWN',CURRENT_DATE,'Synthetic quota fixture'
 FROM generate_series(1,$2::int) n`, opOwner, chargingdomain.MaxObservationsPerAccount-1)
	if err != nil {
		t.Fatal(err)
	}
	barrier := make(chan struct{})
	results := make(chan error, 8)
	for i := 1; i <= 8; i++ {
		draft := opDraft(fmt.Sprintf("eeeeeeee-2222-4222-8222-%012d", i))
		go func() { <-barrier; _, err := s.Submit(ctx, opOwner, draft); results <- err }()
	}
	close(barrier)
	successes := 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if !errors.Is(err, chargingdomain.ErrQuotaExceeded) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("quota race", successes)
	}
	var rows, receipts int
	if err := p.QueryRow(ctx, "SELECT (SELECT count(*) FROM charging_observations),(SELECT count(*) FROM charging_observation_commands)").Scan(&rows, &receipts); err != nil || rows != chargingdomain.MaxObservationsPerAccount || receipts != 1 {
		t.Fatal("quota/receipt atomicity", rows, receipts, err)
	}
}
