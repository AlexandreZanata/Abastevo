package adapters

import (
	"context"
	"encoding/json"
	"errors"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

// PublishEnforcementSource uses the shared guarded atomic correction/replay
// machinery. Chunks bound SQL parameters and avoid one round trip per fact.
func (p *Publisher) PublishEnforcementSource(ctx context.Context, in application.EnforcementSourceInput, newID func() (string, error)) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	req := editionRequest{Source: "enforcement", Stream: domain.StreamEnforcement, Edition: application.EnforcementSourceEdition, Predecessor: in.Predecessor, Resources: in.Resources, Counts: in.Counts, ManifestHash: in.ManifestHash, EnforcementContext: &in.Context}
	stage := func(q *quality.Queries, _, revision pgtype.UUID) error {

		var first, last pgtype.Date
		if in.Context.FirstDocumentDay != "" {
			first, _ = pgDate(in.Context.FirstDocumentDay)
			last, _ = pgDate(in.Context.LastDocumentDay)
		}
		n, e := q.RecordEnforcementSourceContext(ctx, quality.RecordEnforcementSourceContextParams{RevisionID: revision, SourceProfile: in.Context.Profile, SourceFile: in.Context.SourceFile, DateSystem: in.Context.DateSystem, InputCount: int32(in.Counts.Input), AcceptedCount: int32(in.Counts.Accepted), DuplicateCount: int32(in.Counts.Duplicates), FirstDocumentDay: first, LastDocumentDay: last})
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("quality: enforcement context not staged")
		}
		type record struct {
			ID string `json:"id"`
			application.EnforcementSourceRecord
		}
		const chunk = 128
		for start := 0; start < len(in.Records); start += chunk {
			end := min(start+chunk, len(in.Records))
			rows := make([]record, 0, end-start)
			for _, row := range in.Records[start:end] {
				id, e := nextID(newID)
				if e != nil {
					return e
				}
				rows = append(rows, record{ID: uuidString(id), EnforcementSourceRecord: row})
			}
			raw, e := json.Marshal(rows)
			if e != nil {
				return e
			}
			n, e := q.StageEnforcementSourceRecords(ctx, quality.StageEnforcementSourceRecordsParams{RevisionID: revision, Records: raw})
			if e != nil {
				return e
			}
			if n != int64(len(rows)) {
				return errors.New("quality: enforcement chunk not staged")
			}
		}
		n, e = q.CountEnforcementSourceRecords(ctx, revision)
		if e != nil {
			return e
		}
		if n != int64(in.Counts.Accepted) {
			return errors.New("quality: enforcement count diverged")
		}
		return nil
	}
	for attempt := 0; ; attempt++ {
		rev, retry, e := p.publishTx(ctx, req, newID, stage)
		if !retry || attempt >= 2 {
			return rev, e
		}
	}
}
