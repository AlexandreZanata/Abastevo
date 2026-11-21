package runtimegrants

import (
	"fmt"
	"sort"
)

// Privilege names rendered into the script, in rendering order.
const (
	Select = "SELECT"
	Insert = "INSERT"
	Update = "UPDATE"
	Delete = "DELETE"
)

// TableGrant is the derived privilege set of one relation. An empty set
// means the runtime login has no access to it at all.
type TableGrant struct {
	Name       string
	Privileges []string
}

// OperatorOnly is a function the production binaries reference but the
// runtime login must never execute (a reviewed exception).
type OperatorOnly struct {
	Signature, Reason string
}

// TableLimit is a reviewed upper bound on what the derivation may grant.
type TableLimit struct {
	Table, Reason string
	Allowed       []string
}

// Inventory is the complete runtime privilege set for one schema version.
type Inventory struct {
	SchemaVersion string
	Tables        []TableGrant
	Sequences     []string // USAGE
	Functions     []string // explicit EXECUTE, signatures
	OperatorOnly  []OperatorOnly
	Limits        []TableLimit
}

// Exceptions is the reviewed list the derivation cannot see by itself.
type Exceptions struct {
	Limits       []TableLimit
	OperatorOnly []OperatorOnly
}

// ReviewedExceptions is the recorded exception list (EDR-363). Each entry is
// verified against the catalog and the code on every generation, so a stale
// entry fails instead of silently widening or narrowing the inventory.
func ReviewedExceptions() Exceptions {
	return Exceptions{
		Limits: []TableLimit{{
			Table:   "schema_migrations",
			Allowed: []string{Select},
			Reason:  "the readiness probe reads the ledger (migrate.Check); only the migration owner writes it",
		}, {
			Table:  "org_billing_charge_approvals",
			Reason: "only the restricted billing operator reads independent charge approvals (EMP-SUBS-004)",
		}, {
			Table:  "org_document_fiscal_consultations",
			Reason: "serving login cannot read or record external fiscal results (EMP-INTEG-004)",
		}},
		OperatorOnly: []OperatorOnly{{
			Signature: "org_meter_current_rebuild(uuid)",
			Reason:    "operator projection rebuild (EMP-METER-006, EDR-317); never reachable from a route or job",
		}},
	}
}

// Build derives the inventory from the head catalog and the SQL of the
// production binaries. SECURITY INVOKER bodies, policies, defaults, checks
// and views run as the runtime login, so their needs are added; SECURITY
// DEFINER bodies run as the owner and add nothing.
func Build(c Catalog, code *Usage, x Exceptions) (Inventory, error) {
	if c.SchemaVersion == "" {
		return Inventory{}, fmt.Errorf("runtimegrants: empty schema version")
	}
	all := NewUsage()
	all.Merge(code)
	for _, f := range c.Functions {
		if !f.SecurityDef {
			all.AddSQL(f.Body)
		}
	}
	for _, e := range c.Expressions {
		all.AddExpression(e)
	}
	limits := map[string]TableLimit{}
	for _, l := range x.Limits {
		limits[l.Table] = l
	}
	inv := Inventory{SchemaVersion: c.SchemaVersion, Limits: append([]TableLimit(nil), x.Limits...)}
	sort.Slice(inv.Limits, func(i, j int) bool { return inv.Limits[i].Table < inv.Limits[j].Table })
	inserted := map[string]bool{}
	found := map[string]bool{}
	for _, r := range c.Relations {
		found[r.Name] = true
		want := map[string]bool{
			Insert: all.Insert[r.Name], Update: all.Update[r.Name], Delete: all.Delete[r.Name],
		}
		if r.Kind == "v" || r.Kind == "m" {
			want = map[string]bool{}
		}
		want[Select] = all.Mentioned[r.Name] || want[Insert] || want[Update] || want[Delete]
		if l, ok := limits[r.Name]; ok {
			allowed := map[string]bool{}
			for _, p := range l.Allowed {
				allowed[p] = true
			}
			for p := range want {
				want[p] = want[p] && allowed[p]
			}
		}
		grant := TableGrant{Name: r.Name}
		for _, p := range []string{Select, Insert, Update, Delete} {
			if want[p] {
				grant.Privileges = append(grant.Privileges, p)
			}
		}
		inserted[r.Name] = want[Insert]
		inv.Tables = append(inv.Tables, grant)
	}
	for _, l := range x.Limits {
		if !found[l.Table] {
			return Inventory{}, fmt.Errorf("runtimegrants: stale table limit %s", l.Table)
		}
	}
	sort.Slice(inv.Tables, func(i, j int) bool { return inv.Tables[i].Name < inv.Tables[j].Name })
	for _, s := range c.Sequences {
		if inserted[s.Owner] || all.Mentioned[s.Name] {
			inv.Sequences = append(inv.Sequences, s.Name)
		}
	}
	sort.Strings(inv.Sequences)
	operator := map[string]OperatorOnly{}
	for _, o := range x.OperatorOnly {
		operator[o.Signature] = o
	}
	referenced := map[string]bool{}
	for _, f := range c.Functions {
		if f.SecurityDef && f.PublicExecute {
			return Inventory{}, fmt.Errorf("runtimegrants: SECURITY DEFINER function %s is executable by PUBLIC; review it before granting anything", f.Signature)
		}
		if !all.Calls[f.Name] || f.PublicExecute {
			continue
		}
		if o, ok := operator[f.Signature]; ok {
			referenced[f.Signature] = true
			inv.OperatorOnly = append(inv.OperatorOnly, o)
			continue
		}
		inv.Functions = append(inv.Functions, f.Signature)
	}
	for _, o := range x.OperatorOnly {
		if !referenced[o.Signature] {
			return Inventory{}, fmt.Errorf("runtimegrants: stale operator-only function %s", o.Signature)
		}
	}
	sort.Strings(inv.Functions)
	sort.Slice(inv.OperatorOnly, func(i, j int) bool { return inv.OperatorOnly[i].Signature < inv.OperatorOnly[j].Signature })
	return inv, nil
}
