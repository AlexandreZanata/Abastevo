package authority

import (
	"testing"
	"time"
)

// P31-T02 corporate authority verification: applicant linkage and
// sufficient powers for the exact operating branch. Company existence,
// signer identity and authority stay separately supported; name-only,
// brand/root, custodian and shareholder shortcuts deny. Fixtures are
// synthetic; Receita/QSA/corporate-act access stays OPEN.

func TestAdministratorWithBranchCoverageApproves(t *testing.T) {
	facts := companyFacts()
	result := Verify(AuthorityQuery{
		Applicant: Person{Document: "12345678901", Name: "Ana Administradora"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "administrator"},
		Company:   facts,
		At:        now(),
	})
	if result.Outcome != OutcomeSufficient {
		t.Fatalf("result = %+v", result)
	}
}

func TestCustodianShareholderAndNameOnlyDeny(t *testing.T) {
	facts := companyFacts()
	custodian := AuthorityQuery{
		Applicant: Person{Document: "99999999999", Name: "Contador Custódio"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "administrator"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(custodian); got.Outcome != OutcomeDenied {
		t.Fatalf("custodian = %+v (e-CNPJ holder without powers)", got)
	}
	shareholder := AuthorityQuery{
		Applicant: Person{Document: "88888888888", Name: "Sócio Sem Poderes"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "manager"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(shareholder); got.Outcome != OutcomeDenied {
		t.Fatalf("shareholder = %+v", got)
	}
	homonym := AuthorityQuery{
		Applicant: Person{Document: "", Name: "Ana Administradora"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "administrator"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(homonym); got.Outcome != OutcomeDenied {
		t.Fatalf("homonym = %+v (name-only match never resolves identity)", got)
	}
}

func TestBranchMismatchExpiredMandateAndJointSignature(t *testing.T) {
	facts := companyFacts()
	mismatch := AuthorityQuery{
		Applicant: Person{Document: "12345678901", Name: "Ana Administradora"},
		Claim:     BranchClaim{CNPJ: "00428184000195", Role: "administrator"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(mismatch); got.Outcome != OutcomeDenied {
		t.Fatalf("mismatch = %+v (powers cover another branch)", got)
	}
	expired := AuthorityQuery{
		Applicant: Person{Document: "77777777777", Name: "Gerente Vencido"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "manager"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(expired); got.Outcome != OutcomeDenied {
		t.Fatalf("expired = %+v", got)
	}
	joint := AuthorityQuery{
		Applicant: Person{Document: "66666666666", Name: "Sócio Conjunto"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "administrator"},
		Company:   facts,
		At:        now(),
	}
	if got := Verify(joint); got.Outcome != OutcomeInsufficient {
		t.Fatalf("joint = %+v (missing co-signer routes to review, never auto-approval)", got)
	}
}

func TestUnknownCompanyDefers(t *testing.T) {
	query := AuthorityQuery{
		Applicant: Person{Document: "12345678901", Name: "Ana"},
		Claim:     BranchClaim{CNPJ: "04218406000104", Role: "administrator"},
		Company:   CompanyFacts{},
		At:        now(),
	}
	if got := Verify(query); got.Outcome != OutcomeInsufficient {
		t.Fatalf("unknown = %+v", got)
	}
}

func now() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }

func companyFacts() CompanyFacts {
	return CompanyFacts{
		Branches: map[string]bool{"04218406000104": true, "00428184000195": true},
		People: []PersonAuthority{
			{Document: "12345678901", Name: "Ana Administradora", Role: "administrator", Powers: []string{"manage"}, Branches: []string{"04218406000104"}, MandateUntil: now().AddDate(1, 0, 0)},
			{Document: "77777777777", Name: "Gerente Vencido", Role: "manager", Powers: []string{"manage"}, Branches: []string{"04218406000104"}, MandateUntil: now().AddDate(0, 0, -1)},
			{Document: "66666666666", Name: "Sócio Conjunto", Role: "administrator", Powers: []string{"manage"}, Branches: []string{"04218406000104"}, MandateUntil: now().AddDate(1, 0, 0), JointWith: "12345678901"},
			{Document: "88888888888", Name: "Sócio Sem Poderes", Role: "shareholder", Powers: nil, Branches: nil},
			{Document: "99999999999", Name: "Contador Custódio", Role: "custodian", Powers: nil, Branches: nil},
		},
	}
}
