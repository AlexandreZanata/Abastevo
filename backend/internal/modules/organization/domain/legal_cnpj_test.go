package domain_test

import (
	"errors"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

// The all-zero CNPJ passes the shared check-digit arithmetic but identifies no
// company; legal declarations and establishments refuse it (the importer
// kernel keeps its own registry rules).
func TestLegalDeclarationsRefuseTheAllZeroCNPJ(t *testing.T) {
	scope := legalScope(t, 301)
	zero := d.LegalDeclarationInput{Kind: d.Company, LegalName: "Synthetic zero company", Identifier: "00.000.000/0000-00", Establishment: d.EstablishmentInput{Role: d.NotInformed}}
	if _, err := d.NewLegalDeclaration(legalID(1101), zero); !errors.Is(err, d.ErrInvalidCNPJ) {
		t.Fatal("all-zero company declared", err)
	}
	command, err := d.NewLegalDeclaration(legalID(1001), companyLegalInput())
	if err != nil {
		t.Fatal(err)
	}
	entity, _, err := command.Create(scope, legalID(401), legalID(501))
	if err != nil {
		t.Fatal(err)
	}
	original, err := entity.Snapshot(scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.NewEstablishment(scope, legalID(502), entity, d.EstablishmentInput{CNPJ: "00000000000000", Role: d.Branch}); !errors.Is(err, d.ErrInvalidCNPJ) {
		t.Fatal("all-zero establishment accepted", err)
	}
	_ = original
	// Only the full all-zero number is refused: root 00000000 is a real
	// registration (00.000.000/0001-91) and declares, stores and rehydrates.
	real := d.LegalDeclarationInput{Kind: d.Company, LegalName: "Synthetic zero-root company", Identifier: "00.000.000/0001-91", Establishment: d.EstablishmentInput{Role: d.HeadOffice}}
	declared, err := d.NewLegalDeclaration(legalID(1102), real)
	if err != nil {
		t.Fatal("zero-root CNPJ refused", err)
	}
	rooted, _, err := declared.Create(scope, legalID(403), legalID(503))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := rooted.Snapshot(scope)
	if err != nil || snapshot.Identifier != "00000000" {
		t.Fatal("zero-root snapshot", snapshot.Identifier, err)
	}
	if _, err := d.RehydrateLegalEntity(scope, snapshot); err != nil {
		t.Fatal("stored zero root refused", err)
	}
}
