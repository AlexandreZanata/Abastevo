//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/profile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/jackc/pgx/v5"
)

func TestAPIHelperProcess(t *testing.T) {
	if os.Getenv("ANPFUEL_API_HELPER") != "1" {
		return
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
}

type apiHarness struct {
	t      *testing.T
	base   string
	client *http.Client
	conn   *pgx.Conn
}
type apiKey struct {
	priv                  *ecdsa.PrivateKey
	fp, x, y, contributor string
}
type apiChallenge struct {
	ID    string `json:"challenge_id"`
	Nonce string `json:"nonce"`
}

func (h apiHarness) call(req *http.Request, want int) map[string]any {
	h.t.Helper()
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatal(err)
	}
	if resp.StatusCode != want {
		h.t.Fatalf("%s %s: got %d want %d: %s", req.Method, req.URL.Path, resp.StatusCode, want, body)
	}
	if want >= 400 && resp.Header.Get("Cache-Control") != "no-store" {
		h.t.Error("error is cacheable")
	}
	var out map[string]any
	if len(body) > 0 && json.Unmarshal(body, &out) != nil {
		h.t.Fatalf("non-JSON response: %s", body)
	}
	return out
}
func (h apiHarness) req(method, path string, body []byte) *http.Request {
	h.t.Helper()
	req, err := http.NewRequest(method, h.base+path, bytes.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}
func (h apiHarness) challenge(fp, purpose string) apiChallenge {
	raw, _ := json.Marshal(map[string]string{"fingerprint": fp, "purpose": purpose})
	out := h.call(h.req("POST", "/v1/identity/challenges", raw), 201)
	return apiChallenge{out["challenge_id"].(string), out["nonce"].(string)}
}
func newAPIKey(t *testing.T) apiKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x := base64.RawURLEncoding.EncodeToString(k.X.FillBytes(make([]byte, 32)))
	y := base64.RawURLEncoding.EncodeToString(k.Y.FillBytes(make([]byte, 32)))
	fp, err := profile.Thumbprint(x, y)
	if err != nil {
		t.Fatal(err)
	}
	return apiKey{priv: k, fp: fp, x: x, y: y}
}
func signAPI(t *testing.T, key apiKey, lines []string) string {
	t.Helper()
	sum := sha512.Sum512([]byte(strings.Join(lines, "\n")))
	r, s, err := ecdsa.Sign(rand.Reader, key.priv, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...))
}
func publicAPIKey(k apiKey) map[string]string {
	return map[string]string{"kty": "EC", "crv": "P-256", "x": k.x, "y": k.y}
}
func (h apiHarness) enroll(k apiKey) apiKey {
	ch := h.challenge(k.fp, "REGISTER")
	req := h.req("POST", "/v1/contributors", nil)
	created, expires := fmt.Sprint(time.Now().Unix()-1), fmt.Sprint(time.Now().Unix()+240)
	lines := auth.BaseLines(req, "test.invalid", created, expires, k.fp, ch.Nonce, nil, false)
	raw, _ := json.Marshal(map[string]any{"public_jwk": publicAPIKey(k), "challenge_id": ch.ID, "proof": map[string]any{"lines": lines, "signature": signAPI(h.t, k, lines)}})
	out := h.call(h.req("POST", "/v1/contributors", raw), 201)
	k.contributor = out["contributor_id"].(string)
	return k
}
func (h apiHarness) signed(k apiKey, method, path string, body []byte) *http.Request {
	ch := h.challenge(k.fp, "SIGN")
	req := h.req(method, path, body)
	created, expires := fmt.Sprint(time.Now().Unix()-1), fmt.Sprint(time.Now().Unix()+240)
	lines := auth.BaseLines(req, "test.invalid", created, expires, k.fp, ch.Nonce, body, len(body) > 0)
	for name, value := range map[string]string{auth.HeaderSignature: signAPI(h.t, k, lines), auth.HeaderCreated: created, auth.HeaderExpires: expires, auth.HeaderKeyID: k.fp, auth.HeaderNonce: ch.Nonce} {
		req.Header.Set(name, value)
	}
	return req
}

func TestPublicProcessIdentityAndSignedWrites(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("ANPFUEL_TEST_DATABASE_URL must name disposable test infrastructure")
	}
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	dbName := fmt.Sprintf("api_validation_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + dbName
	testDSN := parsed.String()
	conn, err := pgx.Connect(ctx, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAPIHelperProcess$")
	for _, item := range os.Environ() {
		if !strings.HasPrefix(item, "ANPFUEL_") {
			cmd.Env = append(cmd.Env, item)
		}
	}
	storageEndpoint := os.Getenv("ANPFUEL_TEST_STORAGE_ENDPOINT")
	if storageEndpoint != "" {
		for _, name := range []string{"ENDPOINT", "BUCKET", "ACCESS_KEY_ID", "SECRET_ACCESS_KEY", "REGION"} {
			value := os.Getenv("ANPFUEL_TEST_STORAGE_" + name)
			if value == "" {
				t.Fatalf("missing test storage %s", name)
			}
			cmd.Env = append(cmd.Env, "ANPFUEL_R2_"+name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "ANPFUEL_ANP_DISCOVERY_ENABLED=false", "ANPFUEL_API_HELPER=1", "ANPFUEL_ENV=development", "ANPFUEL_DATABASE_URL="+testDSN, "ANPFUEL_HTTP_ADDR="+addr, "ANPFUEL_METRICS_ADDR=", "ANPFUEL_CANONICAL_HOST=test.invalid", "ANPFUEL_CURSOR_SECRET="+strings.Repeat("synthetic", 8))
	log, err := os.Create(filepath.Join(t.TempDir(), "api.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil {
			t.Errorf("API did not drain: %v", err)
		}
	})
	h := apiHarness{t: t, base: "http://" + addr, client: &http.Client{Timeout: 5 * time.Second}, conn: conn}
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := h.client.Get(h.base + "/health/live")
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("API did not start")
		}
		time.Sleep(25 * time.Millisecond)
	}
	h.call(h.req("GET", "/health/live", nil), 200)
	h.call(h.req("GET", "/health/ready", nil), 503)
	if _, err := migrate.Apply(ctx, testDSN, dbmigrations.Files); err != nil {
		t.Fatal(err)
	}
	h.call(h.req("GET", "/health/ready", nil), 200)
	station := "c0000000-0000-4000-8000-000000000001"
	if _, err := conn.Exec(ctx, "INSERT INTO directory_stations(id,display_name,municipality_code,state) VALUES ($1,'Validation Station','3550308','SP')", station); err != nil {
		t.Fatal(err)
	}
	a := h.enroll(newAPIKey(t))
	b := h.enroll(newAPIKey(t))
	again := h.enroll(a)
	if again.contributor != a.contributor {
		t.Fatal("duplicate registration changed identity")
	}
	// Every public read mounts in the actual process; nearby needs real coordinates.
	for _, path := range []string{"/v1/community/feed?state=SP&municipality_code=3550308&fuel_product=GASOLINE_REGULAR", "/v1/stations?q=Validation", "/v1/stations/" + station, "/v1/stations/" + station + "/prices?fuel_product=GASOLINE_REGULAR", "/v1/stations/" + station + "/official-prices", "/v1/stations/nearby?lat=-23.55&lon=-46.63&radius_m=1000"} {
		h.call(h.req("GET", path, nil), 200)
	}
	raw := []byte(fmt.Sprintf(`{"client_submission_id":"process-submit-1","station_id":%q,"fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5900,"currency":"BRL","unit":"L"},"condition":{"kind":"STANDARD"}}`, station))
	req := h.signed(a, "POST", "/v1/observations", raw)
	req.Header.Set("Idempotency-Key", "process-key-1")
	out := h.call(req, 201)
	id := out["id"].(string)
	retry := h.signed(a, "POST", "/v1/observations", raw)
	retry.Header.Set("Idempotency-Key", "process-key-1")
	if h.call(retry, 201)["id"] != id {
		t.Fatal("idempotent retry forked fact")
	}
	// A replayed nonce is denied even with a valid permanent client operation ID.
	replay := h.req("POST", "/v1/observations", raw)
	replay.Header = req.Header.Clone()
	h.call(replay, 401)
	conflict := h.signed(a, "POST", "/v1/observations", bytes.ReplaceAll(raw, []byte("5900"), []byte("6000")))
	conflict.Header.Set("Idempotency-Key", "process-key-1")
	h.call(conflict, 409)
	wrongUnit := h.signed(a, "POST", "/v1/observations", bytes.ReplaceAll(raw, []byte(`"unit":"L"`), []byte(`"unit":"KG"`)))
	wrongUnit.Header.Set("Idempotency-Key", "unit-key")
	h.call(wrongUnit, 400)
	h.call(h.signed(a, "GET", "/v1/observations/"+id, nil), 200)
	h.call(h.signed(b, "GET", "/v1/observations/"+id, nil), 404)
	h.call(h.signed(a, "GET", "/v1/contributors/me/observations", nil), 200)
	h.call(h.signed(a, "POST", "/v1/observations/"+id+"/confirmations", []byte(`{"client_submission_id":"self-vote"}`)), 409)
	// RECEIVED is not published/eligible; a second contributor cannot manufacture validation.
	h.call(h.signed(b, "POST", "/v1/observations/"+id+"/confirmations", []byte(`{"client_submission_id":"early-vote"}`)), 409)
	upload := []byte(`{"client_submission_id":"upload-1","content_type":"image/jpeg","size_bytes":1024,"sha256":"` + strings.Repeat("a", 64) + `"}`)
	if storageEndpoint == "" {
		h.call(h.signed(a, "POST", "/v1/uploads", upload), 503)
	} else {
		workerPath := filepath.Join(t.TempDir(), "worker")
		build := exec.Command("go", "build", "-race", "-o", workerPath, "../worker")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("worker build: %v: %s", err, out)
		}
		worker := exec.Command(workerPath)
		worker.Env = cmd.Env
		worker.Stdout, worker.Stderr = log, log
		if err := worker.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = worker.Process.Signal(syscall.SIGTERM)
			if err := worker.Wait(); err != nil {
				t.Errorf("worker did not drain: %v", err)
			}
		})
		waitSQL := func(query string, args ...any) {
			deadline := time.Now().Add(20 * time.Second)
			for {
				var ready bool
				if err := conn.QueryRow(ctx, query, args...).Scan(&ready); err != nil {
					t.Fatal(err)
				}
				if ready {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("worker timeout: %s", query)
				}
				time.Sleep(50 * time.Millisecond)
			}
		}
		waitSQL("SELECT EXISTS(SELECT 1 FROM community_observation_decisions WHERE observation_id=$1 AND to_state='VALIDATED')", id)
		h.call(h.signed(a, "POST", "/v1/observations/"+id+"/confirmations", []byte(`{"client_submission_id":"validated-self"}`)), 403)
		h.call(h.signed(b, "POST", "/v1/observations/"+id+"/confirmations", []byte(`{"client_submission_id":"independent-vote"}`)), 201)
		var img bytes.Buffer
		if err := jpeg.Encode(&img, image.NewRGBA(image.Rect(0, 0, 4, 4)), &jpeg.Options{Quality: 85}); err != nil {
			t.Fatal(err)
		}
		reserve := func(clientID string, contents []byte) (string, string) {
			sum := sha256.Sum256(contents)
			intent := []byte(fmt.Sprintf(`{"client_submission_id":%q,"content_type":"image/jpeg","size_bytes":%d,"sha256":"%x"}`, clientID, len(contents), sum))
			out := h.call(h.signed(a, "POST", "/v1/uploads", intent), 201)
			uploadURL := out["url"].(string)
			put, _ := http.NewRequest("PUT", uploadURL, bytes.NewReader(contents))
			put.Header.Set("Content-Type", "image/jpeg")
			resp, err := h.client.Do(put)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("presigned PUT: %d", resp.StatusCode)
			}
			// The bucket is private: removing the query proof cannot read it.
			public, _ := url.Parse(uploadURL)
			public.RawQuery = ""
			resp, err = h.client.Get(public.String())
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 403 {
				t.Fatalf("unsigned object read: %d", resp.StatusCode)
			}
			session := out["upload_id"].(string)
			h.call(h.signed(b, "GET", "/v1/uploads/"+session, nil), 404)
			h.call(h.signed(a, "POST", "/v1/uploads/"+session+"/complete", []byte(`{}`)), 202)
			return session, uploadURL
		}
		session, _ := reserve("valid-jpeg", img.Bytes())
		waitSQL("SELECT EXISTS(SELECT 1 FROM evidence_sessions WHERE id=$1 AND status='READY')", session)
		view := h.call(h.signed(a, "GET", "/v1/uploads/"+session, nil), 200)
		evidenceID, ok := view["evidence_id"].(string)
		if !ok {
			t.Fatal("missing verified evidence")
		}
		photoBody := bytes.ReplaceAll(raw, []byte(`"process-submit-1"`), []byte(`"photo-submit"`))
		photoBody = append(photoBody[:len(photoBody)-1], []byte(fmt.Sprintf(`,"evidence_id":%q}`, evidenceID))...)
		photoReq := h.signed(a, "POST", "/v1/observations", photoBody)
		photoReq.Header.Set("Idempotency-Key", "photo-operation")
		photoID := h.call(photoReq, 201)["id"].(string)
		waitSQL("SELECT EXISTS(SELECT 1 FROM community_observation_decisions WHERE observation_id=$1 AND to_state='VALIDATED')", photoID)
		h.call(h.signed(b, "POST", "/v1/observations/"+photoID+"/disputes", []byte(`{"client_submission_id":"process-dispute","reason":"PRICE_CHANGED"}`)), 201)
		badSession, _ := reserve("malformed-jpeg", []byte("not a JPEG"))
		waitSQL("SELECT EXISTS(SELECT 1 FROM evidence_sessions WHERE id=$1 AND status='REJECTED')", badSession)
		badView := h.call(h.signed(a, "GET", "/v1/uploads/"+badSession, nil), 200)
		if badView["evidence_id"] != nil {
			t.Fatal("malformed bytes granted evidence")
		}
	}
	h.call(h.signed(b, "GET", "/v1/uploads/00000000-0000-4000-8000-000000000000", nil), 404)
	h.call(h.signed(b, "POST", "/v1/uploads/00000000-0000-4000-8000-000000000000/complete", []byte(`{}`)), 404)
	t.Run("rotation-binds-both-keys", func(t *testing.T) {
		fresh := newAPIKey(t)
		oldCH, newCH := h.challenge(a.fp, "SIGN"), h.challenge(fresh.fp, "SIGN")
		created, expires := fmt.Sprint(time.Now().Unix()-1), fmt.Sprint(time.Now().Unix()+240)
		jwk := publicAPIKey(fresh)
		// Field order is the documented canonical rotation-intent order.
		intent := []byte(fmt.Sprintf(`{"new_jwk":{"kty":"EC","crv":"P-256","x":%q,"y":%q},"old_challenge_id":%q,"new_challenge_id":%q}`, fresh.x, fresh.y, oldCH.ID, newCH.ID))
		req := h.req("POST", "/v1/contributors/me/keys/rotate", []byte(`{}`))
		oldLines := auth.BaseLines(req, "test.invalid", created, expires, a.fp, oldCH.Nonce, intent, true)
		newLines := auth.BaseLines(req, "test.invalid", created, expires, fresh.fp, newCH.Nonce, intent, true)
		payload := map[string]any{"new_jwk": jwk, "old": map[string]any{"challenge_id": oldCH.ID, "lines": oldLines, "signature": signAPI(t, a, oldLines)}, "new": map[string]any{"challenge_id": newCH.ID, "lines": newLines, "signature": signAPI(t, fresh, newLines)}}
		substituted := newAPIKey(t)
		payload["new_jwk"] = publicAPIKey(substituted)
		bad, _ := json.Marshal(payload)
		h.call(h.req("POST", req.URL.Path, bad), 401)
		payload["new_jwk"] = jwk
		valid, _ := json.Marshal(payload)
		rot := h.call(h.req("POST", req.URL.Path, valid), 200)
		if rot["contributor_id"] != a.contributor {
			t.Fatal("rotation changed contributor")
		}
		h.call(h.signed(a, "GET", "/v1/contributors/me/observations", nil), 401)
		h.call(h.signed(fresh, "GET", "/v1/contributors/me/observations", nil), 200)
	})
}
