package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	docapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// DocumentTargets is the fleet owner's document link resolver (EMP-DOC-003)
// for assets and meter readings. It reads current server facts in the
// caller's scoped transaction and grants nothing by itself.
type DocumentTargets struct{}

var _ docapp.TargetResolver = DocumentTargets{}

// DocumentTargetPermission is the fleet permission that governs associating a
// document with a record: asset document links are fleet.asset.manage
// (fleet permission table) and reading evidence is meter.reading.record
// (BR-METER-025), own readings for SELF roles.
func (DocumentTargets) DocumentTargetPermission(kind docdomain.TargetKind) (permissions.Permission, error) {
	switch kind {
	case docdomain.Asset:
		return "fleet.asset.manage", nil
	case docdomain.MeterReading:
		return "meter.reading.record", nil
	}
	return "", docapp.ErrInvalid
}

// ResolveDocumentTargetTx reads an asset (a DRAFT, which may still be
// discarded, refuses links) or a reading: a stored reading names its author,
// a held reading the member of its accepted operation. Both share one
// reading identity; the stored row wins once a held reading is accepted.
func (DocumentTargets) ResolveDocumentTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, kind docdomain.TargetKind, id string) (docapp.ResolvedTarget, error) {
	if tx == nil || scope.Validate() != nil {
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	if !validID(id) {
		return docapp.ResolvedTarget{}, docapp.ErrNotFound
	}
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return docapp.ResolvedTarget{}, docapp.ErrUnavailable
	}
	org := scope.OrgID().String()
	switch kind {
	case docdomain.Asset:
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM org_assets WHERE org_id=$1 AND id=$2`, org, id).Scan(&state); err != nil {
			return docapp.ResolvedTarget{}, documentTargetError(err)
		}
		if fleet.State(state) == fleet.Draft {
			return docapp.ResolvedTarget{}, docapp.ErrConflict
		}
		return docapp.ResolvedTarget{Binding: docapp.BindAsset}, nil
	case docdomain.MeterReading:
		var author string
		err := tx.QueryRow(ctx, `SELECT author_membership_id::text FROM org_meter_readings WHERE org_id=$1 AND id=$2`, org, id).Scan(&author)
		if err == nil {
			return docapp.ResolvedTarget{Binding: docapp.BindMeterReading, AuthoredBy: author}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return docapp.ResolvedTarget{}, docapp.ErrUnavailable
		}
		if err := tx.QueryRow(ctx, `SELECT o.membership_id::text FROM org_meter_held_readings h
			JOIN org_sync_operations o ON o.org_id=h.org_id AND o.operation_id=h.operation_id
			WHERE h.org_id=$1 AND h.reading_id=$2`, org, id).Scan(&author); err != nil {
			return docapp.ResolvedTarget{}, documentTargetError(err)
		}
		return docapp.ResolvedTarget{Binding: docapp.BindHeldMeterReading, AuthoredBy: author}, nil
	}
	return docapp.ResolvedTarget{}, docapp.ErrInvalid
}

func documentTargetError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return docapp.ErrNotFound
	}
	return docapp.ErrUnavailable
}
