package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
)

func frozenSchema(t *testing.T) *openapi3.T {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "../../../../../contracts/openapi/mobile-exploration-planned.json")
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestInterpretedQueriesValidateAgainstFrozenSchema(t *testing.T) {
	doc := frozenSchema(t)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for question, schema := range map[string]string{
		"Qual o maior preço observado da gasolina comum no Brasil?": "PriceQuery",
		"Qual a maior média de preço do etanol por estado?":         "PriceQuery",
		"Quais ações de fiscalização da ANP no DF?":                 "OfficialQuery",
	} {
		got := Interpret(question, now)
		if got.State != StateResolved {
			t.Fatalf("question %q unresolved: %+v", question, got)
		}
		raw, err := json.Marshal(got.Query)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err := doc.Components.Schemas[schema].Value.VisitJSON(value); err != nil {
			t.Fatalf("question %q outside frozen %s: %v\n%s", question, schema, err, raw)
		}
	}
}

func TestInterpretedChoicesValidateAgainstFrozenSchema(t *testing.T) {
	doc := frozenSchema(t)
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	got := Interpret("Qual a gasolina mais cara do Brasil?", now)
	if got.State != StateNeedsChoice {
		t.Fatalf("expected choice: %+v", got)
	}
	if len(got.Alternatives) < 2 || len(got.Alternatives) > 6 {
		t.Fatal("alternative count outside 2..6")
	}
	for _, alternative := range got.Alternatives {
		raw, err := json.Marshal(alternative)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if err := doc.Components.Schemas["PriceQuery"].Value.VisitJSON(value); err != nil {
			t.Fatalf("alternative outside frozen PriceQuery: %v\n%s", err, raw)
		}
	}
	_ = os.Getenv
}
