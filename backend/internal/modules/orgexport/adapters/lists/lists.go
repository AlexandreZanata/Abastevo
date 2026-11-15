// Package lists admits the operational list export formats (EMP-RPT-002):
// assets, meter readings and the employee register, each a CSV table with
// its manifest in a ZIP package. Readers re-check the recorded requester and
// read the job's organization in one scoped, read-only transaction.
package lists

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Admitted kinds. The table format is CSV; the delivered file is the list
// package (domain.ListMediaType).
const (
	AssetListKind    = "fleet.asset_list.csv"
	ReadingListKind  = "fleet.meter_reading_list.csv"
	EmployeeListKind = "idn.employee_list.csv"
)

// Requesters re-evaluates the recorded requester inside the reading
// transaction (the export authority's session-free check).
type Requesters interface {
	CheckRequesterTx(context.Context, pgx.Tx, kernel.OrgScope, app.Requester, string, permissions.Permission) error
}

// filters are a kind's validated filter values by key.
type filters map[string]string

// list is one admitted list: its spec, its filter rules, its one statement
// and its row decoder.
type list interface {
	spec() domain.ListSpec
	// filter validates one filter value of an allowed key.
	filter(key, value string) bool
	// consistent checks rules across filters (a period's order).
	consistent(filters) bool
	// statement is the scoped read of the organization, bound by limit.
	statement(org string, f filters, asOf, generated time.Time, limit int64) (string, []any)
	// row decodes one result row into the spec's cells.
	row(pgx.Rows) ([]domain.Cell, error)
}

// Format is one admitted list kind.
type Format struct {
	list       list
	pool       *pgxpool.Pool
	requesters Requesters
}

var _ app.Format = Format{}

var all = []list{assetList{}, readingList{}, employeeList{}}

// New composes the three admitted list formats on the runtime pool and the
// export authority. Without either, Produce is unavailable; the declarations
// still serve the request path.
func New(pool *pgxpool.Pool, requesters Requesters) []app.Format {
	out := make([]app.Format, 0, len(all))
	for _, l := range all {
		out = append(out, Format{list: l, pool: pool, requesters: requesters})
	}
	return out
}

// Spec returns the static description of an admitted kind (for tests and
// documentation); false for any other kind.
func Spec(kind string) (domain.ListSpec, bool) {
	for _, l := range all {
		if s := l.spec(); s.Kind == kind {
			return s, true
		}
	}
	return domain.ListSpec{}, false
}

func (f Format) Kind() string { return f.list.spec().Kind }
func (f Format) ReadPermission() permissions.Permission {
	return permissions.Permission(f.list.spec().ReadPermission)
}
func (f Format) MediaType() string     { return domain.ListMediaType }
func (f Format) Limits() domain.Limits { return domain.DefaultLimits() }

// parse admits only the kind's closed filter object: known keys, non-empty
// string values valid for their key, and consistent together.
func (f Format) parse(raw []byte) (filters, error) {
	canonical, err := domain.CanonicalParameters(raw)
	if err != nil || !bytes.Equal(canonical, raw) {
		return nil, domain.ErrInvalid
	}
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil {
		return nil, domain.ErrInvalid
	}
	out := filters{}
	for key, value := range values {
		text, isText := value.(string)
		if !isText || text == "" || !f.list.filter(key, text) {
			return nil, domain.ErrInvalid
		}
		out[key] = text
	}
	if !f.list.consistent(out) {
		return nil, domain.ErrInvalid
	}
	return out, nil
}

// Canonical admits only the kind's closed filter object.
func (f Format) Canonical(raw []byte) ([]byte, error) {
	if _, err := f.parse(raw); err != nil {
		return nil, err
	}
	return bytes.Clone(raw), nil
}

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// Produce re-checks the requester, then reads and writes the list in one
// scoped transaction (B-BR-EMP-255). A loss of authority is ErrRightsLost, a
// ceiling ErrTooLarge; any other failure is retryable and nothing complete
// is left in the sink.
func (f Format) Produce(ctx context.Context, in app.ProduceInput, sink *domain.Sink) error {
	if f.pool == nil || missing(f.requesters) || sink == nil {
		return app.ErrUnavailable
	}
	if in.Scope.Validate() != nil || in.Requester.Validate() != nil || !domain.ValidID(in.ExportID) || in.AsOf.IsZero() {
		return domain.ErrInvalid
	}
	values, err := f.parse(in.Parameters)
	if err != nil {
		return err
	}
	spec := f.list.spec()
	org := in.Scope.OrgID().String()
	return database.WithOrgTx(ctx, f.pool, in.Scope, func(tx pgx.Tx) error {
		if err := f.requesters.CheckRequesterTx(ctx, tx, in.Scope, in.Requester, spec.Kind, f.ReadPermission()); err != nil {
			if errors.Is(err, app.ErrRightsLost) {
				return app.ErrRightsLost
			}
			return err
		}
		// Nothing below can write, whatever its SQL.
		if _, err := tx.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
			return err
		}
		var generated time.Time
		var name, zone string
		if err := tx.QueryRow(ctx, `SELECT transaction_timestamp(),COALESCE(display_name,''),COALESCE(time_zone,'') FROM organizations WHERE org_id=$1::uuid`, org).Scan(&generated, &name, &zone); err != nil {
			return err
		}
		writer, err := domain.NewListWriter(sink, spec, domain.ListBasis{ExportID: in.ExportID, OrganizationID: org, OrganizationName: name, OrganizationTimeZone: zone,
			RequesterMembershipID: in.Requester.MembershipID, AsOf: in.AsOf, GeneratedAt: generated, Filters: in.Parameters})
		if err != nil {
			return err
		}
		query, args := f.list.statement(org, values, in.AsOf.UTC(), generated, f.Limits().Rows+1)
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			cells, err := f.list.row(rows)
			if err != nil {
				return err
			}
			if err := writer.Row(cells...); err != nil {
				return err
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		return writer.Close()
	})
}

func oneOf(value string, allowed ...string) bool { return slices.Contains(allowed, value) }
