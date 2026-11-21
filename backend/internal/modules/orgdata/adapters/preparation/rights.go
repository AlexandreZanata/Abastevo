package preparation

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	queries "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdataops"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidRightsDecision = errors.New("invalid Data rights decision")
	ErrRightsRevision        = errors.New("data rights decision requires the next source revision")
	ErrRightsOperationReused = errors.New("data rights operation reused with different intent or actor")
	rightsDigest             = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	decisionReference        = regexp.MustCompile(`^[!-~]{1,200}$`)
)

// RightsDecision identifies the exact input covered by an operator decision.
type RightsDecision struct {
	OperationID      string
	Source           domain.Source
	Digest           string
	Revision         int64
	State            domain.RightsState
	DecisionRef      string
	LicenseRef       string
	SnapshotDir      string
	SnapshotDigest   string
	OfficialRevision string
}

func (d RightsDecision) Validate() error {
	if !audit.ValidID(d.OperationID) || !rightsDigest.MatchString(d.Digest) ||
		d.Revision < 1 || !decisionReference.MatchString(d.DecisionRef) ||
		(d.Source != domain.PublicRegistry && d.Source != domain.OfficialPrices) {
		return ErrInvalidRightsDecision
	}
	switch d.State {
	case domain.Approved:
		if d.LicenseRef == "" || len(d.LicenseRef) > 300 || !utf8.ValidString(d.LicenseRef) ||
			strings.TrimSpace(d.LicenseRef) != d.LicenseRef || strings.ContainsFunc(d.LicenseRef, unicode.IsControl) ||
			d.SnapshotDir == "" || !rightsDigest.MatchString(d.SnapshotDigest) ||
			(d.Source == domain.OfficialPrices && !audit.ValidID(d.OfficialRevision)) ||
			(d.Source == domain.PublicRegistry && d.OfficialRevision != "") {
			return ErrInvalidRightsDecision
		}
	case domain.Withdrawn:
		if d.LicenseRef != "" || d.SnapshotDir != "" || d.SnapshotDigest != "" || d.OfficialRevision != "" {
			return ErrInvalidRightsDecision
		}
	default:
		return ErrInvalidRightsDecision
	}
	return nil
}

func (d RightsDecision) matchSnapshot(m enterpriseexport.Manifest) error {
	if m.SchemaVersion != enterpriseexport.SchemaVersion || m.ContentVersion != enterpriseexport.ContentVersion ||
		m.PublicFieldPolicy != enterpriseexport.PublicFieldPolicy {
		return domain.ErrSourceBinding
	}
	switch d.Source {
	case domain.PublicRegistry:
		if m.Sources.Directory.Source != string(domain.PublicRegistry) || m.Sources.Directory.Kind != "directory" {
			return domain.ErrSourceBinding
		}
		for _, file := range m.Files {
			if file.Name == "stations.jsonl" && file.SHA256 == d.Digest {
				return nil
			}
		}
	case domain.OfficialPrices:
		var matching int
		for _, official := range m.Sources.Official {
			if official.RevisionID != d.OfficialRevision {
				continue
			}
			if official.SourceChecksum != d.Digest {
				return domain.ErrSourceBinding
			}
			matching++
		}
		if matching == 1 {
			return nil
		}
	}
	return domain.ErrSourceBinding
}

// RecordRights appends one audited source review or replays the exact intent.
// Approval verifies the immutable export before taking the shared catalog's
// writer lock. Withdrawal requires a prior review of the exact digest.
func RecordRights(ctx context.Context, pool *pgxpool.Pool, d RightsDecision) (bool, error) {
	if err := d.Validate(); err != nil {
		return false, err
	}
	attribution := ""
	if d.State == domain.Approved {
		if _, _, err := enterpriseexport.ReadVerifiedCore(ctx, d.SnapshotDir, d.SnapshotDigest, d.matchSnapshot); err != nil {
			return false, err
		}
		attribution = domain.ANPAttribution
	}
	var replayed bool
	err := database.WithOrgBootstrapTx(ctx, pool, func(tx pgx.Tx) error {
		q := queries.New(tx)
		if err := q.LockDataRightsReviewWrite(ctx); err != nil {
			return err
		}
		previous, err := q.ReadDataRightsOperation(ctx, d.OperationID)
		if err == nil {
			if !previous.SameActor || previous.SourceCode != string(d.Source) || previous.SourceDigest != d.Digest ||
				previous.Revision != d.Revision || previous.Status != string(d.State) || previous.DecisionRef != d.DecisionRef ||
				previous.LicenseRef != d.LicenseRef || previous.SnapshotDigest != d.SnapshotDigest ||
				previous.OfficialRevision != d.OfficialRevision {
				return ErrRightsOperationReused
			}
			replayed = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		latest, err := q.ReadLatestSourceReview(ctx, queries.ReadLatestSourceReviewParams{
			SourceCode: string(d.Source), SourceDigest: d.Digest,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if d.Revision-1 != latest.Revision {
			return ErrRightsRevision
		}
		if d.State == domain.Withdrawn && latest.Status != string(domain.Approved) {
			return domain.ErrRightsPending
		}
		return q.InsertDataRightsOperation(ctx, queries.InsertDataRightsOperationParams{
			OperationID: d.OperationID, SourceCode: string(d.Source), SourceDigest: d.Digest,
			Revision: d.Revision, Status: string(d.State), DecisionRef: d.DecisionRef, LicenseRef: d.LicenseRef,
			Attribution: attribution, SnapshotDigest: d.SnapshotDigest, OfficialRevision: d.OfficialRevision,
		})
	})
	return replayed && err == nil, err
}
