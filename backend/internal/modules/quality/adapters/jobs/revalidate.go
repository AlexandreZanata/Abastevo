package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// RevalidationKind is reserved from general claims and scheduled separately.
const RevalidationKind = "quality-revalidate"

// Revalidate sweeps published station links and compares them with live
// Directory evidence. A divergence (binding retired/moved, station not
// active) enqueues one bounded
// fresh import for the affected edition. The fresh attempt re-resolves
// identity and publishes a correction revision through the existing atomic
// publisher; superseded revisions remain frozen and reachable, and no
// business validity window is inferred. A durable cursor advances whole
// editions, so repeated sweeps cover every published association without
// source I/O here or repeated archive fetches on every tick.
type Revalidate struct {
	Pool        *pgxpool.Pool
	Evidence    func(ctx context.Context, cnpjs []string) ([]domain.IdentityEvidence, error)
	Enqueue     func(ctx context.Context, payload ImportPayload, dedupe string) (platformjobs.EnqueueResult, error)
	Now         func() time.Time
	Batch       int
	MaxEditions int
}

func (Revalidate) Kind() string { return RevalidationKind }

// Version implements jobs.Handler.
func (Revalidate) Version() int { return 1 }

func (r Revalidate) batch() int {
	if r.Batch <= 0 {
		return 16
	}
	return r.Batch
}

func (r Revalidate) maxEditions() int {
	if r.MaxEditions <= 0 {
		return 4
	}
	return r.MaxEditions
}

func revalidateError(code string) error { return errors.New("quality revalidate: " + code) }

func uuidText(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

// divergence returns the canonical fingerprint of the active associations
// that no longer match live Directory evidence, or false when every active
// association is still exactly the bound station. Only applied
// associations need invalidation: a record that was unbound and later
// becomes bindable again cannot be republished because its manifest is
// frozen under the superseded revision's publication key (FQ-04c), and it
// is never a false claim about a station (the conservative failure mode is
// a missing association, not a wrong one).
func divergence(rows []quality.ListRevisionLinkRowsRow, evidence map[string][]domain.IdentityEvidence, decidedAt string) (string, bool) {
	expectedByCNPJ := map[string]string{}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.Active {
			continue
		}
		expected, known := expectedByCNPJ[row.SourceCnpj]
		if !known {
			resolved := domain.ResolveCurrentIdentity(domain.SourceRecord{
				RecordKey: row.RecordKey, CNPJ: row.SourceCnpj, ObservedAt: row.ObservedOn.Time.Format("2006-01-02"),
			}, decidedAt, evidence[row.SourceCnpj], false)
			if resolved.Kind == domain.ResolutionBound {
				expected = resolved.Link.StationID
			}
			expectedByCNPJ[row.SourceCnpj] = expected
		}
		stored := uuidText(row.StationID)
		if expected != "" && stored == expected {
			continue
		}
		lines = append(lines, strings.Join([]string{row.RecordKey, row.SourceCnpj, stored, strconv.FormatBool(row.Active), expected}, "\x1f"))
	}
	if len(lines) == 0 {
		return "", false
	}
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:]), true
}

func evidenceIndex(rows []domain.IdentityEvidence) map[string][]domain.IdentityEvidence {
	out := make(map[string][]domain.IdentityEvidence, len(rows))
	for _, row := range rows {
		out[row.CNPJ] = append(out[row.CNPJ], row)
	}
	return out
}

func distinctCNPJs(rows []quality.ListRevisionLinkRowsRow) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		if !seen[row.SourceCnpj] {
			seen[row.SourceCnpj] = true
			out = append(out, row.SourceCnpj)
		}
	}
	sort.Strings(out)
	return out
}

// Handle runs one bounded sweep: read the durable cursor, fetch at most one
// batch of editions, judge each complete edition against live evidence,
// enqueue refreshes within budget, then advance or wrap the cursor.
func (r Revalidate) Handle(ctx context.Context, _ platformjobs.Job) error {
	if r.Pool == nil || r.Evidence == nil || r.Enqueue == nil {
		return revalidateError("ports unavailable")
	}
	now := r.Now
	if now == nil {
		now = time.Now
	}
	q := quality.New(r.Pool)
	params := quality.ListPublishedLinkEditionsParams{RowLimit: int32(r.batch() + 1)}
	if cursor, err := q.GetRevalidationCursor(ctx, RevalidationKind); err == nil {
		params.CursorEdition = cursor.CursorEdition
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	editions, err := q.ListPublishedLinkEditions(ctx, params)
	if err != nil {
		return err
	}
	atEnd := len(editions) <= r.batch()
	if !atEnd {
		editions = editions[:r.batch()]
	}
	decidedAt := now().UTC().Format("2006-01-02")

	enqueued := 0
	processed := params.CursorEdition
	if len(editions) == 0 {
		// Nothing (left) to check: wrap so the next sweep starts over.
		processed = pgtype.UUID{}
	}
	for i, edition := range editions {
		rows, lerr := q.ListRevisionLinkRows(ctx, edition.RevisionID)
		if lerr != nil {
			return lerr
		}
		evidence, eerr := r.Evidence(ctx, distinctCNPJs(rows))
		if eerr != nil {
			return revalidateError("directory evidence unavailable")
		}
		fingerprint, divergent := divergence(rows, evidenceIndex(evidence), decidedAt)
		if divergent {
			if enqueued >= r.maxEditions() {
				break
			}
			if err = r.refresh(ctx, q, edition, fingerprint); err != nil {
				return err
			}
			enqueued++
		}
		processed = edition.EditionID
		if i == len(editions)-1 && atEnd {
			// Whole stream covered; wrap for the next sweep.
			processed = pgtype.UUID{}
		}
	}
	return q.SetRevalidationCursor(ctx, quality.SetRevalidationCursorParams{
		Kind: RevalidationKind, CursorEdition: processed,
	})
}

// refresh enqueues the bounded fresh import for one divergent edition from
// its frozen revision resource. Exactly one member-backed resource must
// exist; anything else refuses the refresh instead of guessing.
func (r Revalidate) refresh(ctx context.Context, q *quality.Queries, edition quality.ListPublishedLinkEditionsRow, fingerprint string) error {
	resources, err := q.ReadRevisionResources(ctx, edition.RevisionID)
	if err != nil {
		return err
	}
	if len(resources) != 1 || strings.TrimSpace(resources[0].MemberName) == "" {
		return revalidateError("frozen revision resource unavailable")
	}
	res := resources[0]
	dedupe := strings.Join([]string{"quality-revalidate", edition.Source, edition.Stream, edition.Edition, uuidText(edition.RevisionID), fingerprint}, "\x1f")
	_, err = r.Enqueue(ctx, ImportPayload{
		Version: 1, Source: edition.Source, Stream: edition.Stream, Edition: edition.Edition,
		URL: res.Url, SHA256: res.Sha256, Member: res.MemberName,
	}, dedupe)
	if err != nil {
		return err
	}
	// An exhausted job stays dead. Only explicit operator replay may renew
	// its budget; repeated automatic sweeps must never bypass that ceiling.
	return nil
}
