package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReferenceAuthority loads the member's current role assignments.
type ReferenceAuthority interface {
	LoadAuthorityTx(context.Context, access.AuthorityQueries, kernel.OrgScope, string) (access.Authority, error)
}

// ReferenceStore derives and serves the per-membership reference feed
// (EMP-OFFL-006, EDR-218).
type ReferenceStore struct {
	Pool      *pgxpool.Pool
	Confirm   func(context.Context, pgx.Tx, app.Source) (time.Time, error)
	Authority ReferenceAuthority
	Assets    app.ReferenceAssets
	Secret    []byte
	NewEpoch  func() (string, error)
}

const referenceReadPermission permission.Permission = "fleet.asset.read"

// referenceCovered returns the member's purpose set: its assigned assets that
// a current fleet.asset.read rule covers, and whether any rule exists at all.
func referenceCovered(scope kernel.OrgScope, member string, authority access.Authority, sources []app.ReferenceAssetSource, at time.Time) (map[string]app.ReferenceAsset, bool) {
	assignments := authority.Assignments
	self, _ := permission.SelfCandidate(scope, member, assignments, referenceReadPermission, at)
	structural, _ := permission.StructuralCandidate(scope, member, assignments, referenceReadPermission, at)
	_, orgErr := permission.CoveringRules(scope, member, assignments, referenceReadPermission, permission.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, at)
	any := self || structural || orgErr == nil
	covered := map[string]app.ReferenceAsset{}
	if !any {
		return covered, false
	}
	for _, source := range sources {
		facts := permission.ResourceFacts{OrgID: scope.OrgID(), ID: source.Asset.ID, AssignedTo: member, AssetID: source.Asset.ID, UnitID: source.UnitID}
		if rules, err := permission.CoveringRules(scope, member, assignments, referenceReadPermission, facts, at); err == nil && len(rules) > 0 {
			covered[source.Asset.ID] = source.Asset
		}
	}
	return covered, true
}

func (s *ReferenceStore) PullReference(ctx context.Context, source app.Source, cursor *app.ReferenceCursor) (app.ReferenceFeed, error) {
	if s == nil || s.Pool == nil || s.Confirm == nil || s.Authority == nil || nilPort(s.Assets) || s.NewEpoch == nil || len(s.Secret) < 32 || source.Validate() != nil {
		return app.ReferenceFeed{}, app.ErrUnavailable
	}
	scope, org, member := source.Scope, source.Scope.OrgID().String(), source.MembershipID
	if cursor != nil && (cursor.OrgID != org || cursor.MembershipID != member || cursor.EnrollmentID != source.EnrollmentID) {
		return app.ReferenceFeed{}, app.ErrReferenceCursor
	}
	epoch, err := s.NewEpoch()
	if err != nil {
		return app.ReferenceFeed{}, app.ErrUnavailable
	}
	var feed app.ReferenceFeed
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		confirm := func() (time.Time, error) {
			at, e := s.Confirm(ctx, tx, source)
			if e != nil {
				if errors.Is(e, app.ErrSourceDenied) {
					return time.Time{}, e
				}
				return time.Time{}, app.SourceDenied(e)
			}
			if at.IsZero() {
				return time.Time{}, app.SourceDenied(app.ErrUnavailable)
			}
			return at, nil
		}
		at, e := confirm()
		if e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `INSERT INTO org_reference_feed_heads(org_id,membership_id,epoch) VALUES($1::uuid,$2::uuid,$3::uuid) ON CONFLICT(org_id,membership_id) DO NOTHING`, org, member, epoch); e != nil {
			return e
		}
		var headEpoch string
		var revision int64
		if e := tx.QueryRow(ctx, `SELECT epoch::text,revision FROM org_reference_feed_heads WHERE org_id=$1::uuid AND membership_id=$2::uuid FOR UPDATE`, org, member).Scan(&headEpoch, &revision); e != nil {
			return e
		}
		authority, e := s.Authority.LoadAuthorityTx(ctx, tx, scope, source.Actor.AccountID)
		if e != nil {
			return app.SourceDenied(e)
		}
		if authority.MembershipID != member {
			return app.SourceDenied(orgapi.NotFound)
		}
		sources, e := s.Assets.ReferenceAssetsDBTX(ctx, app.Borrow(tx), scope, member, at)
		if e != nil {
			return app.ErrUnavailable
		}
		covered, allowed := referenceCovered(scope, member, authority, sources, at)
		digests := make(map[string]string, len(covered))
		for id, asset := range covered {
			if digests[id], e = asset.Digest(); e != nil {
				return app.ErrUnavailable
			}
		}
		last := map[string]app.ReferenceEntry{}
		rows, e := tx.Query(ctx, `SELECT DISTINCT ON(entity_id) revision,entity_type,entity_id::text,op,digest FROM org_reference_feed_entries WHERE org_id=$1::uuid AND membership_id=$2::uuid ORDER BY entity_id,revision DESC`, org, member)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v app.ReferenceEntry
			if e := rows.Scan(&v.Revision, &v.EntityType, &v.EntityID, &v.Op, &v.Digest); e != nil {
				rows.Close()
				return e
			}
			last[v.EntityID] = v
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}
		changes := app.DiffReference(last, digests)
		if len(changes) > 0 {
			if _, e := tx.Exec(ctx, `UPDATE org_reference_feed_heads SET revision=revision+$3 WHERE org_id=$1::uuid AND membership_id=$2::uuid`, org, member, len(changes)); e != nil {
				return e
			}
			for i, change := range changes {
				change.Revision = revision + int64(i) + 1
				if _, e := tx.Exec(ctx, `INSERT INTO org_reference_feed_entries(org_id,membership_id,revision,entity_type,entity_id,op,digest) VALUES($1::uuid,$2::uuid,$3,$4,$5::uuid,$6,$7)`, org, member, change.Revision, change.EntityType, change.EntityID, change.Op, change.Digest); e != nil {
					return e
				}
				last[change.EntityID] = change
			}
			revision += int64(len(changes))
		}
		feed = app.ReferenceFeed{Mode: app.ReferenceDelta, Revision: revision, Changes: []app.ReferenceChange{}}
		var from int64
		switch {
		case !allowed:
			feed.Mode, feed.Reason = app.ReferenceReset, app.ReferenceForbidden
		case cursor == nil:
			feed.Mode, feed.Reason = app.ReferenceReset, app.ReferenceNewCursor
		case cursor.Epoch != headEpoch:
			feed.Mode, feed.Reason = app.ReferenceReset, app.ReferenceExpired
		case cursor.Revision > revision:
			feed.Mode, feed.Reason = app.ReferenceReset, app.ReferenceAhead
		default:
			from = cursor.Revision
			feed.FromRevision = &from
		}
		if allowed {
			for _, entry := range orderedReference(last) {
				if feed.Mode == app.ReferenceReset && entry.Op != app.ReferenceUpsert || feed.Mode == app.ReferenceDelta && entry.Revision <= from {
					continue
				}
				change := app.ReferenceChange{Revision: entry.Revision, EntityType: entry.EntityType, EntityID: entry.EntityID, Op: entry.Op}
				if entry.Op == app.ReferenceUpsert {
					asset := covered[entry.EntityID]
					if asset.ID != entry.EntityID {
						return app.ErrUnavailable
					}
					change.Asset = &asset
				}
				feed.Changes = append(feed.Changes, change)
			}
		}
		if len(feed.Changes) > app.ReferenceMaxEntities {
			return app.ErrUnavailable
		}
		confirmed, e := confirm()
		if e != nil {
			return e
		}
		feed.ConfirmedAt = confirmed
		feed.Cursor, e = app.SignReferenceCursor(s.Secret, app.ReferenceCursor{OrgID: org, MembershipID: member, EnrollmentID: source.EnrollmentID, Epoch: headEpoch, Revision: revision})
		return e
	})
	if err != nil {
		if errors.Is(err, app.ErrSourceDenied) {
			return app.ReferenceFeed{}, err
		}
		return app.ReferenceFeed{}, app.ErrUnavailable
	}
	return feed, nil
}

// orderedReference returns the latest entry per entity by revision.
func orderedReference(last map[string]app.ReferenceEntry) []app.ReferenceEntry {
	out := make([]app.ReferenceEntry, 0, len(last))
	for _, entry := range last {
		out = append(out, entry)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Revision < out[j-1].Revision; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

var _ app.ReferenceRepository = (*ReferenceStore)(nil)
