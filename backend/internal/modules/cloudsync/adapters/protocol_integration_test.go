//go:build integration

package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	cloudhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/go-chi/chi/v5"
)

func protocolHandler(t *testing.T, store *Store, owners map[string]string, keys map[string][2]string) cloudhttp.Handler {
	t.Helper()
	return cloudhttp.Handler{
		Service:      store,
		Origin:       store.Origin,
		CursorSecret: []byte(strings.Repeat("s", 32)),
		Sessions: func(_ context.Context, family, token string) (string, error) {
			owner, ok := owners[family]
			if !ok || token != "synthetic" {
				return "", errUnauthorized()
			}
			return owner, nil
		},
		SignedOwner: func(_ *http.Request, owner string) (string, string, error) {
			kk, ok := keys[owner]
			if !ok {
				return "", "", errUnauthorized()
			}
			return kk[0], kk[1], nil
		},
	}
}

func errUnauthorized() error {
	return domain.ErrUnauthorized
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}

func protocolCall(h cloudhttp.Handler, path, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestTwoClientProtocolAndFailures(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	foreign := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	paidStore(t, pool, foreign)
	deviceA := readyDevice(t, store, owner)
	deviceB := secondDevice(t, pool, store, owner)
	foreignDevice := readyDevice(t, store, foreign)
	ctx := context.Background()
	_ = ctx
	owners := map[string]string{deviceA.FamilyID: owner.AccountID, deviceB.FamilyID: owner.AccountID, foreignDevice.FamilyID: foreign.AccountID}
	keys := map[string][2]string{owner.AccountID: {owner.ContributorID, owner.KeyID}, foreign.AccountID: {foreign.ContributorID, foreign.KeyID}}
	h := protocolHandler(t, store, owners, keys)
	push := func(family, device, batch string) *httptest.ResponseRecorder {
		return protocolCall(h, "/v1/cloud/sync/push", `{"family_id":"`+family+`","access_token":"synthetic","device_id":"`+device+`",`+batch+`}`)
	}
	pull := func(family, device, extra string) *httptest.ResponseRecorder {
		base := `{"family_id":"` + family + `","access_token":"synthetic","device_id":"` + device + `","limit":100`
		if extra != "" {
			base += `,` + extra
		}
		return protocolCall(h, "/v1/cloud/sync/pull", base+`}`)
	}
	v := vehicle(t)
	raw, _ := json.Marshal(v)
	batchCreate := `"epoch":1,"consent_revision":1,"commands":[{"operation_id":"` + newid(t) + `","dataset":"vehicles","id":"` + v.ID + `","expected_revision":0,"payload":` + string(raw) + `}]`
	w := push(deviceA.FamilyID, deviceA.DeviceID, batchCreate)
	if w.Code != 200 {
		t.Fatalf("A create %d %s", w.Code, w.Body.String())
	}
	w = pull(deviceA.FamilyID, deviceA.DeviceID, "")
	if w.Code != 200 {
		t.Fatalf("A initial pull %d", w.Code)
	}
	var first struct {
		Changes []app.Change `json:"changes"`
		Through int64        `json:"through"`
		Next    string       `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &first); err != nil || len(first.Changes) != 1 || first.Next == "" {
		t.Fatalf("A initial feed %v", err)
	}
	updated := v
	updated.Name = "Device B edit while A offline"
	rawUpdated, _ := json.Marshal(updated)
	batchEdit := `"epoch":1,"consent_revision":1,"commands":[{"operation_id":"` + newid(t) + `","dataset":"vehicles","id":"` + v.ID + `","expected_revision":1,"payload":` + string(rawUpdated) + `}]`
	w = push(deviceB.FamilyID, deviceB.DeviceID, batchEdit)
	if w.Code != 200 {
		t.Fatalf("B edit %d %s", w.Code, w.Body.String())
	}
	stale := v
	stale.Name = "Device A stale overwrite"
	rawStale, _ := json.Marshal(stale)
	batchStale := `"epoch":1,"consent_revision":1,"commands":[{"operation_id":"` + newid(t) + `","dataset":"vehicles","id":"` + v.ID + `","expected_revision":1,"payload":` + string(rawStale) + `}]`
	w = push(deviceA.FamilyID, deviceA.DeviceID, batchStale)
	if w.Code != 409 {
		t.Fatalf("stale arrival overwrote %d", w.Code)
	}
	var conflict struct {
		Conflicts []app.Conflict `json:"conflicts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &conflict); err != nil || len(conflict.Conflicts) != 1 || len(conflict.Conflicts[0].CurrentPayload) == 0 {
		t.Fatalf("conflict without base/current/proposed %v", err)
	}
	resolved := stale
	resolved.Name = "Device A resolved edit"
	rawResolved, _ := json.Marshal(resolved)
	batchResolved := `"epoch":1,"consent_revision":1,"commands":[{"operation_id":"` + newid(t) + `","dataset":"vehicles","id":"` + v.ID + `","expected_revision":2,"payload":` + string(rawResolved) + `}]`
	w = push(deviceA.FamilyID, deviceA.DeviceID, batchResolved)
	if w.Code != 200 {
		t.Fatalf("resolved push %d %s", w.Code, w.Body.String())
	}
	dupRaw, _ := json.Marshal(resolved)
	dupBatch := `"epoch":1,"consent_revision":1,"commands":[{"operation_id":"99999999-9999-4999-8999-999999999999","dataset":"vehicles","id":"` + v.ID + `","expected_revision":3,"payload":` + string(dupRaw) + `}]`
	dupBody := `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `",` + dupBatch + `}`
	w = protocolCall(h, "/v1/cloud/sync/push", dupBody)
	if w.Code != 200 {
		t.Fatalf("first duplicate %d", w.Code)
	}
	w = protocolCall(h, "/v1/cloud/sync/push", dupBody)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"replayed":true`) {
		t.Fatalf("lost response replay %d %s", w.Code, w.Body.String())
	}
	w = pull(deviceA.FamilyID, deviceA.DeviceID, `"cursor":"`+first.Next+`"`)
	if w.Code != 200 {
		t.Fatalf("pinned reconnect %d", w.Code)
	}
	w = pull(deviceA.FamilyID, deviceA.DeviceID, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"sequence":2`) {
		t.Fatalf("reconnect missed B edit %d %s", w.Code, w.Body.String())
	}
	w = pull(foreignDevice.FamilyID, foreignDevice.DeviceID, `"cursor":"`+first.Next+`"`)
	if w.Code != 400 && w.Code != 401 {
		t.Fatalf("account crossover leaked %d", w.Code)
	}
	h.Origin = "two.example.test"
	store.Origin = "two.example.test"
	w = pull(deviceA.FamilyID, deviceA.DeviceID, "")
	if w.Code != 401 && w.Code != 400 {
		t.Fatalf("origin crossover admitted %d", w.Code)
	}
	h.Origin = "one.example.test"
	store.Origin = "one.example.test"
	if err := store.RevokeDevice(ctx, deviceB, deviceB.DeviceID); err != nil {
		t.Fatal(err)
	}
	w = push(deviceB.FamilyID, deviceB.DeviceID, batchResolved)
	if w.Code != 401 {
		t.Fatalf("revoked dispatch admitted %d", w.Code)
	}
	store.Paid = nil
	w = push(deviceA.FamilyID, deviceA.DeviceID, batchResolved)
	if w.Code != 403 && w.Code != 409 {
		t.Fatalf("lapsed push admitted %d", w.Code)
	}
	w = pull(deviceA.FamilyID, deviceA.DeviceID, "")
	if w.Code != 403 {
		t.Fatalf("lapsed pull admitted %d", w.Code)
	}
}

func TestTwoClientResnapshotRestartAndBoundedStream(t *testing.T) {
	pool := freshDB(t)
	owner := seedOwner(t, pool)
	store := paidStore(t, pool, owner)
	deviceA := readyDevice(t, store, owner)
	owners := map[string]string{deviceA.FamilyID: owner.AccountID}
	keys := map[string][2]string{owner.AccountID: {owner.ContributorID, owner.KeyID}}
	h := protocolHandler(t, store, owners, keys)
	v := vehicle(t)
	pushBody := func(expected int64, name string) string {
		updated := v
		updated.Name = name
		body, _ := json.Marshal(updated)
		return `{"family_id":"` + deviceA.FamilyID + `","access_token":"synthetic","device_id":"` + deviceA.DeviceID + `","epoch":1,"consent_revision":1,"commands":[{"operation_id":"` + newid(t) + `","dataset":"vehicles","id":"` + v.ID + `","expected_revision":` + itoa(expected) + `,"payload":` + string(body) + `}]}`
	}
	w := protocolCall(h, "/v1/cloud/sync/push", pushBody(0, "Stream revision 1"))
	if w.Code != 200 {
		t.Fatalf("stream seed %d", w.Code)
	}
	for i := int64(1); i < 20; i++ {
		w = protocolCall(h, "/v1/cloud/sync/push", pushBody(i, "Stream revision"))
		if w.Code != 200 {
			t.Fatalf("stream %d %d", i, w.Code)
		}
	}
	w = protocolCall(h, "/v1/cloud/sync/pull", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","device_id":"`+deviceA.DeviceID+`","limit":100}`)
	if w.Code != 200 {
		t.Fatalf("bounded stream pull %d", w.Code)
	}
	var feed struct {
		Changes []app.Change `json:"changes"`
		Through int64        `json:"through"`
		Next    string       `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &feed); err != nil || len(feed.Changes) != 20 || feed.Through != 20 {
		t.Fatalf("bounded stream %d changes through %d %v", len(feed.Changes), feed.Through, err)
	}
	w = protocolCall(h, "/v1/cloud/sync/pull", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","device_id":"`+deviceA.DeviceID+`","limit":5}`)
	if w.Code != 200 {
		t.Fatalf("paged stream %d", w.Code)
	}
	var page struct {
		Changes []app.Change `json:"changes"`
		Through int64        `json:"through"`
		Next    string       `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || len(page.Changes) != 5 || page.Through != 20 || page.Next == "" {
		t.Fatalf("paged watermark %v %v", page, err)
	}
	old := time.Now().Add(-31 * 24 * time.Hour)
	if _, err := pool.Exec(context.Background(), "UPDATE cloud_changes SET created_at=$1 WHERE account_id=$2", old, owner.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE cloud_operations SET created_at=$1 WHERE account_id=$2", old, owner.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Compact(context.Background(), owner.AccountID, time.Now()); err != nil {
		t.Fatal(err)
	}
	restarted := NewStore(pool)
	restarted.Origin = "one.example.test"
	restarted.Paid = store.Paid
	restarted.Station = store.Station
	h2 := protocolHandler(t, restarted, owners, keys)
	h2.Origin = "one.example.test"
	w = protocolCall(h2, "/v1/cloud/sync/pull", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","device_id":"`+deviceA.DeviceID+`","limit":100,"cursor":"`+page.Next+`"}`)
	if w.Code != 409 {
		t.Fatalf("compacted cursor advanced %d", w.Code)
	}
	w = protocolCall(h2, "/v1/cloud/sync/snapshot", `{"family_id":"`+deviceA.FamilyID+`","access_token":"synthetic","device_id":"`+deviceA.DeviceID+`","dataset":"vehicles","limit":100}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), v.ID) {
		t.Fatalf("restart snapshot %d %s", w.Code, w.Body.String())
	}
}
