package runtimegrants

import (
	"reflect"
	"sort"
	"testing"
)

func keys(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestAddSQLDerivesStatementPrivileges(t *testing.T) {
	cases := []struct {
		name                         string
		sql                          string
		insert, update, delete, call []string
		mentioned                    []string
	}{
		{name: "insert", sql: `INSERT INTO org_assets(org_id) VALUES($1)`, insert: []string{"org_assets"}, mentioned: []string{"org_assets"}},
		{name: "qualified quoted insert", sql: `insert into public."Org_Units" (x) select 1`, insert: []string{"org_units"}},
		{name: "update with alias", sql: "UPDATE org_memberships m\n SET state='ENDED' FROM org_units u WHERE m.id=$1", update: []string{"org_memberships"}},
		{name: "update only as alias", sql: `UPDATE ONLY public.accounts AS a SET deleted=true`, update: []string{"accounts"}},
		{name: "do update targets insert", sql: `INSERT INTO identity_rate_windows(k) VALUES($1) ON CONFLICT (k) DO UPDATE SET count=identity_rate_windows.count+1`, insert: []string{"identity_rate_windows"}, update: []string{"identity_rate_windows"}},
		{name: "do nothing stays insert", sql: `INSERT INTO org_sync_operations(id) VALUES($1) ON CONFLICT DO NOTHING`, insert: []string{"org_sync_operations"}},
		{name: "row lock reads", sql: `SELECT 1 FROM org_sync_review_items i JOIN org_sync_operations o ON o.id=i.op WHERE i.id=$1 FOR UPDATE OF i`, update: []string{"org_sync_operations", "org_sync_review_items"}},
		{name: "share lock", sql: `SELECT 1 FROM organizations WHERE org_id=$1 FOR KEY SHARE`, update: []string{"organizations"}},
		{name: "no key lock in subquery", sql: `UPDATE job_queue SET state='RUNNING' WHERE id IN (SELECT id FROM job_queue WHERE state='QUEUED' FOR NO KEY UPDATE SKIP LOCKED)`, update: []string{"job_queue"}},
		{name: "delete in cte", sql: `WITH gone AS (DELETE FROM ONLY company_web_sessions WHERE expires_at<now() RETURNING id) SELECT count(*) FROM gone`, delete: []string{"company_web_sessions"}},
		{name: "lock applies to its own statement only", sql: `SELECT 1 FROM accounts; SELECT 1 FROM org_units FOR UPDATE`, update: []string{"org_units"}},
		{name: "function calls", sql: `SELECT public.org_access_owner_coverage($1,NULL,NULL,NULL), pg_catalog.set_config('a',$2,true)`, call: []string{"org_access_owner_coverage", "set_config"}},
		{name: "commented statements ignored", sql: "SELECT 1 -- DELETE FROM accounts\n/* UPDATE accounts SET x=1 */ FROM organizations", mentioned: []string{"organizations"}},
		{name: "trigger keywords are not updates", sql: `SELECT 1 FROM t WHERE x IS DISTINCT FROM 'ON UPDATE'; PERFORM 1 FROM org_trips FOR UPDATE`, update: []string{"org_trips"}},
		{name: "plain prose ignored", sql: `organization creation failed`, mentioned: []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := NewUsage()
			u.AddSQL(c.sql)
			for _, want := range []struct {
				label string
				got   map[string]bool
				want  []string
			}{{"insert", u.Insert, c.insert}, {"update", u.Update, c.update}, {"delete", u.Delete, c.delete}} {
				w := want.want
				if w == nil {
					w = []string{}
				}
				if !reflect.DeepEqual(keys(want.got), w) {
					t.Fatalf("%s = %v, want %v", want.label, keys(want.got), w)
				}
			}
			for _, name := range c.call {
				if !u.Calls[name] {
					t.Fatalf("call %s not recorded: %v", name, keys(u.Calls))
				}
			}
			if c.mentioned != nil {
				for _, name := range c.mentioned {
					if !u.Mentioned[name] {
						t.Fatalf("mention %s not recorded", name)
					}
				}
				if len(c.mentioned) == 0 && len(u.Mentioned) != 0 {
					t.Fatalf("prose recorded as SQL: %v", keys(u.Mentioned))
				}
				if u.Mentioned["delete"] || u.Mentioned["x"] {
					t.Fatal("comment text recorded")
				}
			}
		})
	}
}

func TestAddExpressionRecordsDefaultsAndPolicies(t *testing.T) {
	u := NewUsage()
	u.AddExpression(`nextval('admin_audit_events_id_seq'::regclass)`)
	u.AddExpression(`(org_id = (current_setting('anpfuel.org_id'::text, true))::uuid)`)
	if !u.Mentioned["admin_audit_events_id_seq"] || !u.Calls["nextval"] || !u.Calls["current_setting"] || len(u.Insert)+len(u.Update)+len(u.Delete) != 0 {
		t.Fatal("expression usage", keys(u.Mentioned), keys(u.Calls))
	}
}
