package verify

import (
	"crypto/x509"
	"testing"
)

// P31-T01 independent declaration verification: exact content binding,
// chain building against injected roots, validity windows and explicit
// revocation knowledge. Unknown revocation stays indeterminate (never
// guessed); test roots never approve in production mode. Real
// ICP-Brasil/gov.br trust proof needs the provisioned environment —
// synthetic PKI proves logic only.

func TestValidChainBindsDeclaration(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: pki.roots,
		At: pki.validAt, Revocation: knownGood{},
	})
	if result.Outcome != OutcomeValid {
		t.Fatalf("result = %+v", result)
	}
	if result.Signer == "" {
		t.Fatal("valid result must name the signer")
	}
}

func TestTamperedDeclarationFails(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:tampered\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: pki.roots,
		At: pki.validAt, Revocation: knownGood{},
	})
	if result.Outcome != OutcomeInvalid {
		t.Fatalf("result = %+v", result)
	}
}

func TestExpiredChainFails(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: pki.roots,
		At: pki.validAt.AddDate(10, 0, 0), Revocation: knownGood{},
	})
	if result.Outcome != OutcomeInvalid {
		t.Fatalf("result = %+v", result)
	}
}

func TestRevokedChainFails(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: pki.roots,
		At: pki.validAt, Revocation: revokedSerial(pki.leafSerial),
	})
	if result.Outcome != OutcomeInvalid {
		t.Fatalf("result = %+v", result)
	}
}

func TestUnknownRevocationIsIndeterminate(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: pki.roots,
		At: pki.validAt, Revocation: unknownRevocation{},
	})
	if result.Outcome != OutcomeIndeterminate {
		t.Fatalf("result = %+v", result)
	}
}

func TestForgedChainFails(t *testing.T) {
	pki := testPKI(t)
	attacker := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), attacker.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{attacker.leafPEM, attacker.caPEM}, Roots: pki.roots,
		At: pki.validAt, Revocation: knownGood{},
	})
	if result.Outcome != OutcomeInvalid {
		t.Fatalf("result = %+v", result)
	}
}

func TestTestRootsNeverApproveInProduction(t *testing.T) {
	pki := testPKI(t)
	doc := append([]byte("SIGNED-DECLARATION:"+declarationText+"\n"), pki.leafPEM...)
	result := Verify(Input{
		Doc: doc, Declaration: declarationText, ExpectedDigest: digestOf(declarationText),
		CertPEMs: [][]byte{pki.leafPEM, pki.caPEM}, Roots: systemRoots(t),
		At: pki.validAt, Revocation: knownGood{}, Production: true,
	})
	if result.Outcome != OutcomeInvalid {
		t.Fatalf("result = %+v (test chain must not validate against system roots)", result)
	}
}

func TestMissingMaterialIsIndeterminate(t *testing.T) {
	result := Verify(Input{Doc: []byte("x"), Declaration: "y", ExpectedDigest: digestOf("y")})
	if result.Outcome != OutcomeIndeterminate {
		t.Fatalf("result = %+v", result)
	}
}

type knownGood struct{}

func (knownGood) Revoked(_ string) (revoked, known bool) { return false, true }

type unknownRevocation struct{}

func (unknownRevocation) Revoked(_ string) (revoked, known bool) { return false, false }

type revokedSerial string

func (s revokedSerial) Revoked(serial string) (revoked, known bool) {
	return serial == string(s), true
}

var _ = x509.ErrUnsupportedAlgorithm
