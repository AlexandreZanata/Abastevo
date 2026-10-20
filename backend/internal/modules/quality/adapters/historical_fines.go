package adapters

import (
	"context"
	"errors"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

// PublishHistoricalFines stages source context, observations and public notes in
// the existing atomic provenance/correction transaction. It infers no station links.
func (p *Publisher) PublishHistoricalFines(ctx context.Context, in application.HistoricalFineInput, newID func() (string, error)) (string, error) {
	if e := in.Validate(); e != nil {
		return "", e
	}
	req := editionRequest{Source: "sanctions", Stream: domain.StreamSanction, Edition: in.Context.SourcePeriod, Predecessor: in.Predecessor, Resources: in.Resources, Counts: in.Counts, ManifestHash: in.ManifestHash, HistoricalFineContext: &in.Context}
	stage := func(q *quality.Queries, _, revision pgtype.UUID) error {
		c := in.Context
		if _, e := q.RecordHistoricalFineContext(ctx, quality.RecordHistoricalFineContextParams{RevisionID: revision, InputContract: c.InputContract, SourcePeriod: c.SourcePeriod, SourceFile: c.SourceFile, HeaderRecord: int32(c.HeaderRecord), SourceClockPrecision: c.Clock.Precision, SourceClockValue: c.Clock.Value, SourceRecordCount: int32(c.SourceRecordCount), MetadataCount: int32(c.MetadataRecords)}); e != nil {
			return e
		}
		for _, row := range in.Observations {
			id, e := nextID(newID)
			if e != nil {
				return e
			}
			due, e := pgDate(row.DueOn)
			if e != nil {
				return e
			}
			transit := pgtype.Date{}
			if row.SourceTransitOn != nil {
				transit, e = pgDate(*row.SourceTransitOn)
				if e != nil {
					return e
				}
			}
			paid := pgtype.Int8{}
			if row.PaidMinor != nil {
				paid = pgtype.Int8{Int64: *row.PaidMinor, Valid: true}
			}
			if _, e = q.StageHistoricalFineObservation(ctx, quality.StageHistoricalFineObservationParams{ID: id, RevisionID: revision, NativeKey: row.ObservationKey, ProceedingRef: row.ProceedingRef, PartyCnpj: row.SourceCNPJ, AmountMinor: row.AmountMinor, Status: row.Status, EvidenceRef: row.EvidenceRef, ReferenceLabel: pgText(row.ReferenceLabel), ReferenceRef: pgText(row.ReferenceRef), SourceTransitOn: transit, UnlabelledField: pgText(row.UnlabelledField), Department: pgText(row.Department), DueOn: due, AmountText: pgText(row.AmountText), PaidText: pgText(row.PaidText), PaidMinor: paid}); e != nil {
				return e
			}
		}
		for _, note := range in.Notes {
			if _, e := q.StageHistoricalFineNote(ctx, quality.StageHistoricalFineNoteParams{RevisionID: revision, EvidenceRef: note.EvidenceRef, Kind: note.Kind, NoteText: note.Text}); e != nil {
				return e
			}
		}
		count, e := q.CountRevisionSanctions(ctx, revision)
		if e != nil {
			return e
		}
		notes, e := q.HistoricalFineNotes(ctx, revision)
		if e != nil {
			return e
		}
		if count != int64(len(in.Observations)) || len(notes) != len(in.Notes) {
			return errors.New("quality: historical fine staged counts diverged")
		}
		return nil
	}
	for attempt := 0; ; attempt++ {
		id, retry, e := p.publishTx(ctx, req, newID, stage)
		if !retry || attempt >= 2 {
			return id, e
		}
	}
}
