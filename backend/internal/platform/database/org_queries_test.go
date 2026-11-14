package database_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type orgQueryProbe struct {
	scope, isolation string
	unsafe           bool
	calls            int
	failure          error
}

func (p *orgQueryProbe) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("scope validation cannot execute mutations")
}
func (p *orgQueryProbe) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("scope validation needs only bounded rows")
}
func (p *orgQueryProbe) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	p.calls++
	return orgProbeRow{p, sql}
}

type orgProbeRow struct {
	p   *orgQueryProbe
	sql string
}

func (r orgProbeRow) Scan(dest ...any) error {
	if r.p.failure != nil {
		return r.p.failure
	}
	if strings.Contains(r.sql, "current_user") {
		if len(dest) != 1 {
			panic("unexpected role projection")
		}
		*dest[0].(*bool) = r.p.unsafe
		return nil
	}
	if !strings.Contains(r.sql, "anpfuel.org_id") || len(dest) != 2 {
		panic("unexpected scope query")
	}
	*dest[0].(*string) = r.p.scope
	*dest[1].(*string) = r.p.isolation
	return nil
}
func TestQueryOnlyScopeValidationRetainsRoleScopeAndIsolation(t *testing.T) {
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000101")
	scope, _ := kernel.NewOrgScope(id)
	for _, c := range []struct {
		name                 string
		unsafe               bool
		installed, isolation string
		want                 error
	}{
		{"ordinary borrowed query", false, id.String(), "read committed", nil},
		{"unsafe runtime", true, id.String(), "read committed", database.ErrUnsafeOrgRole},
		{"wrong installed scope", false, "7b0e2a54-0000-4000-8000-000000000102", "read committed", kernel.ErrInvalidOrgScope},
		{"wrong isolation", false, id.String(), "repeatable read", database.ErrInvalidOrgTx},
	} {
		t.Run(c.name, func(t *testing.T) {
			probe := &orgQueryProbe{scope: c.installed, isolation: c.isolation, unsafe: c.unsafe}
			if err := database.ValidateOrgDBTX(t.Context(), probe, scope); !errors.Is(err, c.want) {
				t.Fatal("borrowed query boundary changed", err)
			}
			if probe.calls < 1 {
				t.Fatal("role proof not evaluated")
			}
		})
	}
	probe := &orgQueryProbe{}
	if err := database.ValidateOrgDBTX(t.Context(), probe, kernel.OrgScope{}); !errors.Is(err, kernel.ErrInvalidOrgScope) || probe.calls != 0 {
		t.Fatal("invalid scope reached database")
	}
	for _, queries := range []database.OrgDBTX{nil, (*orgQueryProbe)(nil)} {
		if err := database.ValidateOrgDBTX(t.Context(), queries, scope); !errors.Is(err, database.ErrInvalidOrgTx) {
			t.Fatal("missing query proof accepted")
		}
	}
	probe = &orgQueryProbe{failure: errors.New("synthetic query failure")}
	if err := database.ValidateOrgDBTX(t.Context(), probe, scope); err == nil {
		t.Fatal("source outage accepted")
	}
}
