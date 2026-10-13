// Command staging-retention seeds bounded synthetic transport-only fixtures in
// the owned staging store, or observes them read-only. It never runs cleanup,
// backdates clocks, publishes prices or creates training derivatives.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	adapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const prefix = "rva07-staging-20261012-"

var phase = "startup"

type copyRef struct {
	Session, Original, Final, Kind string
	CreatedAt                      time.Time
}
type manifest struct {
	Purpose  string
	SeededAt time.Time
	Copies   []copyRef
}
type fixture struct {
	pool   *pgxpool.Pool
	store  *adapters.Store
	cfg    config.Config
	client *http.Client
}

func main() {
	mode := flag.String("mode", "inspect", "seed or inspect (manifest on stdin)")
	local := flag.Bool("pod-local-storage", false, "use the same pod private S3 listener; does not test public upload limits")
	allow := flag.Bool("allow-staging-fixtures", false, "explicitly authorize five synthetic staging sessions")
	flag.Parse()
	if (*mode != "seed" && *mode != "inspect") || flag.NArg() != 0 || (*mode == "seed" && !*allow) {
		fmt.Fprintln(os.Stderr, "invalid mode or missing explicit fixture authorization")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	err := run(ctx, *mode, *local)
	if err != nil {
		fmt.Fprintf(os.Stderr, "staging retention: stage=%s class=%s (private details suppressed)\n", phase, safeFailure(err))
		os.Exit(1)
	}
}
func run(ctx context.Context, mode string, local bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Env != config.EnvStaging || cfg.CanonicalHost != "teste.abastevo.com.br" || cfg.R2 == nil || cfg.R2.Endpoint != "https://teste.abastevo.com.br" || cfg.R2.Bucket != "abastevo-dev-photos" {
		return errors.New("owned staging identity required")
	}
	if local {
		cfg.R2.Endpoint = "http://127.0.0.1:9000"
	}
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return err
	}
	pc.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return err
	}
	defer pool.Close()
	f := fixture{pool, adapters.NewStore(pool), cfg, &http.Client{Timeout: 5 * time.Second}}
	if mode == "seed" {
		m, err := f.seed(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(m)
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, 8193))
	if err != nil || len(raw) > 8192 {
		return errors.New("invalid manifest bound")
	}
	var m manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if err = validate(m); err != nil {
		return err
	}
	return f.inspect(ctx, m)
}
func validate(m manifest) error {
	if m.Purpose != prefix || len(m.Copies) != 5 || m.SeededAt.IsZero() {
		return errors.New("invalid fixture manifest")
	}
	seen := map[string]bool{}
	canaries := 0
	for _, c := range m.Copies {
		if len(c.Session) != 36 || !strings.HasPrefix(c.Original, "q/"+prefix) || !strings.HasPrefix(c.Final, "f/") || len(c.Final) != 66 || c.CreatedAt.IsZero() || c.CreatedAt.Before(m.SeededAt) || c.CreatedAt.After(m.SeededAt.Add(time.Minute)) || seen[c.Session] {
			return errors.New("invalid copy binding")
		}
		if c.Kind == "elapsed" {
			canaries++
		} else if c.Kind != "abandoned" {
			return errors.New("invalid case")
		}
		seen[c.Session] = true
	}
	if canaries != 1 || m.Copies[0].Kind != "elapsed" {
		return errors.New("exactly one elapsed canary required")
	}
	return nil
}
func syntheticJPEG(i int) ([]byte, error) {
	im := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			im.Set(x, y, color.RGBA{R: uint8(20 + i*30), A: 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, nil); err != nil {
		return nil, err
	}
	// Trailing zero padding exercises the maximum transfer size. These are
	// transport fixtures, not minimized content or OCR/photographic evidence.
	raw := make([]byte, domain.MaxUploadBytes)
	copy(raw, b.Bytes())
	return raw, nil
}
func (f fixture) input(key string) storage.PresignInput {
	ns := "f/"
	if strings.HasPrefix(key, "q/") {
		ns = "q/"
	}
	return storage.PresignInput{Endpoint: f.cfg.R2.Endpoint, Bucket: f.cfg.R2.Bucket, Key: key, Namespace: ns, ContentType: domain.AllowedMIME, MaxBytes: domain.MaxUploadBytes, TTL: time.Minute}
}
func (f fixture) credentials() storage.Credentials {
	return storage.Credentials{AccessKeyID: f.cfg.R2.AccessKeyID, SecretAccessKey: f.cfg.R2.SecretAccessKey}
}
func (f fixture) put(ctx context.Context, key string, body []byte) error {
	signed, err := storage.PresignPUT(f.input(key), f.credentials())
	if err != nil {
		return err
	}
	return storage.Put(ctx, f.client, signed.URL, domain.AllowedMIME, body)
}
func (f fixture) exists(ctx context.Context, key string) (bool, error) {
	signed, err := storage.PresignHEAD(f.input(key), f.credentials())
	if err != nil {
		return false, err
	}
	return storage.Exists(ctx, f.client, signed.URL)
}
func (f fixture) seed(ctx context.Context) (manifest, error) {
	m := manifest{Purpose: prefix, SeededAt: time.Now().UTC()}
	runID, err := jobs.NewUUIDv4()
	if err != nil {
		return m, err
	}
	for i := 0; i < 5; i++ {
		id, err := jobs.NewUUIDv4()
		if err != nil {
			return m, err
		}
		raw, err := syntheticJPEG(i)
		if err != nil {
			return m, err
		}
		copy(raw[len(raw)-len(runID):], runID)
		sum := sha256.Sum256(raw)
		hash := hex.EncodeToString(sum[:])
		now := time.Now().UTC()
		key := "q/" + prefix + runID + "/" + id
		sess, _, err := domain.NewSession(domain.Params{ID: id, ContributorRef: prefix + runID, ClientSessionID: prefix + id, MIME: domain.AllowedMIME, DeclaredBytes: int64(len(raw)), ClaimedSHA256: hash, QuarantineKey: key, CreatedAt: now, PolicyVersion: domain.PolicyV1})
		if err != nil {
			return m, err
		}
		// Explicit internal fixture path; this bypasses photo intake, never
		// certifies the public admission budget, and cannot create contributions.
		sess, _, err = f.store.ReserveSession(ctx, sess, domain.ReserveBudget{Now: now, DayStart: now.Truncate(24 * time.Hour), MaxCount: 10, MaxBytes: 30 << 20})
		if err != nil {
			return m, err
		}
		phase = "original-transfer"
		if err = f.put(ctx, key, raw); err != nil {
			return m, err
		}
		if err = f.store.CompleteSession(ctx, id, func(context.Context, pgx.Tx, string, []byte, string) error { return nil }); err != nil {
			return m, err
		}
		nonce, err := jobs.NewUUIDv4()
		if err != nil {
			return m, err
		}
		final := "f/" + hash
		authority, err := f.store.BeginVerificationWrite(ctx, id, final, hash, nonce)
		if err != nil {
			return m, err
		}
		phase = "final-transfer"
		if err = f.put(ctx, final, raw); err != nil {
			return m, err
		}
		kind := "abandoned"
		if i == 0 {
			oid, err := jobs.NewUUIDv4()
			if err != nil {
				return m, err
			}
			if _, err = f.store.RecordVerifiedGuarded(ctx, id, adapters.ObjectData{ID: oid, FinalKey: final, SourceSHA256: hash, SanitizedSHA256: hash, Width: 16, Height: 16}, authority); err != nil {
				return m, err
			}
			kind = "elapsed"
		} else if err = f.store.RecordRejected(ctx, id, []string{"synthetic-abandoned-writer"}); err != nil {
			return m, err
		}
		m.Copies = append(m.Copies, copyRef{id, key, final, kind, sess.CreatedAt})
	}
	return m, validate(m)
}
func (f fixture) inspect(ctx context.Context, m manifest) error {
	summary := map[string]any{"checked_at": time.Now().UTC().Format(time.RFC3339), "seeded_at": m.SeededAt.Format(time.RFC3339), "original_present": 0, "final_present": 0, "abandoned_copies": 4, "autonomous_cleanup": true, "backdated": false}
	originals, finals, abandonedPurged := 0, 0, 0
	var elapsedDeleted *time.Time
	var latestOriginal *time.Time
	for _, c := range m.Copies {
		var actualKey, clientKey, state string
		var created, deadline time.Time
		var originalDeleted, finalDeleted *time.Time
		err := f.pool.QueryRow(ctx, `SELECT s.quarantine_key,s.client_session_id,s.created_at,s.quarantine_deleted_at,o.state,o.deadline_at,o.purged_at FROM evidence_sessions s JOIN evidence_final_outputs o ON o.session_id=s.id AND o.final_key=$2 WHERE s.id=$1::uuid`, c.Session, c.Final).Scan(&actualKey, &clientKey, &created, &originalDeleted, &state, &deadline, &finalDeleted)
		if err != nil {
			return err
		}
		if actualKey != c.Original || clientKey != prefix+c.Session || created.Sub(c.CreatedAt).Abs() > time.Microsecond || deadline.Sub(created.Add(24*time.Hour)).Abs() > time.Microsecond {
			return errors.New("fixture receipt mismatch")
		}
		present, err := f.exists(ctx, c.Original)
		if err != nil {
			return err
		}
		if present {
			originals++
		}
		finalPresent, err := f.exists(ctx, c.Final)
		if err != nil {
			return err
		}
		if finalPresent {
			finals++
		}
		if originalDeleted != nil && present {
			return errors.New("original receipt contradicts provider")
		}
		if state == "PURGED" && (finalDeleted == nil || finalPresent) {
			return errors.New("final receipt contradicts provider")
		}
		if originalDeleted != nil && (latestOriginal == nil || originalDeleted.After(*latestOriginal)) {
			latestOriginal = originalDeleted
		}
		if c.Kind == "abandoned" && !present && !finalPresent && originalDeleted != nil && state == "PURGED" {
			abandonedPurged++
		}
		if c.Kind == "elapsed" {
			summary["deadline_at"] = deadline.UTC().Format(time.RFC3339)
			summary["cleanup_due_at"] = deadline.Add(-2 * time.Hour).UTC().Format(time.RFC3339)
			elapsedDeleted = finalDeleted
			summary["elapsed_final_present"] = finalPresent
			if finalDeleted != nil {
				summary["elapsed_final_deleted_at"] = finalDeleted.UTC().Format(time.RFC3339)
				summary["elapsed_final_within_deadline"] = !finalDeleted.After(deadline)
			}
		}
	}
	summary["original_present"], summary["final_present"], summary["abandoned_purged"] = originals, finals, abandonedPurged
	if latestOriginal != nil {
		summary["latest_original_deleted_at"] = latestOriginal.UTC().Format(time.RFC3339)
	}
	summary["elapsed_retention_proof_complete"] = elapsedDeleted != nil && originals == 0 && finals == 0 && abandonedPurged == 4 && !elapsedDeleted.After(m.Copies[0].CreatedAt.Add(24*time.Hour)) && !time.Now().Before(m.Copies[0].CreatedAt.Add(24*time.Hour))
	return json.NewEncoder(os.Stdout).Encode(summary)
}

// Only types/status codes are diagnostic. Error strings may carry signed URLs.
func safeFailure(err error) string {
	var status *storage.StatusError
	if errors.As(err, &status) {
		return fmt.Sprintf("HTTP_%d", status.Status)
	}
	var remote *url.Error
	if errors.As(err, &remote) {
		return fmt.Sprintf("transport_%T", remote.Err)
	}
	return fmt.Sprintf("%T", err)
}
