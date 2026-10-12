package adapters

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

const finalWriteLease = 60 * time.Second

func canonicalDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

// BeginVerificationWrite commits the exact destination before any PUT. A
// failed/ambiguous write keeps its lease and inventory until cleanup confirms
// absence. Retrying never silently replaces an unresolved provider effect.
func (s *Store) BeginVerificationWrite(ctx context.Context, sessionID, key, sanitizedHash, nonce string) (application.FinalWrite, error) {
	var none application.FinalWrite
	uid, err := mustUUID(sessionID)
	if err != nil {
		return none, err
	}
	token, err := mustUUID(nonce)
	if err != nil {
		return none, err
	}
	if !canonicalDigest(sanitizedHash) {
		return none, domain.ErrBadTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return none, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockFinalOutputKey(ctx, key); err != nil {
		return none, err
	}
	sess, err := q.GetUploadSessionForUpdate(ctx, uid)
	if err != nil {
		return none, err
	}
	now := time.Now()
	if sess.Status != domain.StateVerifying || key != "f/"+sess.ClaimedSha256 || !canonicalDigest(sess.ClaimedSha256) || !now.Add(finalWriteLease).Before(sess.CreatedAt.Time.Add(application.CopyRetention)) {
		return none, domain.ErrBadTransition
	}
	if err := q.InventoryExistingFinalOutput(ctx, key); err != nil {
		return none, err
	}
	if err := q.InventoryFinalOutput(ctx, evidence.InventoryFinalOutputParams{FinalKey: key, SessionID: uid, SanitizedSha256: sanitizedHash, DeadlineAt: pgTime(sess.CreatedAt.Time.Add(application.CopyRetention)), CreatedAt: sess.CreatedAt}); err != nil {
		return none, err
	}
	row, err := q.GetFinalOutputForUpdate(ctx, key)
	if err != nil {
		return none, err
	}
	if !now.Add(finalWriteLease).Before(row.DeadlineAt.Time) {
		return none, domain.ErrBadTransition
	}
	if row.State == "READY" {
		object, err := q.GetLiveFinalObject(ctx, key)
		if err != nil {
			return none, err
		}
		if object.SourceSha256 != sess.ClaimedSha256 || object.SanitizedSha256 != sanitizedHash {
			return none, domain.ErrBadTransition
		}
		return application.FinalWrite{Reused: true}, tx.Commit(ctx)
	}
	if row.State != "PENDING" || row.SessionID != uid {
		return none, domain.ErrBadTransition
	}
	n, err := q.BeginFinalOutputWrite(ctx, evidence.BeginFinalOutputWriteParams{FinalKey: key, SessionID: uid, SanitizedSha256: sanitizedHash, LeaseNonce: token, LeaseUntil: pgTime(now.Add(finalWriteLease))})
	if err != nil {
		return none, err
	}
	if n != 1 {
		return none, domain.ErrBadTransition
	}
	return application.FinalWrite{Nonce: nonce}, tx.Commit(ctx)
}

// RecordVerifiedGuarded is the only production verification commit port. The
// legacy metadata helper below Store.RecordVerified does not grant byte writes.
// Session expiry and outcome commit take the same row lock; no stale snapshot
// can recreate READY after an accepted erasure transition.
func (s *Store) RecordVerifiedGuarded(ctx context.Context, sessionID string, obj ObjectData, authority application.FinalWrite) (string, error) {
	uid, err := mustUUID(sessionID)
	if err != nil {
		return "", err
	}
	oid, err := mustUUID(obj.ID)
	if err != nil {
		return "", err
	}
	if !canonicalDigest(obj.SourceSHA256) || !canonicalDigest(obj.SanitizedSHA256) || obj.FinalKey != "f/"+obj.SourceSHA256 || obj.Width < 1 || obj.Height < 1 {
		return "", domain.ErrBadTransition
	}
	if authority.Reused == (authority.Nonce != "") {
		return "", domain.ErrBadTransition
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := evidence.New(tx)
	if err := q.LockFinalOutputKey(ctx, obj.FinalKey); err != nil {
		return "", err
	}
	sess, err := q.GetUploadSessionForUpdate(ctx, uid)
	if err != nil {
		return "", err
	}
	if sess.Status != domain.StateVerifying || obj.SourceSHA256 != sess.ClaimedSha256 {
		return "", domain.ErrBadTransition
	}
	row, err := q.GetFinalOutputForUpdate(ctx, obj.FinalKey)
	if err != nil {
		return "", err
	}
	now := time.Now()
	if !now.Before(row.DeadlineAt.Time) || !now.Before(sess.CreatedAt.Time.Add(application.CopyRetention)) || row.SanitizedSha256 != obj.SanitizedSHA256 {
		return "", domain.ErrBadTransition
	}
	id := obj.ID
	if authority.Reused {
		if row.State != "READY" {
			return "", domain.ErrBadTransition
		}
		object, err := q.GetLiveFinalObject(ctx, obj.FinalKey)
		if err != nil {
			return "", err
		}
		if object.SourceSha256 != obj.SourceSHA256 || object.SanitizedSha256 != obj.SanitizedSHA256 {
			return "", domain.ErrBadTransition
		}
		id = uuidString(object.ID)
	} else {
		token, err := mustUUID(authority.Nonce)
		if err != nil {
			return "", err
		}
		if row.State != "WRITING" || row.SessionID != uid || row.LeaseNonce != token || !now.Before(row.LeaseUntil.Time) {
			return "", domain.ErrBadTransition
		}
		// Use the reservation anchor, never caller UpdatedAt or a later retry time.
		n, err := q.InsertGuardedObject(ctx, evidence.InsertGuardedObjectParams{ID: oid, SessionID: uid, FinalKey: obj.FinalKey, SourceSha256: obj.SourceSHA256, SanitizedSha256: obj.SanitizedSHA256, Width: int32(obj.Width), Height: int32(obj.Height), Dhash: int64(obj.DHash), CreatedAt: pgTime(now), ReceivedAt: sess.CreatedAt})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", domain.ErrBadTransition
		}
		n, err = q.FinishFinalOutputWrite(ctx, evidence.FinishFinalOutputWriteParams{FinalKey: obj.FinalKey, SessionID: uid, LeaseNonce: token})
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", domain.ErrBadTransition
		}
	}
	n, err := q.MarkSessionReady(ctx, uid)
	if err != nil {
		return "", err
	}
	if n != 1 {
		return "", domain.ErrBadTransition
	}
	return id, tx.Commit(ctx)
}
