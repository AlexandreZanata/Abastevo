package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func legalID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }
func legalScope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(legalID(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Generate synthetic values; never copy a person's identifier. The equivalent
// ten-times/remainder formulation differs from the parser's complement formula.
func generatedLegalCPF(n int) string {
	value := fmt.Sprintf("%09d", 341572000+n)
	for len(value) < 11 {
		sum := 0
		for i, ch := range value {
			sum += int(ch-'0') * (len(value) + 1 - i)
		}
		digit := (sum * 10) % 11
		if digit == 10 {
			digit = 0
		}
		value += fmt.Sprint(digit)
	}
	return value
}
func TestCPFGeneratedValuesAndMalformedInput(t *testing.T) {
	for n := range 1000 {
		raw := generatedLegalCPF(n)
		value, err := d.ParseCPF(raw)
		if err != nil || value.ForStorage() != raw {
			t.Fatal("generated CPF validation", n, err)
		}
		formatted := raw[:3] + "." + raw[3:6] + "." + raw[6:9] + "-" + raw[9:]
		other, err := d.ParseCPF(formatted)
		if err != nil || other.ForStorage() != raw {
			t.Fatal("format normalization", n)
		}
		mutated := raw[:10] + string(byte('0'+(raw[10]-'0'+1)%10))
		if _, err := d.ParseCPF(mutated); !errors.Is(err, d.ErrInvalidCPF) {
			t.Fatal("bad check digit accepted", n)
		}
		if value.Masked() != "***.***.***-"+raw[9:] {
			t.Fatal("unsafe mask")
		}
	}
	for _, raw := range []string{"", "short", "１２３４５６７８９００", strings.Repeat("0", 11), strings.Repeat("9", 11), generatedLegalCPF(1) + "1", generatedLegalCPF(1) + "\n", strings.Replace(generatedLegalCPF(1), "3", "A", 1)} {
		if _, err := d.ParseCPF(raw); !errors.Is(err, d.ErrInvalidCPF) {
			t.Fatal("malformed CPF accepted")
		}
	}
}
func companyLegalInput() d.LegalDeclarationInput {
	return d.LegalDeclarationInput{Kind: d.Company, LegalName: " Synthetic company legal name ", Identifier: "ex.emp.l01/0001-70", Establishment: d.EstablishmentInput{CNPJ: "EXEMPL01000170", Role: d.NotInformed, TradeName: " Synthetic trade name ", MunicipalityCode: "5103403"}}
}
func TestLegalDeclarationAndExplicitEstablishmentRole(t *testing.T) {
	scope := legalScope(t, 301)
	input := companyLegalInput()
	command, err := d.NewLegalDeclaration(legalID(1001), input)
	if err != nil {
		t.Fatal(err)
	}
	entity, first, err := command.Create(scope, legalID(401), legalID(501))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := entity.Snapshot(scope)
	if err != nil || snapshot.Kind != d.Company || snapshot.Identifier != "EXEMPL01" || snapshot.State != d.LegalDeclared || snapshot.Revision != 1 || snapshot.LegalName != "Synthetic company legal name" {
		t.Fatal("incorrect declaration", err)
	}
	establishment, err := first.Snapshot(scope)
	if err != nil || establishment.Role != d.NotInformed || establishment.EntityID != snapshot.ID || establishment.CNPJ != "EXEMPL01000170" {
		t.Fatal("inferred head office", err)
	}
	input.Establishment.Role = d.Branch
	branch, err := d.NewLegalDeclaration(legalID(1001), input)
	if err != nil || branch.Digest(scope) == command.Digest(scope) {
		t.Fatal("role absent from intent", err)
	}
	if _, err := entity.Snapshot(legalScope(t, 302)); !errors.Is(err, d.ErrScope) {
		t.Fatal("foreign entity exposed")
	}
	if _, err := first.Snapshot(legalScope(t, 302)); !errors.Is(err, d.ErrScope) {
		t.Fatal("foreign establishment exposed")
	}
	if _, err := d.NewEstablishment(scope, legalID(502), entity, d.EstablishmentInput{CNPJ: "EXEMPL02000114", Role: d.HeadOffice}); !errors.Is(err, d.ErrInvalidCNPJ) {
		t.Fatal("different root accepted", err)
	}
}
func TestNaturalPartyRequiresPurposeAndRedactsValues(t *testing.T) {
	scope := legalScope(t, 301)
	raw := generatedLegalCPF(7)
	input := d.LegalDeclarationInput{Kind: d.NaturalPerson, LegalName: "Synthetic private party", Identifier: raw, Purpose: d.VerificationAndAgreements}
	command, err := d.NewLegalDeclaration(legalID(1001), input)
	if err != nil {
		t.Fatal(err)
	}
	entity, establishment, err := command.Create(scope, legalID(401), "")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := entity.Snapshot(scope)
	if err != nil || snapshot.Identifier != raw || snapshot.State != d.LegalDeclared || snapshot.Purpose != d.VerificationAndAgreements {
		t.Fatal("missing natural declaration", err)
	}
	if _, err := establishment.Snapshot(scope); err == nil {
		t.Fatal("fabricated establishment")
	}
	if entity.MaskedIdentifier(scope) == raw {
		t.Fatal("CPF exposed")
	}
	cpf, err := d.ParseCPF(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{cpf, command, entity, snapshot} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			text := fmt.Sprintf(verb, v)
			if strings.Contains(text, raw) || strings.Contains(text, "Synthetic private party") {
				t.Fatal("private formatting leaked")
			}
		}
		encoded, err := json.Marshal(map[string]any{"v": v})
		if err != nil || strings.Contains(string(encoded), raw) {
			t.Fatal("private encoding leaked", err)
		}
	}
	input.Purpose = ""
	if _, err := d.NewLegalDeclaration(legalID(1001), input); err == nil {
		t.Fatal("CPF without purpose")
	}
}
func TestLegalInvalidInputAndWithdrawal(t *testing.T) {
	for _, change := range []func(*d.LegalDeclarationInput){func(v *d.LegalDeclarationInput) { v.Kind = "VERIFIED" }, func(v *d.LegalDeclarationInput) { v.Identifier = "bad" }, func(v *d.LegalDeclarationInput) { v.LegalName = " " }, func(v *d.LegalDeclarationInput) { v.LegalName = "unsafe\nname" }, func(v *d.LegalDeclarationInput) { v.Establishment.Role = "INFER_FROM_CNPJ" }, func(v *d.LegalDeclarationInput) { v.Establishment.MunicipalityCode = "bad-city" }, func(v *d.LegalDeclarationInput) { v.Establishment.TradeName = strings.Repeat("a", 121) }, func(v *d.LegalDeclarationInput) { v.Establishment.CNPJ = "EXEMPL02000114" }} {
		input := companyLegalInput()
		change(&input)
		if _, err := d.NewLegalDeclaration(legalID(1001), input); err == nil {
			t.Fatal("invalid declaration accepted")
		}
	}
	scope := legalScope(t, 301)
	command, err := d.NewLegalDeclaration(legalID(1001), companyLegalInput())
	if err != nil {
		t.Fatal(err)
	}
	entity, _, err := command.Create(scope, legalID(401), legalID(501))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := command.Create(scope, "bad", legalID(501)); err == nil {
		t.Fatal("bad generated identity")
	}
	removed, err := entity.Withdraw(scope, 1, d.LegalReferences{})
	if err != nil || !removed.Remove {
		t.Fatal("unreferenced party not removable", err)
	}
	retained, err := entity.Withdraw(scope, 1, d.LegalReferences{Current: 1})
	if err != nil || retained.Remove {
		t.Fatal("referenced party removed", err)
	}
	snapshot, err := retained.Entity.Snapshot(scope)
	if err != nil || snapshot.State != d.LegalRevoked || snapshot.Revision != 2 {
		t.Fatal("withdrawal did not revoke", err)
	}
	if _, err := entity.Withdraw(scope, 2, d.LegalReferences{}); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale withdrawal admitted", err)
	}
	if _, err := entity.Withdraw(scope, 1, d.LegalReferences{Current: -1}); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("negative reference count")
	}
	snapshot.State = "VERIFIED"
	if _, err := d.RehydrateLegalEntity(scope, snapshot); err == nil {
		t.Fatal("unproven verification accepted")
	}
}

func TestLegalRehydrationCannotNormalizeCorruptStorageOrCrossScopes(t *testing.T) {
	scope := legalScope(t, 301)
	command, err := d.NewLegalDeclaration(legalID(1001), companyLegalInput())
	if err != nil {
		t.Fatal(err)
	}
	entity, first, err := command.Create(scope, legalID(401), legalID(501))
	if err != nil {
		t.Fatal(err)
	}
	original, err := entity.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*d.LegalEntitySnapshot){func(v *d.LegalEntitySnapshot) { v.ID = "bad" }, func(v *d.LegalEntitySnapshot) { v.OrgID = kernel.OrgID{} }, func(v *d.LegalEntitySnapshot) { v.Revision = 0 }, func(v *d.LegalEntitySnapshot) { v.Kind = "unknown" }, func(v *d.LegalEntitySnapshot) { v.LegalName = " bad padding " }, func(v *d.LegalEntitySnapshot) { v.Identifier = "short" }, func(v *d.LegalEntitySnapshot) { v.Identifier = "lowerxxx" }, func(v *d.LegalEntitySnapshot) { v.Purpose = d.VerificationAndAgreements }, func(v *d.LegalEntitySnapshot) { v.Kind = d.NaturalPerson; v.Identifier = "bad" }} {
		v := original
		change(&v)
		if _, err := d.RehydrateLegalEntity(scope, v); err == nil {
			t.Fatal("corrupt legal snapshot accepted")
		}
	}
	snap, err := first.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*d.EstablishmentSnapshot){func(v *d.EstablishmentSnapshot) { v.ID = "bad" }, func(v *d.EstablishmentSnapshot) { v.EntityID = "bad" }, func(v *d.EstablishmentSnapshot) { v.ID = v.EntityID }, func(v *d.EstablishmentSnapshot) { v.Revision = 0 }, func(v *d.EstablishmentSnapshot) { v.Role = "" }, func(v *d.EstablishmentSnapshot) { v.CNPJ = strings.ToLower(v.CNPJ) }, func(v *d.EstablishmentSnapshot) { v.TradeName = " padding " }, func(v *d.EstablishmentSnapshot) { v.MunicipalityCode = "0000000" }, func(v *d.EstablishmentSnapshot) { v.MunicipalityCode = "510" }} {
		v := snap
		change(&v)
		if _, err := d.RehydrateEstablishment(scope, v); err == nil {
			t.Fatal("corrupt establishment snapshot accepted")
		}
	}
	if _, err := entity.Snapshot(kernel.OrgScope{}); err == nil {
		t.Fatal("zero scope accepted")
	}
	if _, _, err := (d.LegalDeclaration{}).Create(scope, legalID(401), legalID(501)); err == nil {
		t.Fatal("zero declaration accepted")
	}
	if _, err := d.NewLegalDeclaration("bad", companyLegalInput()); err == nil {
		t.Fatal("bad operation accepted")
	}
	if got := entity.MaskedIdentifier(legalScope(t, 302)); got != "" {
		t.Fatal("foreign mask exposed")
	}
	if got := entity.MaskedIdentifier(scope); got != "EXEMPL01" {
		t.Fatal("company root missing")
	}
	history, err := entity.Withdraw(scope, 1, d.LegalReferences{VerificationHistory: true})
	if err != nil || history.Remove {
		t.Fatal("verification history removed", err)
	}
	if _, err := history.Entity.Withdraw(scope, 2, d.LegalReferences{}); err != d.ErrTransition {
		t.Fatal("revoked declaration withdrawn again", err)
	}
	if _, err := d.NewEstablishment(scope, legalID(502), history.Entity, d.EstablishmentInput{CNPJ: "EXEMPL01000170"}); err != d.ErrTransition {
		t.Fatal("revoked declaration accepts new establishment", err)
	}
	original.Revision = 9223372036854775807
	exhausted, err := d.RehydrateLegalEntity(scope, original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exhausted.Withdraw(scope, original.Revision, d.LegalReferences{}); err != d.ErrRevision {
		t.Fatal("revision overflow", err)
	}
	input := companyLegalInput()
	input.Purpose = d.VerificationAndAgreements
	if _, err := d.NewLegalDeclaration(legalID(1001), input); err == nil {
		t.Fatal("company accepted CPF purpose")
	}
}

func TestEstablishmentMetadataKeepsIdentifiersAndChecksRevision(t *testing.T) {
	scope := legalScope(t, 301)
	command, err := d.NewLegalDeclaration(legalID(1001), companyLegalInput())
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := command.Create(scope, legalID(401), legalID(501))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := first.ChangeMetadata(scope, 1, d.EstablishmentMetadata{Role: d.HeadOffice, TradeName: " Updated synthetic trade ", MunicipalityCode: "3550308"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := first.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	after, err := changed.Snapshot(scope)
	if err != nil || before.CNPJ != after.CNPJ || before.EntityID != after.EntityID || before.ID != after.ID || after.Revision != 2 || after.Role != d.HeadOffice || after.TradeName != "Updated synthetic trade" {
		t.Fatal("metadata changed identity", err)
	}
	if _, err := changed.ChangeMetadata(scope, 1, d.EstablishmentMetadata{}); err != d.ErrConflict {
		t.Fatal("stale metadata accepted", err)
	}
	for _, m := range []d.EstablishmentMetadata{{Role: "UNKNOWN"}, {TradeName: "unsafe\ntrade"}, {MunicipalityCode: "510"}, {MunicipalityCode: "0000000"}, {MunicipalityCode: "51x3403"}} {
		if _, err := first.ChangeMetadata(scope, 1, m); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	if _, err := first.ChangeMetadata(legalScope(t, 302), 1, d.EstablishmentMetadata{}); err != d.ErrScope {
		t.Fatal("foreign metadata change", err)
	}
}
