package adapters

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters/lists"
	exportapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AccountingCompetenceCSVKind = "fin.accounting_competence.csv"
	AccountingCashCSVKind       = "fin.accounting_cash.csv"
	TripLogbookPDFKind          = "rpt.trip_logbook.pdf"
	FuelingStatementPDFKind     = "rpt.fueling_statement.pdf"
	PeriodSummaryPDFKind        = "rpt.period_summary.pdf"
	FullOrganizationZIPKind     = "org.full_export.zip"
)

type f6ExportSpec struct {
	kind       string
	permission permissions.Permission
	mediaType  string
}

var f6ExportSpecs = []f6ExportSpec{
	{kind: AccountingCompetenceCSVKind, permission: permissions.Permission("exp.expense.read"), mediaType: "text/csv"},
	{kind: AccountingCashCSVKind, permission: permissions.Permission("exp.expense.read"), mediaType: "text/csv"},
	{kind: TripLogbookPDFKind, permission: permissions.Permission("trip.trip.read"), mediaType: "application/pdf"},
	{kind: FuelingStatementPDFKind, permission: permissions.Permission("fuel.fueling.read"), mediaType: "application/pdf"},
	{kind: PeriodSummaryPDFKind, permission: permissions.Permission("kpi.dashboard.read"), mediaType: "application/pdf"},
	{kind: FullOrganizationZIPKind, permission: permissions.Permission("audit.event.read"), mediaType: "application/zip"},
}

type f6ExportFormat struct {
	spec       f6ExportSpec
	pool       *pgxpool.Pool
	requesters lists.Requesters
}

var _ exportapp.Format = f6ExportFormat{}

// ExportFormats composes the Phase F6 export formats (accounting CSV,
// deterministic operational/financial PDFs, and full organization ZIP).
func ExportFormats(pool *pgxpool.Pool, requesters lists.Requesters) []exportapp.Format {
	out := make([]exportapp.Format, 0, len(f6ExportSpecs))
	for _, spec := range f6ExportSpecs {
		out = append(out, f6ExportFormat{spec: spec, pool: pool, requesters: requesters})
	}
	return out
}

func (f f6ExportFormat) Kind() string                           { return f.spec.kind }
func (f f6ExportFormat) ReadPermission() permissions.Permission { return f.spec.permission }
func (f f6ExportFormat) MediaType() string                      { return f.spec.mediaType }
func (f f6ExportFormat) Limits() domain.Limits                  { return domain.DefaultLimits() }

func (f f6ExportFormat) Canonical(raw []byte) ([]byte, error) {
	canonical, err := domain.CanonicalParameters(raw)
	if err != nil {
		return nil, domain.ErrInvalid
	}
	var vals map[string]any
	if err := json.Unmarshal(canonical, &vals); err != nil {
		return nil, domain.ErrInvalid
	}
	if basis := accountingBasis(f.spec.kind); basis != "" {
		return canonicalAccounting(canonical, vals, basis)
	}
	for k, v := range vals {
		switch k {
		case "period_start", "period_end":
			s, ok := v.(string)
			if !ok {
				return nil, domain.ErrInvalid
			}
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return nil, domain.ErrInvalid
			}
		case "include_pending":
			if _, ok := v.(bool); !ok {
				return nil, domain.ErrInvalid
			}
		case "basis":
			s, ok := v.(string)
			if !ok || (s != "COMPETENCE" && s != "CASH") {
				return nil, domain.ErrInvalid
			}
		default:
			return nil, domain.ErrInvalid
		}
	}
	return canonical, nil
}

func accountingBasis(kind string) string {
	switch kind {
	case AccountingCompetenceCSVKind:
		return "COMPETENCE"
	case AccountingCashCSVKind:
		return "CASH"
	default:
		return ""
	}
}

func canonicalAccounting(canonical []byte, vals map[string]any, basis string) ([]byte, error) {
	for key, value := range vals {
		switch key {
		case "period_start", "period_end":
			day, ok := value.(string)
			parsed, err := time.Parse(time.DateOnly, day)
			if !ok || err != nil || parsed.Year() < 1 || parsed.Format(time.DateOnly) != day {
				return nil, domain.ErrInvalid
			}
		case "basis":
			if value != basis {
				return nil, domain.ErrInvalid
			}
		case "include_pending":
			if value != false {
				return nil, domain.ErrInvalid
			}
		case "layout_version":
			if value != float64(1) {
				return nil, domain.ErrInvalid
			}
		default:
			return nil, domain.ErrInvalid
		}
	}
	start, _ := vals["period_start"].(string)
	end, _ := vals["period_end"].(string)
	if start != "" && end != "" && start > end || vals["layout_version"] != nil && (start == "" || end == "") {
		return nil, domain.ErrInvalid
	}
	return canonical, nil
}

func (f f6ExportFormat) Produce(ctx context.Context, in exportapp.ProduceInput, sink *domain.Sink) error {
	if f.pool == nil || f.requesters == nil || sink == nil {
		return exportapp.ErrUnavailable
	}
	if in.Scope.Validate() != nil || in.Requester.Validate() != nil || !domain.ValidID(in.ExportID) || in.AsOf.IsZero() {
		return domain.ErrInvalid
	}
	paramsRaw, err := f.Canonical(in.Parameters)
	if err != nil {
		return domain.ErrInvalid
	}
	var params map[string]any
	_ = json.Unmarshal(paramsRaw, &params)
	org := in.Scope.OrgID().String()

	return database.WithOrgTx(ctx, f.pool, in.Scope, func(tx pgx.Tx) error {
		if err := f.requesters.CheckRequesterTx(ctx, tx, in.Scope, in.Requester, f.spec.kind, f.spec.permission); err != nil {
			if errors.Is(err, exportapp.ErrRightsLost) {
				return exportapp.ErrRightsLost
			}
			return err
		}
		if _, err := tx.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
			return err
		}
		var orgName, orgTZ string
		var generatedAt time.Time
		if err := tx.QueryRow(ctx, `SELECT transaction_timestamp(),COALESCE(display_name,''),COALESCE(time_zone,'America/Sao_Paulo') FROM organizations WHERE org_id=$1::uuid`, org).
			Scan(&generatedAt, &orgName, &orgTZ); err != nil {
			return err
		}
		switch f.spec.kind {
		case AccountingCompetenceCSVKind, AccountingCashCSVKind:
			return produceAccountingCSV(ctx, tx, sink, org, orgTZ, accountingBasis(f.spec.kind), in.AsOf, params)
		case TripLogbookPDFKind:
			return produceTripLogbookPDF(ctx, tx, sink, org, orgName, orgTZ, in.ExportID, in.AsOf)
		case FuelingStatementPDFKind:
			return produceFuelingStatementPDF(ctx, tx, sink, org, orgName, orgTZ, in.ExportID, in.AsOf)
		case PeriodSummaryPDFKind:
			return producePeriodSummaryPDF(ctx, tx, sink, org, orgName, orgTZ, in.ExportID, in.AsOf)
		case FullOrganizationZIPKind:
			return produceFullZIP(ctx, tx, sink, org, orgName, in.ExportID, in.Requester.MembershipID, in.AsOf, generatedAt)
		default:
			return domain.ErrInvalid
		}
	})
}

func csvLineBytes(cells []string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cells); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func produceAccountingCSV(ctx context.Context, tx pgx.Tx, sink *domain.Sink, org, zone, basis string, asOf time.Time, params map[string]any) error {
	if _, err := sink.Write([]byte(domain.ByteOrderMark)); err != nil {
		return err
	}
	hdr, err := csvLineBytes([]string{"entry_id", "basis", "origin_kind", "category", "asset_id", "reporting_day", "occurred_at", "paid_centavos", "company_centavos", "review_state"})
	if err != nil {
		return err
	}
	if _, err := sink.Write(hdr); err != nil {
		return err
	}
	start, _ := params["period_start"].(string)
	end, _ := params["period_end"].(string)
	var query string
	args := []any{org, start, end, asOf}
	if basis == "COMPETENCE" {
		query = `
			SELECT id::text, origin_kind, category, COALESCE(asset_id::text,''), reporting_day::text, occurred_at, paid_centavos, company_centavos, review_state
			FROM (
			  SELECT e.id, e.origin_kind, e.category, e.asset_id, e.reporting_day, e.occurred_at, e.paid_centavos, e.company_centavos, 'APPROVED' AS review_state
			  FROM org_expenses e WHERE e.org_id=$1::uuid AND e.received_at<=$4
			    AND COALESCE((SELECT r.state FROM org_fueling_review_entries r
			                 WHERE (r.org_id,r.fueling_id,r.revision)=(e.org_id,e.fueling_id,e.source_revision) AND r.recorded_at<=$4
			                 ORDER BY r.sequence DESC LIMIT 1),e.review_state)='APPROVED'
			  UNION ALL
			  SELECT id, origin_kind, category, asset_id, reporting_day, occurred_at, paid_centavos, company_centavos, review_state
			  FROM org_operating_cost_entries WHERE org_id=$1::uuid AND operating_cost AND review_state='APPROVED' AND created_at<=$4
			) u
			WHERE occurred_at<=$4 AND (NULLIF($2,'')::date IS NULL OR reporting_day>=NULLIF($2,'')::date)
			  AND (NULLIF($3,'')::date IS NULL OR reporting_day<=NULLIF($3,'')::date)
			ORDER BY reporting_day ASC, occurred_at ASC, id ASC`
	} else {
		query = `
			SELECT id::text, origin_kind, kind, ''::text, reporting_day::text, paid_at, settled_centavos, settled_centavos, state
			FROM (
			  SELECT id, 'PAYABLE_STATEMENT' AS origin_kind, kind, (created_at AT TIME ZONE $5::text)::date AS reporting_day,
			         created_at AS paid_at, settled_centavos, state
			  FROM org_payable_statements WHERE org_id=$1::uuid AND state='SETTLED' AND updated_at<=$4
			) c
			WHERE paid_at<=$4 AND (NULLIF($2,'')::date IS NULL OR reporting_day>=NULLIF($2,'')::date)
			  AND (NULLIF($3,'')::date IS NULL OR reporting_day<=NULLIF($3,'')::date)
			ORDER BY paid_at ASC, id ASC`
		args = append(args, zone)
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, origin, cat, aid, rday, st string
		var paid, comp int64
		var at time.Time
		if err := rows.Scan(&id, &origin, &cat, &aid, &rday, &at, &paid, &comp, &st); err != nil {
			return err
		}
		line, err := csvLineBytes([]string{
			domain.InertText(id), domain.InertText(basis), domain.InertText(origin), domain.InertText(cat),
			domain.InertText(aid), domain.InertText(rday), at.UTC().Format(time.RFC3339Nano),
			strconv.FormatInt(paid, 10), strconv.FormatInt(comp, 10), domain.InertText(st),
		})
		if err != nil {
			return err
		}
		if err := sink.Row(line); err != nil {
			return err
		}
	}
	return rows.Err()
}

func pdfTextEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 32 || r > 126 {
			r = '?'
		}
		switch r {
		case '\\', '(', ')':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func buildDeterministicPDF(lines []string) []byte {
	var stream strings.Builder
	stream.WriteString("BT\n/F1 10 Tf\n40 760 Td\n14 TL\n")
	for i, l := range lines {
		if i > 0 {
			stream.WriteString("T*\n")
		}
		stream.WriteString("(")
		stream.WriteString(pdfTextEscape(l))
		stream.WriteString(") Tj\n")
	}
	stream.WriteString("ET\n")
	content := stream.String()

	objs := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 5 0 R /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
		fmt.Sprintf("5 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(content), content),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for i, obj := range objs {
		offsets[i+1] = out.Len()
		out.WriteString(obj)
	}
	xrefPos := out.Len()
	out.WriteString(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1))
	for i := 1; i <= len(objs); i++ {
		out.WriteString(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}
	out.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefPos))
	return out.Bytes()
}

func writePDFLines(sink *domain.Sink, lines []string) error {
	for range lines {
		if err := sink.CountRow(); err != nil {
			return err
		}
	}
	_, err := sink.Write(buildDeterministicPDF(lines))
	return err
}

func produceTripLogbookPDF(ctx context.Context, tx pgx.Tx, sink *domain.Sink, org, orgName, orgTZ, exportID string, asOf time.Time) error {
	lines := []string{
		"ABASTEVO EMPRESAS - TRIP LOGBOOK REPORT",
		fmt.Sprintf("Org: %s (%s) TZ: %s", orgName, org, orgTZ),
		fmt.Sprintf("Export: %s AsOf: %s Review: APPROVED_AND_COMPLETED", exportID, asOf.UTC().Format(time.RFC3339)),
		"------------------------------------------------------------------------",
	}
	rows, err := tx.Query(ctx, `SELECT id::text, asset_id::text, reporting_day::text, state, occurred_at FROM org_trips WHERE org_id=$1::uuid ORDER BY occurred_at DESC, id LIMIT 200`, org)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, aid, rday, st string
		var at time.Time
		if err := rows.Scan(&id, &aid, &rday, &st, &at); err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("trip=%s asset=%s day=%s state=%s at=%s", id, aid, rday, st, at.UTC().Format(time.RFC3339)))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return writePDFLines(sink, lines)
}

func produceFuelingStatementPDF(ctx context.Context, tx pgx.Tx, sink *domain.Sink, org, orgName, orgTZ, exportID string, asOf time.Time) error {
	lines := []string{
		"ABASTEVO EMPRESAS - FUELING STATEMENT",
		fmt.Sprintf("Org: %s (%s) TZ: %s", orgName, org, orgTZ),
		fmt.Sprintf("Export: %s AsOf: %s Basis: APPROVED_ONLY", exportID, asOf.UTC().Format(time.RFC3339)),
		"------------------------------------------------------------------------",
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, asset_id::text, product, unit, quantity, paid_centavos, occurred_at, source_kind
		FROM (
		  SELECT id, asset_id, product, unit, quantity, paid_centavos, occurred_at, 'EXTERNAL' AS source_kind FROM org_fuelings WHERE org_id=$1::uuid
		  UNION ALL
		  SELECT id, asset_id, product, unit, quantity_milli AS quantity, cost_centavos AS paid_centavos, occurred_at, 'INTERNAL_TANK' AS source_kind FROM org_internal_fuelings WHERE org_id=$1::uuid AND review_state='APPROVED'
		) s ORDER BY occurred_at DESC, id LIMIT 200`, org)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, aid, prod, unit, src string
		var qty, cents int64
		var at time.Time
		if err := rows.Scan(&id, &aid, &prod, &unit, &qty, &cents, &at, &src); err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("id=%s src=%s asset=%s prod=%s qty=%d(%s) cents=%d at=%s", id, src, aid, prod, qty, unit, cents, at.UTC().Format(time.RFC3339)))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return writePDFLines(sink, lines)
}

func producePeriodSummaryPDF(ctx context.Context, tx pgx.Tx, sink *domain.Sink, org, orgName, orgTZ, exportID string, asOf time.Time) error {
	var opCents, woCount, openFindings int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(company_centavos),0) FROM (
		  SELECT company_centavos FROM org_expenses WHERE org_id=$1::uuid AND review_state IN('APPROVED','SUBMITTED')
		  UNION ALL
		  SELECT company_centavos FROM org_operating_cost_entries WHERE org_id=$1::uuid AND operating_cost AND review_state='APPROVED'
		) x`, org).Scan(&opCents); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_work_orders WHERE org_id=$1::uuid`, org).Scan(&woCount); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM org_findings WHERE org_id=$1::uuid AND state IN('OPEN','IN_REVIEW')`, org).Scan(&openFindings); err != nil {
		return err
	}
	lines := []string{
		"ABASTEVO EMPRESAS - PERIOD SUMMARY REPORT",
		fmt.Sprintf("Org: %s (%s) TZ: %s", orgName, org, orgTZ),
		fmt.Sprintf("Export: %s AsOf: %s Method: kpi-v1", exportID, asOf.UTC().Format(time.RFC3339)),
		fmt.Sprintf("Operating Cost Approved+Pending (centavos): %d", opCents),
		fmt.Sprintf("Work Orders Count: %d", woCount),
		fmt.Sprintf("Open Findings Count: %d", openFindings),
	}
	return writePDFLines(sink, lines)
}

type zipSinkWriter struct{ sink *domain.Sink }

func (w zipSinkWriter) Write(p []byte) (int, error) { return w.sink.Write(p) }

func produceFullZIP(ctx context.Context, tx pgx.Tx, sink *domain.Sink, org, orgName, exportID, memberID string, asOf, generatedAt time.Time) error {
	zw := zip.NewWriter(zipSinkWriter{sink: sink})
	type entrySpec struct {
		file  string
		query string
		cols  []string
	}
	entries := []entrySpec{
		{file: "tanks.csv", query: `SELECT id::text, name, product, unit, quantity_milli::text, value_centavos::text, state FROM org_fuel_tanks WHERE org_id=$1::uuid ORDER BY name, id`, cols: []string{"id", "name", "product", "unit", "quantity_milli", "value_centavos", "state"}},
		{file: "work_orders.csv", query: `SELECT id::text, number::text, asset_id::text, kind, state, COALESCE(paid_cents,0)::text FROM org_work_orders WHERE org_id=$1::uuid ORDER BY number, id`, cols: []string{"id", "number", "asset_id", "kind", "state", "paid_cents"}},
		{file: "findings.csv", query: `SELECT id::text, finding_type, subject_key, severity, state, method_version FROM org_findings WHERE org_id=$1::uuid ORDER BY created_at, id`, cols: []string{"id", "finding_type", "subject_key", "severity", "state", "method_version"}},
		{file: "audit_events.csv", query: `SELECT operation_id::text, action, actor_id::text, target_type, target_id::text, recorded_at::text FROM org_audit_events WHERE org_id=$1::uuid ORDER BY recorded_at, operation_id LIMIT 1000`, cols: []string{"operation_id", "action", "actor_id", "target_type", "target_id", "recorded_at"}},
	}
	type manifestFile struct {
		Name   string `json:"name"`
		Rows   int64  `json:"rows"`
		Bytes  int64  `json:"bytes"`
		SHA256 string `json:"sha256"`
	}
	mFiles := make([]manifestFile, 0, len(entries))
	totalRows := int64(0)
	for _, spec := range entries {
		var buf bytes.Buffer
		buf.WriteString(domain.ByteOrderMark)
		hdr, err := csvLineBytes(spec.cols)
		if err != nil {
			return err
		}
		buf.Write(hdr)
		rows, err := tx.Query(ctx, spec.query, org)
		if err != nil {
			return err
		}
		rowCount := int64(0)
		for rows.Next() {
			vals := make([]any, len(spec.cols))
			ptrs := make([]any, len(spec.cols))
			for i := range vals {
				var s string
				vals[i] = &s
				ptrs[i] = vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				rows.Close()
				return err
			}
			cells := make([]string, len(spec.cols))
			for i := range vals {
				cells[i] = domain.InertText(*vals[i].(*string))
			}
			line, err := csvLineBytes(cells)
			if err != nil {
				rows.Close()
				return err
			}
			buf.Write(line)
			if err := sink.CountRow(); err != nil {
				rows.Close()
				return err
			}
			rowCount++
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		totalRows += rowCount
		raw := buf.Bytes()
		sum := sha256.Sum256(raw)
		w, err := zw.CreateHeader(&zip.FileHeader{Name: spec.file, Method: zip.Deflate, Modified: asOf.UTC()})
		if err != nil {
			return err
		}
		if _, err := w.Write(raw); err != nil {
			return err
		}
		mFiles = append(mFiles, manifestFile{Name: spec.file, Rows: rowCount, Bytes: int64(len(raw)), SHA256: hex.EncodeToString(sum[:])})
	}
	slices.SortFunc(mFiles, func(a, b manifestFile) int { return strings.Compare(a.Name, b.Name) })
	manifestRaw, _ := json.MarshalIndent(map[string]any{
		"schema":                  "orgexport.full_zip.v1",
		"kind":                    FullOrganizationZIPKind,
		"export_id":               exportID,
		"organization_id":         org,
		"organization_name":       orgName,
		"requester_membership_id": memberID,
		"as_of":                   asOf.UTC().Format(time.RFC3339Nano),
		"generated_at":            generatedAt.UTC().Format(time.RFC3339Nano),
		"row_count":               totalRows,
		"files":                   mFiles,
	}, "", "  ")
	mw, err := zw.CreateHeader(&zip.FileHeader{Name: domain.ListManifestName, Method: zip.Deflate, Modified: asOf.UTC()})
	if err != nil {
		return err
	}
	if _, err := mw.Write(manifestRaw); err != nil {
		return err
	}
	return zw.Close()
}
