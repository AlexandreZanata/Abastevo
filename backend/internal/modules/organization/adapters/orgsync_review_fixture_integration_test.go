//go:build integration

package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

func (h *syncSQLHandler) ReviewProof() syncapp.ReviewProof { return h }
func (h *syncSQLHandler) CheckHeldTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	if _, leaked := db.(interface{ Commit(context.Context) error }); leaked {
		return syncapp.ErrUnavailable
	}
	var code, value, target, member, owner, origin, enrollment, digest, kind string
	err := db.QueryRow(ctx, `SELECT i.code,r.value,r.target_id::text,r.author_member::text,o.owner_account_id::text,o.api_origin,o.enrollment_id::text,o.payload_digest,o.operation_type FROM org_sync_review_items i JOIN org_sync_test_records r ON(r.org_id,r.operation_id,r.review_id)=(i.org_id,i.operation_id,i.id) JOIN org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest) WHERE i.org_id=$1 AND i.id=$2 AND i.operation_id=$3 AND r.held AND i.proof_version=1`, source.Scope.OrgID().String(), outcome.ReviewItemID, op.ID()).Scan(&code, &value, &target, &member, &owner, &origin, &enrollment, &digest, &kind)
	var payload struct {
		Value string `json:"value"`
	}
	if err != nil || json.Unmarshal(op.PayloadBytes(), &payload) != nil || outcome.Result != protocol.AcceptedForReview || outcome.EntityRevision != nil || outcome.Code != code || value != payload.Value || target != op.Snapshot().Entity.ID || member != source.MembershipID || owner != source.Actor.AccountID || origin != source.Origin || enrollment != source.EnrollmentID || digest != op.PayloadDigest() || kind != syncTestType {
		return syncapp.ErrUnavailable
	}
	return nil
}
func (h *syncSQLHandler) CheckOwnReceiptTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation) (bool, error) {
	var id, code string
	err := db.QueryRow(ctx, `SELECT i.id::text,i.code FROM org_sync_review_items i JOIN org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version) JOIN org_sync_operations o ON(o.org_id,o.operation_id)=(i.org_id,i.operation_id) WHERE i.org_id=$1 AND i.operation_id=$2 AND o.owner_account_id=$3 AND o.api_origin=$4 AND o.membership_id=$5 AND o.enrollment_id=$6 AND o.payload_digest=$7`, source.Scope.OrgID().String(), op.ID(), source.Actor.AccountID, source.Origin, source.MembershipID, source.EnrollmentID, op.PayloadDigest()).Scan(&id, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, syncapp.ErrUnavailable
	}
	if err := h.CheckHeldTx(ctx, db, source, op, syncapp.Outcome{Result: protocol.AcceptedForReview, Code: code, ReviewItemID: id}); err != nil {
		return false, err
	}
	return true, nil
}
func syncRecordReview(ctx context.Context, db syncapp.DBTX, source syncapp.Source, op protocol.Operation, outcome syncapp.Outcome) error {
	if outcome.Result != protocol.AcceptedForReview {
		return nil
	}
	tag, err := db.Exec(ctx, `INSERT INTO org_sync_review_items(org_id,id,operation_id,operation_type,operation_schema,payload_digest,code,author_employee_id) SELECT o.org_id,$2,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest,$4,m.employee_id FROM org_sync_operations o JOIN org_memberships m ON(m.org_id,m.id)=(o.org_id,o.membership_id) JOIN org_sync_test_records r ON(r.org_id,r.operation_id)=(o.org_id,o.operation_id) WHERE o.org_id=$1 AND o.operation_id=$3 AND r.held AND r.review_id=$2 AND r.author_member=o.membership_id`, source.Scope.OrgID().String(), outcome.ReviewItemID, op.ID(), outcome.Code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return syncapp.ErrUnavailable
	}
	return nil
}

// This named synthetic owner extends only the actual validation.setting held
// table. Unknown owners still delegate to the unchanged production Meter source.
func syncInstallReviewSource(t *testing.T, fx deviceFixtureState) {
	t.Helper()
	_, err := fx.f.Admin.Exec(t.Context(), `ALTER FUNCTION org_sync_review_source_valid(uuid,uuid,text,bigint) RENAME TO org_sync_review_source_valid_synthetic_base;
 CREATE FUNCTION org_sync_review_source_valid(org uuid,review uuid,owner_type text,owner_schema bigint) RETURNS boolean LANGUAGE SQL VOLATILE SET search_path=pg_catalog AS $$
 SELECT CASE WHEN owner_type='validation.setting' AND owner_schema=1 THEN EXISTS(
 SELECT 1 FROM public.org_sync_review_items i JOIN public.org_sync_test_records r ON(r.org_id,r.operation_id,r.review_id)=(i.org_id,i.operation_id,i.id)
 JOIN public.org_sync_operations o ON(o.org_id,o.operation_id,o.operation_type,o.operation_schema,o.payload_digest)=(i.org_id,i.operation_id,i.operation_type,i.operation_schema,i.payload_digest)
 JOIN public.org_sync_acknowledgements a ON(a.org_id,a.operation_id,a.review_item_id,a.result,a.code,a.review_proof_version)=(i.org_id,i.operation_id,i.id,i.acknowledgement_result,i.code,i.proof_version)
 JOIN public.org_memberships m ON(m.org_id,m.id,m.employee_id)=(i.org_id,o.membership_id,i.author_employee_id)
 WHERE i.org_id=org AND i.id=review AND i.operation_type=owner_type AND i.operation_schema=owner_schema AND o.entity_type='setting' AND o.entity_id=r.target_id AND r.held AND r.author_member=o.membership_id)
 ELSE public.org_sync_review_source_valid_synthetic_base(org,review,owner_type,owner_schema) END $$;`)
	if err != nil {
		t.Fatal("named actual held fixture source unavailable", err)
	}
}
