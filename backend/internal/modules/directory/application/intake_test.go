package application

import (
	"testing"
)

// P27-T01 private suggestion proposal rules: structured input with
// bounded fields, optional kernel-validated CNPJ and bounded optional
// coordinates. Anonymous contributor proof alone is insufficient (the
// handler requires a live account session); knowing a public CNPJ
// grants nothing.

func TestProposalRequiresDisplayAndLocation(t *testing.T) {
	if _, err := ParseProposal(ProposalInput{DisplayName: "  ", MunicipalityCode: "3550308", State: "SP"}); err == nil {
		t.Fatal("blank display must fail")
	}
	if _, err := ParseProposal(ProposalInput{DisplayName: "Posto", MunicipalityCode: "35503", State: "SP"}); err == nil {
		t.Fatal("short IBGE must fail")
	}
	if _, err := ParseProposal(ProposalInput{DisplayName: "Posto", MunicipalityCode: "3550308", State: "SPP"}); err == nil {
		t.Fatal("long UF must fail")
	}
}

func TestProposalAcceptsValidCNPJAndCoords(t *testing.T) {
	p, err := ParseProposal(ProposalInput{
		DisplayName: "Posto Alfa", MunicipalityCode: "3550308", State: "SP",
		CNPJ: "12ABC34501DE35", Latitude: -23.55, Longitude: -46.63, HasCoords: true,
	})
	if err != nil {
		t.Fatalf("valid proposal: %v", err)
	}
	if p.CNPJ != "12ABC34501DE35" {
		t.Fatalf("cnpj = %q", p.CNPJ)
	}
}

func TestProposalRejectsBadCNPJCoordsAndEvidence(t *testing.T) {
	base := ProposalInput{DisplayName: "Posto", MunicipalityCode: "3550308", State: "SP"}
	if _, err := ParseProposal(withCNPJ(base, "123")); err == nil {
		t.Fatal("short CNPJ must fail")
	}
	if _, err := ParseProposal(withCoords(base, 91, -46.63)); err == nil {
		t.Fatal("latitude out of range must fail")
	}
	if _, err := ParseProposal(withCoords(base, 0, 0)); err == nil {
		t.Fatal("null-island coordinates must fail")
	}
	long := make([]byte, MaxEvidenceRef+1)
	for i := range long {
		long[i] = 'a'
	}
	in := base
	in.EvidenceRef = string(long)
	if _, err := ParseProposal(in); err == nil {
		t.Fatal("oversize evidence ref must fail")
	}
}

func withCNPJ(in ProposalInput, cnpj string) ProposalInput { in.CNPJ = cnpj; return in }

func withCoords(in ProposalInput, lat, lon float64) ProposalInput {
	in.Latitude, in.Longitude, in.HasCoords = lat, lon, true
	return in
}
