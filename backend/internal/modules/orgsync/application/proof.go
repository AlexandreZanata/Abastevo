package application

import (
	"context"
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// AtomicReview is a source-owned same-transaction review obligation. It never
// treats a missing consumer or a review identifier alone as a real held fact.
const AtomicReview FollowupMode = "ATOMIC_REVIEW"

type ReviewProof interface {
	CheckHeldTx(context.Context, DBTX, Source, protocol.Operation, Outcome) error
	CheckOwnReceiptTx(context.Context, DBTX, Source, protocol.Operation) (bool, error)
}

type ReviewProofProvider interface{ ReviewProof() ReviewProof }

type borrowedReviewProof struct {
	privateValue
	source ReviewProof
}

func (proof borrowedReviewProof) CheckHeldTx(ctx context.Context, db DBTX, source Source, op protocol.Operation, outcome Outcome) error {
	return proof.source.CheckHeldTx(ctx, Borrow(db), source, op, outcome.Copy())
}
func (proof borrowedReviewProof) CheckOwnReceiptTx(ctx context.Context, db DBTX, source Source, op protocol.Operation) (bool, error) {
	return proof.source.CheckOwnReceiptTx(ctx, Borrow(db), source, op)
}

func captureReviewProof(handler Handler, descriptor Descriptor) (ReviewProof, error) {
	if !slices.Contains(descriptor.Results(), protocol.AcceptedForReview) {
		return nil, nil
	}
	provider, ok := handler.(ReviewProofProvider)
	if !ok || nilPort(provider) {
		return nil, ErrUnavailable
	}
	proof := provider.ReviewProof()
	if nilPort(proof) {
		return nil, ErrUnavailable
	}
	return borrowedReviewProof{source: proof}, nil
}

func (handler registeredHandler) ReviewProof() ReviewProof { return handler.proof }
