package runtimegrants

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func syntheticCatalog() Catalog {
	return Catalog{
		SchemaVersion: "000999",
		Relations: []Relation{
			{"accounts", "r"}, {"org_audit_events", "r"}, {"org_units", "r"}, {"schema_migrations", "r"},
			{"operator_notes", "r"}, {"admin_audit_events", "r"}, {"unit_view", "v"},
		},
		Sequences: []Sequence{{"admin_audit_events_id_seq", "admin_audit_events"}, {"unused_seq", "operator_notes"}},
		Functions: []Function{
			{Signature: "org_unit_projection()", Name: "org_unit_projection", PublicExecute: true, Body: `BEGIN INSERT INTO org_audit_events(x) VALUES(1); RETURN NEW; END`},
			{Signature: "org_meter_current_rebuild(uuid)", Name: "org_meter_current_rebuild", SecurityDef: true},
			{Signature: "org_definer_count(uuid)", Name: "org_definer_count", SecurityDef: true},
			{Signature: "org_closed_helper(uuid)", Name: "org_closed_helper"},
		},
		Expressions: []string{`nextval('admin_audit_events_id_seq'::regclass)`, `(org_id = (current_setting('anpfuel.org_id'::text, true))::uuid)`},
	}
}

func syntheticCode() *Usage {
	u := NewUsage()
	u.AddSQL(`INSERT INTO org_units(org_id) VALUES($1)`)
	u.AddSQL(`DELETE FROM accounts WHERE id=$1`)
	u.AddSQL(`INSERT INTO schema_migrations(version) VALUES($1)`)
	u.AddSQL(`SELECT org_meter_current_rebuild($1), org_closed_helper($1) FROM unit_view`)
	u.AddSQL(`INSERT INTO admin_audit_events(a) VALUES($1)`)
	return u
}

func TestBuildDerivesLeastPrivilegeWithReviewedExceptions(t *testing.T) {
	x := Exceptions{Limits: []TableLimit{{Table: "schema_migrations", Allowed: []string{Select}, Reason: "ledger"}}, OperatorOnly: []OperatorOnly{{Signature: "org_meter_current_rebuild(uuid)", Reason: "operator"}}}
	inv, err := Build(syntheticCatalog(), syntheticCode(), x)
	if err != nil {
		t.Fatal(err)
	}
	want := []TableGrant{
		{"accounts", []string{Select, Delete}},
		{"admin_audit_events", []string{Select, Insert}},
		{"operator_notes", nil},
		{"org_audit_events", []string{Select, Insert}}, // written only by the invoker trigger body
		{"org_units", []string{Select, Insert}},
		{"schema_migrations", []string{Select}}, // migrate package insert limited
		{"unit_view", []string{Select}},
	}
	if !reflect.DeepEqual(inv.Tables, want) {
		t.Fatalf("tables\n got %v\nwant %v", inv.Tables, want)
	}
	if !reflect.DeepEqual(inv.Sequences, []string{"admin_audit_events_id_seq"}) {
		t.Fatal("sequences", inv.Sequences)
	}
	if !reflect.DeepEqual(inv.Functions, []string{"org_closed_helper(uuid)"}) || len(inv.OperatorOnly) != 1 || inv.OperatorOnly[0].Signature != "org_meter_current_rebuild(uuid)" {
		t.Fatal("functions", inv.Functions, inv.OperatorOnly)
	}
	for _, p := range inv.Tables {
		for _, privilege := range p.Privileges {
			if privilege == "TRUNCATE" || privilege == "REFERENCES" || privilege == "TRIGGER" {
				t.Fatal("forbidden privilege derived")
			}
		}
	}
}

func TestBuildRefusesStaleOrUnsafeInputs(t *testing.T) {
	base := ReviewedExceptions()
	if _, err := Build(Catalog{}, NewUsage(), base); err == nil {
		t.Fatal("empty schema version accepted")
	}
	stale := Exceptions{Limits: []TableLimit{{Table: "dropped_table", Allowed: []string{Select}}}}
	if _, err := Build(syntheticCatalog(), syntheticCode(), stale); err == nil || !strings.Contains(err.Error(), "stale table limit") {
		t.Fatal("stale limit accepted", err)
	}
	unreferenced := Exceptions{OperatorOnly: []OperatorOnly{{Signature: "org_definer_count(uuid)"}}}
	if _, err := Build(syntheticCatalog(), NewUsage(), unreferenced); err == nil || !strings.Contains(err.Error(), "stale operator-only") {
		t.Fatal("unreferenced operator-only function accepted", err)
	}
	open := syntheticCatalog()
	open.Functions[2].PublicExecute = true
	if _, err := Build(open, syntheticCode(), Exceptions{OperatorOnly: base.OperatorOnly}); err == nil || !strings.Contains(err.Error(), "SECURITY DEFINER") {
		t.Fatal("PUBLIC SECURITY DEFINER function accepted", err)
	}
}

func TestRenderIsDeterministicAndParameterized(t *testing.T) {
	x := Exceptions{Limits: []TableLimit{{Table: "schema_migrations", Allowed: []string{Select}, Reason: "ledger"}}, OperatorOnly: []OperatorOnly{{Signature: "org_meter_current_rebuild(uuid)", Reason: "operator"}}}
	inv, err := Build(syntheticCatalog(), syntheticCode(), x)
	if err != nil {
		t.Fatal(err)
	}
	first, second := Render(inv), Render(inv)
	if !bytes.Equal(first, second) {
		t.Fatal("render is not deterministic")
	}
	text := string(first)
	for _, want := range []string{
		"GRANT SELECT, DELETE ON TABLE public.accounts TO :\"runtime_role\";",
		"GRANT SELECT ON TABLE public.schema_migrations TO :\"runtime_role\";",
		"-- no access: public.operator_notes",
		"GRANT USAGE ON SEQUENCE public.admin_audit_events_id_seq TO :\"runtime_role\";",
		"GRANT EXECUTE ON FUNCTION public.org_closed_helper(uuid) TO :\"runtime_role\";",
		"--   org_meter_current_rebuild(uuid): operator.",
		"IF head IS DISTINCT FROM '000999' THEN",
		"ARRAY['org_closed_helper(uuid)']::text[]",
		"NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION",
		"BEGIN;", "COMMIT;",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("script lacks %q", want)
		}
	}
	var statements []string
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "--") {
			statements = append(statements, strings.ToUpper(line))
		}
	}
	code := strings.Join(statements, "\n")
	for _, forbidden := range []string{"PASSWORD", "GRANT TRUNCATE", ", TRUNCATE ON", "GRANT ALL", "OWNER TO", "SET ROLE", "CREATE TABLE", "ALTER TABLE", "DROP ", "WITH GRANT OPTION"} {
		if strings.Contains(code, forbidden) {
			t.Fatalf("script statement contains %q", forbidden)
		}
	}
	applied, err := ForRole(first, "abastevo_runtime")
	if err != nil || strings.Contains(applied, ":'"+RoleVariable) || strings.Contains(applied, `:"`+RoleVariable) || !strings.Contains(applied, `TO "abastevo_runtime";`) || !strings.Contains(applied, `set_config('abastevo.runtime_role', 'abastevo_runtime', true)`) {
		t.Fatal("role substitution", err)
	}
	for _, role := range []string{"", "Abastevo", "x;drop", "a\"b", strings.Repeat("a", 64), "1runtime"} {
		if _, err := ForRole(first, role); err == nil {
			t.Fatalf("invalid role %q accepted", role)
		}
	}
	if _, err := ForRole([]byte("SELECT :'other'"), "abastevo_runtime"); err == nil {
		t.Fatal("unresolved psql variable accepted")
	}
}
