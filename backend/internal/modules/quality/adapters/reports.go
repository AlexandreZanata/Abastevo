package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	jobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReportAccess ports are supplied by composition. Guards must lock live
// account/session/binding/key until the transaction commits, including replays.
type ReportAccess struct {
	Owner string
	Guard func(context.Context, pgx.Tx) error
}

type ReportStore struct {
	Pool    *pgxpool.Pool
	Now     func() time.Time
	Station func(context.Context, string) (bool, error)
	Enqueue func(context.Context, pgx.Tx, []byte, string) error
}

func (s *ReportStore) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *ReportStore) begin(ctx context.Context, a ReportAccess) (pgx.Tx, error) {
	if a.Guard == nil {
		return nil, domain.ErrReportUnauthorized
	}
	if _, err := mustUUID(a.Owner); err != nil {
		return nil, domain.ErrReportUnauthorized
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if err = a.Guard(ctx, tx); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s *ReportStore) Execute(ctx context.Context, a ReportAccess, c domain.ReportCommand) (domain.ReportReceipt, error) {
	var out domain.ReportReceipt
	now := s.now()
	if err := c.Validate(now); err != nil {
		return out, err
	}
	tx, err := s.begin(ctx, a)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serializes only this owner's report commands; stable ordering also fences
	// a concurrent replay when tests inject a guard without account row locks.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "quality-reports:"+a.Owner); err != nil {
		return out, err
	}
	q := quality.New(tx)
	owner, _ := mustUUID(a.Owner)
	id, _ := mustUUID(c.ReportID)
	commandID, _ := mustUUID(c.CommandID)
	receipt, e := q.GetReportCommand(ctx, quality.GetReportCommandParams{AccountID: owner, CommandID: commandID})
	if e == nil {
		if receipt.PayloadHash != c.Hash() {
			return out, domain.ErrReportConflict
		}
		out = domain.ReportReceipt{ReportID: uuidString(receipt.ReportID), Revision: receipt.Revision, Status: receipt.Status}
		return out, tx.Commit(ctx)
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	revision := int32(1)
	status := "PENDING_REVIEW"
	if c.Action != "WITHDRAW" {
		if s.Station == nil {
			return out, domain.ErrReportUnavailable
		}
		exists, e := s.Station(ctx, c.StationID)
		if e != nil {
			return out, e
		}
		if !exists {
			return out, domain.ErrReportInvalid
		}
		if c.RevisionID != "" {
			rid, _ := mustUUID(c.RevisionID)
			station, _ := mustUUID(c.StationID)
			rows, e := q.LockReportSampleTarget(ctx, quality.LockReportSampleTargetParams{RevisionID: rid, StationID: station, SampleRef: c.SampleRef, Fuel: c.Fuel})
			if e != nil {
				return out, e
			}
			if len(rows) == 0 {
				return out, domain.ErrReportConflict
			}
		}
	}
	at := pgtype.Timestamptz{Time: now, Valid: true}
	if c.Action == "SUBMIT" {
		station, _ := mustUUID(c.StationID)
		day, _ := pgDate(c.OccurredOn)
		var rid pgtype.UUID
		if c.RevisionID != "" {
			rid, _ = mustUUID(c.RevisionID)
		}
		err = q.InsertReport(ctx, quality.InsertReportParams{ID: id, AccountID: owner, Kind: c.Kind, StationID: station, Fuel: c.Fuel, OccurredOn: day, Symptom: c.Symptom, Body: c.Text, OfficialRevisionID: rid, SampleRef: c.SampleRef, At: at})
		if isQualityUnique(err) {
			return out, domain.ErrReportConflict
		}
		if err != nil {
			return out, err
		}
	} else {
		old, e := q.LockReport(ctx, quality.LockReportParams{ID: id, AccountID: owner})
		if errors.Is(e, pgx.ErrNoRows) {
			return out, domain.ErrReportNotFound
		}
		if e != nil {
			return out, e
		}
		if old.Revision != c.ExpectedRevision || old.Revision == math.MaxInt32 || old.Status == "WITHDRAWN" || old.Status == "HIDDEN" {
			return out, domain.ErrReportConflict
		}
		revision = old.Revision + 1
		body := c.Text
		day := old.OccurredOn
		symptom := old.Symptom
		if c.Action == "WITHDRAW" {
			status = "WITHDRAWN"
			body = ""
		} else {
			if c.Kind != old.Kind || c.StationID != uuidString(old.StationID) || c.Fuel != old.Fuel || c.RevisionID != uuidString(old.OfficialRevisionID) || c.SampleRef != old.SampleRef {
				return out, domain.ErrReportInvalid
			}
			day, _ = pgDate(c.OccurredOn)
			symptom = c.Symptom
		}
		if err = q.UpdateReport(ctx, quality.UpdateReportParams{ID: id, AccountID: owner, Body: body, OccurredOn: day, Symptom: symptom, Revision: revision, Status: status, At: at}); err != nil {
			return out, err
		}
	}
	if err = q.InsertReportAudit(ctx, quality.InsertReportAuditParams{ReportID: id, Revision: revision, Action: c.Action, PolicyVersion: "quality-report-v1", At: at}); err != nil {
		return out, err
	}
	if err = q.InsertReportCommand(ctx, quality.InsertReportCommandParams{AccountID: owner, CommandID: commandID, PayloadHash: c.Hash(), ReportID: id, Revision: revision, Status: status, At: at}); err != nil {
		return out, err
	}
	if status == "PENDING_REVIEW" {
		payload, _ := json.Marshal(ReportReviewPayload{Version: 1, ReportID: c.ReportID, Revision: revision})
		dedupe := fmt.Sprintf("quality-review:%s:%d", c.ReportID, revision)
		if s.Enqueue != nil {
			err = s.Enqueue(ctx, tx, payload, dedupe)
		} else {
			_, err = jobs.Enqueue(ctx, tx, "quality-report-review", payload, dedupe, 5, time.Time{})
		}
		if err != nil {
			return out, err
		}
	}
	out = domain.ReportReceipt{ReportID: c.ReportID, Revision: revision, Status: status}
	return out, tx.Commit(ctx)
}

func isQualityUnique(err error) bool {
	type coder interface{ SQLState() string }
	var c coder
	return errors.As(err, &c) && c.SQLState() == "23505"
}

func reportView(r quality.QualityReport) domain.Report {
	return domain.Report{ID: uuidString(r.ID), Revision: r.Revision, Kind: r.Kind, StationID: uuidString(r.StationID), Fuel: r.Fuel, OccurredOn: r.OccurredOn.Time.Format("2006-01-02"), Symptom: r.Symptom, Text: r.Body, OfficialRevisionID: uuidString(r.OfficialRevisionID), SampleRef: r.SampleRef, Status: r.Status, EvidenceLevel: "USER_CLAIM"}
}

func (s *ReportStore) Get(ctx context.Context, a ReportAccess, id string) (domain.Report, error) {
	var out domain.Report
	uid, e := mustUUID(id)
	if e != nil {
		return out, domain.ErrReportInvalid
	}
	tx, e := s.begin(ctx, a)
	if e != nil {
		return out, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	owner, _ := mustUUID(a.Owner)
	r, e := quality.New(tx).GetReport(ctx, quality.GetReportParams{ID: uid, AccountID: owner})
	if errors.Is(e, pgx.ErrNoRows) {
		return out, domain.ErrReportNotFound
	}
	if e != nil {
		return out, e
	}
	return reportView(r), tx.Commit(ctx)
}

func (s *ReportStore) List(ctx context.Context, a ReportAccess, after string) ([]domain.Report, error) {
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	uid, e := mustUUID(after)
	if e != nil {
		return nil, domain.ErrReportInvalid
	}
	tx, e := s.begin(ctx, a)
	if e != nil {
		return nil, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	owner, _ := mustUUID(a.Owner)
	rows, e := quality.New(tx).ListOwnerReports(ctx, quality.ListOwnerReportsParams{AccountID: owner, AfterID: uid, PageLimit: 51})
	if e != nil {
		return nil, e
	}
	out := make([]domain.Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, reportView(r))
	}
	return out, tx.Commit(ctx)
}

// EraseAccountBatch is bounded and idempotent; audits retain no owner/body.
func (s *ReportStore) EraseAccountBatch(ctx context.Context, owner string) (bool, error) {
	id, e := mustUUID(owner)
	if e != nil {
		return false, e
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := quality.New(tx)
	n, e := q.EraseQualityReports(ctx, id)
	if e != nil {
		return false, e
	}
	m, e := q.EraseQualityReportCommands(ctx, id)
	if e != nil {
		return false, e
	}
	return n < 100 && m < 100, tx.Commit(ctx)
}

type ReportReviewPayload struct {
	Version  int    `json:"version"`
	ReportID string `json:"report_id"`
	Revision int32  `json:"revision"`
}
