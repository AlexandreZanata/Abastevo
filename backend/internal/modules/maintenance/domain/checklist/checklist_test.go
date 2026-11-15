package checklist_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
)

const (
	categoryA = "9c000000-0000-4000-8000-000000001001"
	categoryB = "9c000000-0000-4000-8000-000000001002"
	assetA    = "9c000000-0000-4000-8000-000000002001"
)

func TestClosedVocabulariesAndDefaults(t *testing.T) {
	for _, raw := range []string{"DEPARTURE", "RETURN", "AD_HOC"} {
		if c, err := checklist.ParseContext(raw); err != nil || string(c) != raw {
			t.Fatal("context", raw, err)
		}
	}
	for _, raw := range []string{"ORGANIZATION", "CATEGORIES", "ASSETS"} {
		if k, err := checklist.ParseTargetKind(raw); err != nil || string(k) != raw {
			t.Fatal("target kind", raw, err)
		}
	}
	for _, raw := range []string{"BLOCK", "WARN"} {
		if m, err := checklist.ParseMode(raw); err != nil || string(m) != raw {
			t.Fatal("mode", raw, err)
		}
	}
	for _, raw := range []string{"", "departure", " DEPARTURE", "PARKING"} {
		if _, err := checklist.ParseContext(raw); !errors.Is(err, checklist.ErrInvalid) {
			t.Fatal("context accepted", raw)
		}
	}
	if _, err := checklist.ParseTargetKind("UNITS"); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("target kind accepted")
	}
	if _, err := checklist.ParseMode("ALLOW"); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("mode accepted")
	}
	p := checklist.DefaultPolicy()
	if p.Mode != checklist.Block || p.ValiditySeconds != checklist.DefaultValiditySeconds || checklist.DefaultValiditySeconds != 12*3600 || p.Validate() != nil {
		t.Fatal("default policy", p)
	}
	if (checklist.Policy{}).Validate() == nil {
		t.Fatal("zero policy valid")
	}
}

func TestDefinitionIsACanonicalIndependentCopy(t *testing.T) {
	ids := []string{categoryB, categoryA}
	list := []checklist.Item{{Label: "Pneus", Critical: true, PhotoOnNotOK: true}, {Label: "Estepe", Optional: true}}
	in := checklist.Definition{Name: "Saída diária", Context: checklist.Departure, Target: checklist.Target{Kind: checklist.TargetCategories, IDs: ids}, Items: list}
	d, err := checklist.NewDefinition(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Target.IDs, ",") != categoryA+","+categoryB || ids[0] != categoryB {
		t.Fatal("target not sorted on a copy", d.Target.IDs, ids)
	}
	ids[1], list[0].Label = "mutated", "mutated"
	if d.Target.IDs[0] != categoryA || d.Items[0].Label != "Pneus" {
		t.Fatal("definition aliases its input")
	}
	again, err := checklist.NewDefinition(d)
	if err != nil || fmt.Sprint(again) != fmt.Sprint(d) {
		t.Fatal("canonical definition is not stable", err)
	}
	org, err := checklist.NewDefinition(checklist.Definition{Name: "Padrão", Context: checklist.Return, Target: checklist.Target{Kind: checklist.TargetOrganization, IDs: []string{}}, Items: list[:1]})
	if err != nil || org.Target.IDs != nil {
		t.Fatal("organization target keeps no identifier list", err)
	}
	if _, err := checklist.NewDefinition(checklist.Definition{Name: "Inválido", Context: checklist.Departure, Target: checklist.Target{Kind: checklist.TargetAssets, IDs: []string{assetA}}, Items: []checklist.Item{{Label: "x", Critical: true, Optional: true}}}); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("critical optional item accepted", err)
	}
	if _, err := checklist.NewDefinition(checklist.Definition{Name: string([]byte{0xff}), Context: checklist.Departure, Target: checklist.Target{Kind: checklist.TargetOrganization}, Items: list}); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("invalid UTF-8 name accepted", err)
	}
}

func TestEvaluateNeverMutatesItsInputs(t *testing.T) {
	list := []checklist.Item{{Label: "Freios", Critical: true}, {Label: "Buzina"}}
	responses := []checklist.Response{{Answer: checklist.AnswerNotOK}, {Answer: checklist.AnswerNotOK, Note: "Fraca"}}
	e, err := checklist.Evaluate(list, checklist.Policy{Mode: checklist.Warn, ValiditySeconds: 3600}, responses)
	if err != nil || e.Outcome != checklist.CompletedWithIssues || !e.AcknowledgementRequired || fmt.Sprint(e.Issues) != "[1 2]" || fmt.Sprint(e.Critical) != "[1]" {
		t.Fatal("warn evaluation", e, err)
	}
	if responses[1].Note != "Fraca" || list[0].Label != "Freios" {
		t.Fatal("inputs changed")
	}
	clean, err := checklist.Evaluate(list, checklist.DefaultPolicy(), []checklist.Response{{Answer: checklist.AnswerOK}, {Answer: checklist.AnswerOK}})
	if err != nil || clean.Outcome != checklist.CompletedOK || clean.Issues != nil || clean.Critical != nil || clean.AcknowledgementRequired {
		t.Fatal("clean evaluation lists nothing", clean, err)
	}
}

func TestResponseNoteIsRedactedOnEveryFormattingSurface(t *testing.T) {
	const secret = "Placa SYN1A23 motorista fictício"
	r := checklist.Response{Answer: checklist.AnswerNotOK, Note: secret, Photo: true}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("response", "value", r, "list", []checklist.Response{r})
	for _, rendered := range []string{fmt.Sprint(r), fmt.Sprintf("%v %+v %#v", r, r, r), string(raw), log.String(), fmt.Sprint([]checklist.Response{r})} {
		if strings.Contains(rendered, "SYN1A23") || strings.Contains(rendered, "fictício") || strings.Contains(rendered, "506c616361") {
			t.Fatal("response note leaked", rendered)
		}
	}
}

func TestPolicyAtRefusesAnInstantOutsideTheCalendar(t *testing.T) {
	history := []checklist.PolicyRevision{{Revision: 1, Policy: checklist.DefaultPolicy(), EffectiveAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}}
	if _, _, err := checklist.PolicyAt(history, time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("year 10000 accepted", err)
	}
	if _, _, err := checklist.PolicyAt([]checklist.PolicyRevision{{Revision: 1, Policy: checklist.DefaultPolicy(), EffectiveAt: time.Date(0, 12, 31, 0, 0, 0, 0, time.UTC)}}, time.Now()); !errors.Is(err, checklist.ErrInvalid) {
		t.Fatal("year 0 accepted", err)
	}
	local := time.Date(2026, 11, 1, 1, 0, 0, 0, time.FixedZone("x", 3600))
	r, ok, err := checklist.PolicyAt(history, local)
	if err != nil || !ok || r.Revision != 1 {
		t.Fatal("offset instant compares as an instant", r, ok, err)
	}
}

func TestCanonicalIdentifiersAreLowercaseDashedAndNonNil(t *testing.T) {
	for raw, want := range map[string]bool{
		assetA:                                 true,
		"9c000000-0000-4000-8000-00000000200g": false,
		"9c000000x0000-4000-8000-000000002001": false,
		"9c0000000000-4000-8000-0000000020011": false,
		"9C000000-0000-4000-8000-000000002001": false,
		"00000000-0000-0000-0000-000000000000": false,
		"":                                     false,
	} {
		if checklist.CanonicalID(raw) != want {
			t.Fatal("canonical identifier", raw)
		}
	}
}
