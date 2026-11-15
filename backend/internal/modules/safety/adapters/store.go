// Package adapters persists community safety (B-BR-SAFE-01…09) in
// PostgreSQL: private blocks and user reports owned by the safety module.
// Content authors come from explicit author sources wired by the
// composition root; this package never reads forum or feedback tables.
package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	safetysql "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/safety"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
)

// TargetCommunityAccount is the moderation case target for user reports.
const TargetCommunityAccount = "COMMUNITY_ACCOUNT"

// AuthorSource resolves authors for one content kind.
type AuthorSource struct {
	// PublicAuthorTx returns the author of content that public reads serve
	// now; ok=false for missing, pending, hidden or removed content.
	PublicAuthorTx func(ctx context.Context, tx pgx.Tx, id string) (accountID, alias string, ok bool, err error)
	// Authors maps listed ids to author account ids, whatever their state.
	Authors func(ctx context.Context, ids []string) (map[string]string, error)
}

// CaseOpener opens or joins the open moderation case for a target inside
// the caller's transaction.
type CaseOpener func(ctx context.Context, tx pgx.Tx, targetType, targetID, reason, detail string) (caseID string, joined bool, err error)

// BlockQuota consumes one new-block admission for the account inside the
// transaction; it returns a *domain.QuotaError when the window is spent.
type BlockQuota func(ctx context.Context, tx pgx.Tx, accountID string) error

// Store implements application.Store.
type Store struct {
	pool       *pgxpool.Pool
	Sources    map[domain.ContentKind]AuthorSource
	OpenCase   CaseOpener
	BlockQuota BlockQuota
	Now        func() time.Time
}

var _ application.Store = (*Store)(nil)

// NewStore wires the owned queries to a pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, Now: time.Now}
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

// resolvePublicTx derives the author of public content and applies the
// target rules (B-BR-SAFE-02).
func (s *Store) resolvePublicTx(ctx context.Context, tx pgx.Tx, actor string, ref domain.ContentRef) (domain.Author, error) {
	src, ok := s.Sources[ref.Kind]
	if !ok || src.PublicAuthorTx == nil {
		return domain.Author{}, fmt.Errorf("safety: no author source for %s", ref.Kind)
	}
	accountID, alias, found, err := src.PublicAuthorTx(ctx, tx, ref.ID)
	if err != nil {
		return domain.Author{}, err
	}
	if !found {
		return domain.Author{}, domain.ErrNotFound
	}
	author := domain.Author{AccountID: strings.ToLower(accountID), Label: domain.BlockLabel(alias)}
	if err := domain.CheckTarget(actor, author); err != nil {
		return domain.Author{}, err
	}
	return author, nil
}

func (s *Store) begin(ctx context.Context, guard application.Guard) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("safety: begin: %w", err)
	}
	if guard != nil {
		if err := guard(ctx, tx); err != nil {
			_ = tx.Rollback(ctx)
			return nil, err
		}
	}
	return tx, nil
}

// Block stores one block of the author of a public content reference,
// converging on the existing block of the same pair (B-BR-SAFE-05).
func (s *Store) Block(ctx context.Context, cmd application.BlockCommand) (domain.BlockResult, error) {
	blocker, err := toUUID(cmd.AccountID)
	if err != nil {
		return domain.BlockResult{}, domain.ErrInvalid
	}
	tx, err := s.begin(ctx, cmd.Guard)
	if err != nil {
		return domain.BlockResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	author, err := s.resolvePublicTx(ctx, tx, cmd.AccountID, cmd.Ref)
	if err != nil {
		return domain.BlockResult{}, err
	}
	blocked, err := toUUID(author.AccountID)
	if err != nil {
		return domain.BlockResult{}, fmt.Errorf("safety: author id: %w", err)
	}
	q := safetysql.New(tx)
	if err := q.LockSafetySubject(ctx, "safety-block:"+cmd.AccountID); err != nil {
		return domain.BlockResult{}, fmt.Errorf("safety: lock blocker: %w", err)
	}
	existing, err := q.GetBlockByPair(ctx, safetysql.GetBlockByPairParams{BlockerAccountID: blocker, BlockedAccountID: blocked})
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.BlockResult{}, fmt.Errorf("safety: commit replay: %w", err)
		}
		return domain.BlockResult{Block: toBlock(existing.ID, existing.Label, existing.SourceKind, existing.CreatedAt), Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.BlockResult{}, fmt.Errorf("safety: get block: %w", err)
	}
	active, err := q.CountActiveBlocks(ctx, blocker)
	if err != nil {
		return domain.BlockResult{}, fmt.Errorf("safety: count blocks: %w", err)
	}
	if err := domain.AdmitBlock(int(active)); err != nil {
		return domain.BlockResult{}, err
	}
	if s.BlockQuota == nil {
		return domain.BlockResult{}, errors.New("safety: block quota unconfigured")
	}
	if err := s.BlockQuota(ctx, tx, cmd.AccountID); err != nil {
		return domain.BlockResult{}, err
	}
	id, err := newUUID()
	if err != nil {
		return domain.BlockResult{}, err
	}
	row, err := q.InsertBlock(ctx, safetysql.InsertBlockParams{
		ID: id, BlockerAccountID: blocker, BlockedAccountID: blocked,
		Label: author.Label, SourceKind: string(cmd.Ref.Kind),
		CreatedAt: pgtype.Timestamptz{Time: s.now(), Valid: true},
	})
	if err != nil {
		return domain.BlockResult{}, fmt.Errorf("safety: insert block: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.BlockResult{}, fmt.Errorf("safety: commit block: %w", err)
	}
	return domain.BlockResult{Block: toBlock(row.ID, row.Label, row.SourceKind, row.CreatedAt)}, nil
}

// Unblock removes one of the owner's blocks; foreign and unknown ids
// refuse alike (B-BR-SAFE-07).
func (s *Store) Unblock(ctx context.Context, cmd application.UnblockCommand) error {
	owner, err := toUUID(cmd.AccountID)
	if err != nil {
		return domain.ErrInvalid
	}
	id, err := toUUID(cmd.BlockID)
	if err != nil {
		return domain.ErrNotFound
	}
	tx, err := s.begin(ctx, cmd.Guard)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n, err := safetysql.New(tx).DeleteBlock(ctx, safetysql.DeleteBlockParams{ID: id, BlockerAccountID: owner})
	if err != nil {
		return fmt.Errorf("safety: delete block: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("safety: commit unblock: %w", err)
	}
	return nil
}

// ListBlocks returns the owner's blocks newest first (B-BR-SAFE-07).
func (s *Store) ListBlocks(ctx context.Context, accountID string) ([]domain.Block, error) {
	owner, err := toUUID(accountID)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	rows, err := safetysql.New(s.pool).ListBlocks(ctx, safetysql.ListBlocksParams{BlockerAccountID: owner, RowLimit: domain.MaxActiveBlocks})
	if err != nil {
		return nil, fmt.Errorf("safety: list blocks: %w", err)
	}
	out := make([]domain.Block, 0, len(rows))
	for _, r := range rows {
		out = append(out, toBlock(r.ID, r.Label, r.SourceKind, r.CreatedAt))
	}
	return out, nil
}

// Hidden returns the references written by accounts the owner blocked
// (B-BR-SAFE-06). Without blocks no author is resolved at all.
func (s *Store) Hidden(ctx context.Context, accountID string, refs []domain.ContentRef) ([]domain.ContentRef, error) {
	owner, err := toUUID(accountID)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	ids, err := safetysql.New(s.pool).ListBlockedAccounts(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("safety: blocked accounts: %w", err)
	}
	if len(ids) == 0 {
		return []domain.ContentRef{}, nil
	}
	blocked := make(map[string]bool, len(ids))
	for _, id := range ids {
		blocked[uuidString(id)] = true
	}
	byKind := make(map[domain.ContentKind][]string)
	for _, ref := range refs {
		byKind[ref.Kind] = append(byKind[ref.Kind], ref.ID)
	}
	authors := make(map[domain.ContentRef]string, len(refs))
	for kind, list := range byKind {
		src, ok := s.Sources[kind]
		if !ok || src.Authors == nil {
			return nil, fmt.Errorf("safety: no author source for %s", kind)
		}
		found, err := src.Authors(ctx, list)
		if err != nil {
			return nil, err
		}
		for id, author := range found {
			authors[domain.ContentRef{Kind: kind, ID: strings.ToLower(id)}] = author
		}
	}
	return domain.HiddenSubset(refs, authors, blocked), nil
}

// ReportUser files one report about the author of a public content
// reference and opens or joins its moderation case (B-BR-SAFE-04).
func (s *Store) ReportUser(ctx context.Context, cmd application.ReportCommand) (domain.ReportReceipt, error) {
	reporter, err := toUUID(cmd.AccountID)
	if err != nil {
		return domain.ReportReceipt{}, domain.ErrInvalid
	}
	source, err := toUUID(cmd.Ref.ID)
	if err != nil {
		return domain.ReportReceipt{}, domain.ErrInvalid
	}
	tx, err := s.begin(ctx, cmd.Guard)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	author, err := s.resolvePublicTx(ctx, tx, cmd.AccountID, cmd.Ref)
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	reported, err := toUUID(author.AccountID)
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: author id: %w", err)
	}
	q := safetysql.New(tx)
	if err := q.LockSafetySubject(ctx, "safety-report:"+cmd.AccountID); err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: lock reporter: %w", err)
	}
	existing, err := q.GetReportByPair(ctx, safetysql.GetReportByPairParams{ReporterAccountID: reporter, ReportedAccountID: reported})
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return domain.ReportReceipt{}, fmt.Errorf("safety: commit replay: %w", err)
		}
		return domain.ReportReceipt{ID: uuidString(existing.ID), Reason: domain.ReportReason(existing.Reason), CreatedAt: existing.CreatedAt.Time.UTC(), Replayed: true}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.ReportReceipt{}, fmt.Errorf("safety: get report: %w", err)
	}
	now := s.now()
	recent, err := q.CountReportsSince(ctx, safetysql.CountReportsSinceParams{
		ReporterAccountID: reporter,
		Since:             pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: count reports: %w", err)
	}
	if err := domain.AdmitReport(int(recent)); err != nil {
		return domain.ReportReceipt{}, err
	}
	if s.OpenCase == nil {
		return domain.ReportReceipt{}, errors.New("safety: case opener unconfigured")
	}
	id, err := newUUID()
	if err != nil {
		return domain.ReportReceipt{}, err
	}
	row, err := q.InsertReport(ctx, safetysql.InsertReportParams{
		ID: id, ReporterAccountID: reporter, ReportedAccountID: reported,
		SourceKind: string(cmd.Ref.Kind), SourceID: source,
		Reason: string(cmd.Reason), Detail: cmd.Detail,
		CreatedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: insert report: %w", err)
	}
	caseID, _, err := s.OpenCase(ctx, tx, TargetCommunityAccount, author.AccountID, string(cmd.Reason), domain.CaseDetail(cmd.Ref))
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: open case: %w", err)
	}
	caseUUID, err := toUUID(caseID)
	if err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: case id: %w", err)
	}
	if err := q.SetReportCase(ctx, safetysql.SetReportCaseParams{ID: row.ID, ModerationCaseID: caseUUID}); err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: link case: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ReportReceipt{}, fmt.Errorf("safety: commit report: %w", err)
	}
	return domain.ReportReceipt{ID: uuidString(row.ID), Reason: domain.ReportReason(row.Reason), CreatedAt: row.CreatedAt.Time.UTC()}, nil
}

// EraseAccountBatch deletes at most limit blocks made or received by the
// account and scrubs at most limit report details it filed
// (B-BR-SAFE-08). done reports that nothing is left to erase.
func (s *Store) EraseAccountBatch(ctx context.Context, accountID string, limit int) (bool, error) {
	if limit <= 0 {
		limit = 100
	}
	id, err := toUUID(accountID)
	if err != nil {
		return false, domain.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("safety: begin erasure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := safetysql.New(tx)
	if _, err := q.DeleteBlocksForErasure(ctx, safetysql.DeleteBlocksForErasureParams{AccountID: id, RowLimit: int32(limit)}); err != nil {
		return false, fmt.Errorf("safety: erase blocks: %w", err)
	}
	if _, err := q.ScrubReportDetailForErasure(ctx, safetysql.ScrubReportDetailForErasureParams{AccountID: id, RowLimit: int32(limit)}); err != nil {
		return false, fmt.Errorf("safety: scrub reports: %w", err)
	}
	pending, err := q.HasPendingSafetyErasure(ctx, id)
	if err != nil {
		return false, fmt.Errorf("safety: pending erasure: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("safety: commit erasure: %w", err)
	}
	return !pending.Bool, nil
}

func toBlock(id pgtype.UUID, label, kind string, created pgtype.Timestamptz) domain.Block {
	return domain.Block{ID: uuidString(id), Label: label, SourceKind: domain.ContentKind(kind), CreatedAt: created.Time.UTC()}
}

func toUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(strings.ReplaceAll(text, "-", ""))
	if err != nil || len(raw) != 16 || len(text) != 36 {
		return pgtype.UUID{}, errors.New("safety: malformed uuid")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	h := hex.EncodeToString(id.Bytes[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func newUUID() (pgtype.UUID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return pgtype.UUID{}, fmt.Errorf("safety: uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: b, Valid: true}, nil
}
