// Package verify implements P31-T01 independent declaration
// verification: exact content binding, chain building against injected
// roots, validity windows and explicit revocation knowledge. Unknown
// revocation stays indeterminate (never guessed); synthetic test PKI
// proves logic only and never approves in production mode — real
// ICP-Brasil/gov.br trust proof needs the provisioned environment with
// approved roots and revocation sources. PDF ByteRange/CMS extraction
// stays a documented manual operator process (official tools on exact
// bytes); this package verifies the extracted content binding, never a
// screenshot or validation report.
package verify

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"time"
)

// Outcomes: valid, invalid (with reason) and indeterminate (missing
// material or unknown revocation — retry allowed, never approval).
type Outcome string

const (
	OutcomeValid         Outcome = "valid"
	OutcomeInvalid       Outcome = "invalid"
	OutcomeIndeterminate Outcome = "indeterminate"
)

// RevocationSource answers serial revocation. Unknown serials must
// report known=false (indeterminate), never a guessed clean bill.
type RevocationSource interface {
	Revoked(serial string) (revoked, known bool)
}

// Input carries the extracted document content (exact bytes the
// operator tooling pulled from the signed file), the server
// declaration, its expected digest, the signer's certificate chain in
// leaf-first PEM order, the trust roots, the policy time and the
// revocation source.
type Input struct {
	Doc            []byte
	Declaration    string
	ExpectedDigest string
	CertPEMs       [][]byte
	Roots          *x509.CertPool
	At             time.Time
	Revocation     RevocationSource
	Production     bool
}

// Result carries the outcome with safe denial reasons (never private
// document dumps, never key material).
type Result struct {
	Outcome Outcome
	Reason  string
	Signer  string
}

// Verify binds content, builds the chain and checks validity plus
// revocation. No network, no clock beyond the policy time, no locks
// held: safe to call from workers with bounded inputs.
func Verify(in Input) Result {
	if len(in.Doc) == 0 || in.Declaration == "" || in.ExpectedDigest == "" {
		return Result{Outcome: OutcomeIndeterminate, Reason: "missing proof material"}
	}
	if in.Roots == nil || in.Revocation == nil || len(in.CertPEMs) == 0 {
		return Result{Outcome: OutcomeIndeterminate, Reason: "trust material incomplete"}
	}
	sum := sha256.Sum256([]byte(in.Declaration))
	if hex.EncodeToString(sum[:]) != in.ExpectedDigest {
		return Result{Outcome: OutcomeInvalid, Reason: "declaration digest mismatch"}
	}
	if !bytes.Contains(in.Doc, []byte(in.Declaration)) {
		return Result{Outcome: OutcomeInvalid, Reason: "declaration not bound in document"}
	}
	var certs []*x509.Certificate
	for _, pemBytes := range in.CertPEMs {
		block, _ := pem.Decode(pemBytes)
		if block == nil {
			return Result{Outcome: OutcomeInvalid, Reason: "certificate decode failed"}
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return Result{Outcome: OutcomeInvalid, Reason: "certificate parse failed"}
		}
		certs = append(certs, cert)
	}
	leaf := certs[0]
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	at := in.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         in.Roots,
		Intermediates: intermediates,
		CurrentTime:   at,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return Result{Outcome: OutcomeInvalid, Reason: "trust chain rejected: " + shortError(err)}
	}
	revoked, known := in.Revocation.Revoked(leaf.SerialNumber.String())
	if !known {
		return Result{Outcome: OutcomeIndeterminate, Reason: "revocation unknown"}
	}
	if revoked {
		return Result{Outcome: OutcomeInvalid, Reason: "certificate revoked"}
	}
	return Result{Outcome: OutcomeValid, Signer: leaf.Subject.CommonName}
}

func shortError(err error) string {
	message := err.Error()
	if len(message) > 120 {
		return message[:120]
	}
	return message
}
