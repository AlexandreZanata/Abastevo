//go:build integration

package http

import (
	"context"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/go-chi/chi/v5"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// Opt-in cross-language harness. Synthetic auth exists ONLY in this test binary;
// production sessions and endpoints are not changed. Each run owns a fresh DB.
func TestOperationalWireServer(t *testing.T) {
	path := os.Getenv("ANPFUEL_CHARGING_WIRE_RECEIPT")
	if path == "" {
		t.Skip("explicit cross-language harness not requested")
	}
	p := observationTestPool(t)
	_, err := p.Exec(context.Background(), `INSERT INTO charging_batches(id,format,profile_id,source_revision,manifest_sha256,input_count,accepted_count) VALUES('dddddddd-1111-4111-8111-000000000001','charging-prepared-v1','synthetic-wire','test',repeat('a',64),1,1);
 INSERT INTO charging_places(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points) VALUES('bbbbbbbb-1111-4111-8111-000000000001','dddddddd-1111-4111-8111-000000000001',repeat('b',64),'synthetic-wire-place','Synthetic wire test','BR','0','0','UNKNOWN',0);`)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	CommandsHandler{Store: prepared.ObservationStore{Pool: p}, Sessions: func(_ context.Context, family, token string) (string, error) {
		if family == "bbbbbbbb-1111-4111-8111-000000000001" && token == "synthetic-wire-proof" {
			return "aaaaaaaa-1111-4111-8111-000000000001", nil
		}
		return "", fmt.Errorf("invalid test proof")
	}}.RegisterRoutes(router)
	HistoryHandler{Pool: p}.RegisterHistoryRoutes(router)
	server := httptest.NewServer(router)
	defer server.Close()
	if err := os.WriteFile(path, []byte(server.URL), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(5 * time.Minute)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("wire client did not finish")
		case <-tick.C:
			if _, err := os.Stat(path + ".done"); err == nil {
				return
			}
		}
	}
}
