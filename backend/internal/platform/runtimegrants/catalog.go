package runtimegrants

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Querier is the read-only catalog access the loader needs.
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Relation is one application table or view of schema public.
type Relation struct {
	Name string
	Kind string // r, p, v or m
}

// Sequence is one application sequence and the relation that owns it.
type Sequence struct {
	Name, Owner string
}

// Function is one application function or procedure of schema public.
type Function struct {
	Signature     string // regprocedure text, e.g. org_meter_current_rebuild(uuid)
	Name          string
	SecurityDef   bool
	PublicExecute bool
	Body          string // source of SECURITY INVOKER functions only
}

// Catalog holds the head facts the inventory is derived from. Extension
// members (PostGIS, btree_gist) are excluded: their own grants apply.
type Catalog struct {
	SchemaVersion string
	Relations     []Relation
	Sequences     []Sequence
	Functions     []Function
	// Expressions evaluated as the calling login: RLS policies, column
	// defaults, CHECK constraints and non-extension view definitions.
	Expressions []string
}

const notExtension = `NOT EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid=%s AND d.objid=%s AND d.deptype='e')`

// LoadCatalog reads the migrated head as the migration owner.
func LoadCatalog(ctx context.Context, q Querier) (Catalog, error) {
	var c Catalog
	if err := q.QueryRow(ctx, `SELECT COALESCE(max(version),'') FROM public.schema_migrations`).Scan(&c.SchemaVersion); err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: schema version: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT c.relname, c.relkind::text FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m') AND `+fmt.Sprintf(notExtension, `'pg_catalog.pg_class'::regclass`, `c.oid`)+` ORDER BY 1`)
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: relations: %w", err)
	}
	c.Relations, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Relation, error) {
		var r Relation
		return r, row.Scan(&r.Name, &r.Kind)
	})
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: relations: %w", err)
	}
	rows, err = q.Query(ctx, `SELECT s.relname, COALESCE((SELECT t.relname FROM pg_catalog.pg_depend d JOIN pg_catalog.pg_class t ON t.oid=d.refobjid
			WHERE d.classid='pg_catalog.pg_class'::regclass AND d.objid=s.oid AND d.refclassid='pg_catalog.pg_class'::regclass AND d.deptype IN ('a','i') LIMIT 1),'')
		FROM pg_catalog.pg_class s JOIN pg_catalog.pg_namespace n ON n.oid=s.relnamespace
		WHERE n.nspname='public' AND s.relkind='S' AND `+fmt.Sprintf(notExtension, `'pg_catalog.pg_class'::regclass`, `s.oid`)+` ORDER BY 1`)
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: sequences: %w", err)
	}
	c.Sequences, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Sequence, error) {
		var s Sequence
		return s, row.Scan(&s.Name, &s.Owner)
	})
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: sequences: %w", err)
	}
	rows, err = q.Query(ctx, `SELECT p.oid::regprocedure::text, p.proname, p.prosecdef, pg_catalog.has_function_privilege('public', p.oid, 'EXECUTE'),
			CASE WHEN p.prosecdef THEN '' ELSE COALESCE(p.prosrc,'') END
		FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid=p.pronamespace
		WHERE n.nspname='public' AND `+fmt.Sprintf(notExtension, `'pg_catalog.pg_proc'::regclass`, `p.oid`)+` ORDER BY 1`)
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: functions: %w", err)
	}
	c.Functions, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Function, error) {
		var f Function
		return f, row.Scan(&f.Signature, &f.Name, &f.SecurityDef, &f.PublicExecute, &f.Body)
	})
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: functions: %w", err)
	}
	rows, err = q.Query(ctx, `SELECT text FROM (
		SELECT COALESCE(qual,'')||' '||COALESCE(with_check,'') AS text FROM pg_catalog.pg_policies WHERE schemaname='public'
		UNION ALL SELECT pg_catalog.pg_get_expr(a.adbin, a.adrelid) FROM pg_catalog.pg_attrdef a JOIN pg_catalog.pg_class c ON c.oid=a.adrelid
			JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND `+fmt.Sprintf(notExtension, `'pg_catalog.pg_class'::regclass`, `c.oid`)+`
		UNION ALL SELECT pg_catalog.pg_get_constraintdef(k.oid) FROM pg_catalog.pg_constraint k JOIN pg_catalog.pg_namespace n ON n.oid=k.connamespace
			WHERE n.nspname='public' AND k.contype='c'
		UNION ALL SELECT pg_catalog.pg_get_viewdef(c.oid) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
			WHERE n.nspname='public' AND c.relkind IN ('v','m') AND `+fmt.Sprintf(notExtension, `'pg_catalog.pg_class'::regclass`, `c.oid`)+`
	) expressions ORDER BY 1`)
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: expressions: %w", err)
	}
	c.Expressions, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return Catalog{}, fmt.Errorf("runtimegrants: expressions: %w", err)
	}
	return c, nil
}
