package application

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeCorrectionLocationBounds(t *testing.T) {
	good := json.RawMessage(`{"latitude":-3.119,"longitude":-60.0217}`)
	if _, _, err := NormalizeCorrection(CorrectionFieldLocation, good, ""); err != nil {
		t.Fatalf("valid point: %v", err)
	}
	for _, raw := range []string{
		`{"latitude":0,"longitude":0}`,
		`{"latitude":91,"longitude":-60}`,
		`{"latitude":-3,"longitude":10}`,
		`{"latitude":-3,"longitude":-60,"extra":1}`,
		`{"latitude":"-3","longitude":-60}`,
	} {
		if _, _, err := NormalizeCorrection(CorrectionFieldLocation, json.RawMessage(raw), ""); err == nil {
			t.Fatalf("accepted invalid location %s", raw)
		}
	}
}

func TestNormalizeCorrectionTextAndAddress(t *testing.T) {
	if _, _, err := NormalizeCorrection(CorrectionFieldDisplayName, json.RawMessage(`{"value":"  Posto Alfa  "}`), ""); err != nil {
		t.Fatalf("valid name: %v", err)
	}
	if _, _, err := NormalizeCorrection(CorrectionFieldDisplayName, json.RawMessage(`{"value":"<script>"}`), ""); err == nil {
		t.Fatal("HTML in a text field must fail")
	}
	addr := json.RawMessage(`{"street":"Rua A","number":"100","district":"Centro","postal_code":"69980000","municipality_code":"3550308","state":"SP"}`)
	value, _, err := NormalizeCorrection(CorrectionFieldAddress, addr, "")
	if err != nil || value.PostalCode != "69980000" || value.State != "SP" {
		t.Fatalf("address = %+v, err = %v", value, err)
	}
	if _, _, err := NormalizeCorrection(CorrectionFieldAddress, json.RawMessage(`{"street":"Rua A","postal_code":"123"}`), ""); err == nil {
		t.Fatal("short postal code must fail")
	}
}

func TestNormalizeCorrectionRejectsPIIAndUnknownFields(t *testing.T) {
	good := json.RawMessage(`{"value":"Posto Alfa"}`)
	for _, note := range []string{
		"meu email a@b.com",
		"veja https://example.test",
		"CPF 123.456.789-09",
		strings.Repeat("a", MaxCorrectionNote+1),
	} {
		if _, _, err := NormalizeCorrection(CorrectionFieldDisplayName, good, note); err == nil {
			t.Fatalf("accepted PII/oversize note %q", note)
		}
	}
	if _, _, err := NormalizeCorrection(CorrectionFieldOperatingStatus, json.RawMessage(`{"status":"deleted"}`), ""); err == nil {
		t.Fatal("unknown operating status must fail")
	}
	if _, _, err := NormalizeCorrection("unknown", good, ""); err == nil {
		t.Fatal("unknown field must fail")
	}
}
