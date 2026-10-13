package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"time"
)

type OutputInventory interface {
	ValidateOutput(context.Context, PromotionResult, string, string) (string, error)
	BeginOutput(context.Context, PromotionResult, string, time.Time, string, string) error
	FinishOutput(context.Context, PromotionResult, string, bool, time.Time, string, string) error
	PrepareOutputCleanup(context.Context, PromotionResult, string, time.Time) error
}
type OutputObjects interface {
	ObjectStore
	Put(context.Context, string, []byte) error
	HasHash(context.Context, string, string) (bool, error)
}
type PublishPorts struct {
	Store    OutputInventory
	Objects  OutputObjects
	Terms    TermsProvider
	NewNonce func() (string, error)
	Clock    func() time.Time
}

// Publish repairs one immutable output. A receipt is selectable only after
// bounded bytes verify and the live writer revalidates consent atomically.
func Publish(ctx context.Context, p PublishPorts, r PromotionResult, body []byte) error {
	if p.Store == nil || p.Objects == nil || p.Terms == nil || p.NewNonce == nil || p.Clock == nil {
		return errors.New("training output: unconfigured")
	}
	sum := sha256.Sum256(body)
	if len(body) == 0 || len(body) > 3<<20 || hex.EncodeToString(sum[:]) != r.DerivativeSHA256 || r.ContributorID == "" || r.ObjectKey == "" || r.ObjectKey != TrainingObjectKey(r.ImageID, r.ConsentRevision, r.PromotedAt) {
		return domain.ErrInvalidRequest
	}
	state, err := p.Store.ValidateOutput(ctx, r, p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
	if err != nil {
		return err
	}
	if state == domain.OutputReady {
		inspect, cancel := context.WithTimeout(ctx, 5*time.Second)
		valid, err := p.Objects.HasHash(inspect, r.ObjectKey, r.DerivativeSHA256)
		cancel()
		if err != nil {
			return err
		}
		if valid {
			return nil
		}
	}
	nonce, err := p.NewNonce()
	if err != nil {
		return err
	}
	if err := p.Store.BeginOutput(ctx, r, nonce, p.Clock(), p.Terms.CurrentVersion(), p.Terms.CurrentDigest()); err != nil {
		return err
	}
	transfer, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = p.Objects.Put(transfer, r.ObjectKey, body)
	if err == nil {
		var valid bool
		valid, err = p.Objects.HasHash(transfer, r.ObjectKey, r.DerivativeSHA256)
		if err == nil && !valid {
			err = errors.New("training output: stored hash mismatch")
		}
	}
	cancel()
	if err == nil {
		ack, stop := context.WithTimeout(ctx, 3*time.Second)
		err = p.Store.FinishOutput(ack, r, nonce, true, p.Clock(), p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
		stop()
	}
	if err == nil {
		return nil
	}
	// Fence before deleting: a stale writer must never delete a newer
	// writer's output. Failed cleanup remains durable for reconciliation.
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer stop()
	if pinErr := p.Store.PrepareOutputCleanup(cleanup, r, nonce, p.Clock()); pinErr != nil {
		return errors.Join(err, pinErr)
	}
	if deleteErr := p.Objects.Delete(cleanup, r.ObjectKey); deleteErr != nil {
		return errors.Join(err, deleteErr)
	}
	exists, checkErr := p.Objects.Exists(cleanup, r.ObjectKey)
	if checkErr != nil {
		return errors.Join(err, checkErr)
	}
	if exists {
		return errors.Join(err, errors.New("training output: cleanup unconfirmed"))
	}
	finishErr := p.Store.FinishOutput(cleanup, r, nonce, false, p.Clock(), p.Terms.CurrentVersion(), p.Terms.CurrentDigest())
	return errors.Join(err, finishErr)
}
