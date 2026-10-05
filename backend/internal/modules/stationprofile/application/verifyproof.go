package application

import (
	"context"
	"crypto/x509"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
)

// VerifyPorts isolates proof verification: trust roots, revocation
// knowledge, object bytes and certificate extraction all arrive as
// ports. Missing trust material resolves indeterminate (retry
// allowed), never approval. External validation never holds a DB
// lock: bytes and chains resolve before any state write.
type VerifyPorts struct {
	Proofs  ProofStore
	Claims  ClaimStore
	Bytes   ProofBytesReader
	Roots   func() (*x509.CertPool, error)
	Revoker verify.RevocationSource
	Clock   func() time.Time
	Certs   func(ctx context.Context, proofID string) ([][]byte, error)
}

// ProofBytesReader fetches immutable proof objects by server key.
type ProofBytesReader interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

var ErrVerifyProofClosed = errors.New("stationprofile: proof is no longer verifiable")

// VerifyProof evaluates one received proof against its active
// declaration: exact content binding, chain building, validity and
// revocation. Terminal results (valid/invalid) consume the declaration
// and stamp the proof; indeterminate bumps attempts without
// consuming, expiring the declaration only at the frozen attempt cap.
// Attempts never reset: replays cannot manufacture fresh tries.
func VerifyProof(ctx context.Context, ports VerifyPorts, proofID string) (verify.Result, error) {
	proof, err := ports.Proofs.GetProof(ctx, proofID)
	if err != nil {
		return verify.Result{}, err
	}
	if proof.Status != "received" {
		return verify.Result{}, ErrVerifyProofClosed
	}
	claim, err := ports.Claims.Claim(ctx, proof.ClaimID)
	if err != nil {
		return verify.Result{}, err
	}
	if !domain.ClaimOpen(claim.State) {
		return verify.Result{}, ErrVerifyProofClosed
	}
	declaration, err := ports.Claims.GetDeclaration(ctx, proof.DeclarationID)
	if err != nil {
		return verify.Result{}, ErrVerifyProofClosed
	}
	if declaration.State != "active" || declaration.ClaimID != claim.ID {
		return verify.Result{}, ErrVerifyProofClosed
	}
	if ports.Clock().After(declaration.ExpiresAt) {
		return expireForExhaustion(ctx, ports, proof, declaration, "declaration expired")
	}
	doc, err := ports.Bytes.Get(ctx, proof.ObjectKey)
	if err != nil {
		return verify.Result{}, err
	}
	certs, err := ports.Certs(ctx, proof.ID)
	if err != nil {
		return verify.Result{}, err
	}
	roots, err := ports.Roots()
	if err != nil || roots == nil {
		return bumpIndeterminate(ctx, ports, proof, declaration, "trust roots unavailable")
	}
	result := verify.Verify(verify.Input{
		Doc: doc, Declaration: declaration.Declaration,
		ExpectedDigest: declaration.ExpectedDigest, CertPEMs: certs,
		Roots: roots, At: ports.Clock(), Revocation: ports.Revoker,
	})
	switch result.Outcome {
	case verify.OutcomeValid:
		if err := ports.Proofs.ConsumeDeclaration(ctx, declaration.ID); err != nil {
			return verify.Result{}, err
		}
		if err := ports.Proofs.SetProofStatus(ctx, proof.ID, "verified"); err != nil {
			return verify.Result{}, err
		}
		return result, nil
	case verify.OutcomeInvalid:
		if err := ports.Proofs.ConsumeDeclaration(ctx, declaration.ID); err != nil {
			return verify.Result{}, err
		}
		if err := ports.Proofs.SetProofStatus(ctx, proof.ID, "rejected"); err != nil {
			return verify.Result{}, err
		}
		return result, nil
	default:
		return bumpIndeterminate(ctx, ports, proof, declaration, result.Reason)
	}
}

func bumpIndeterminate(ctx context.Context, ports VerifyPorts, proof ProofRow, declaration DeclarationRow, reason string) (verify.Result, error) {
	attempts, err := ports.Proofs.BumpAttempts(ctx, declaration.ID)
	if err != nil {
		return verify.Result{}, err
	}
	if attempts > int64(domain.MaxProofAttempts) {
		return expireForExhaustion(ctx, ports, proof, declaration, "attempt budget exhausted")
	}
	return verify.Result{Outcome: verify.OutcomeIndeterminate, Reason: reason}, nil
}

func expireForExhaustion(ctx context.Context, ports VerifyPorts, proof ProofRow, declaration DeclarationRow, reason string) (verify.Result, error) {
	if err := ports.Proofs.ExpireDeclaration(ctx, declaration.ID); err != nil {
		return verify.Result{}, err
	}
	if err := ports.Proofs.SetProofStatus(ctx, proof.ID, "rejected"); err != nil {
		return verify.Result{}, err
	}
	return verify.Result{Outcome: verify.OutcomeInvalid, Reason: reason}, nil
}
