package lists

import (
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Vocabularies mirror the stored CHECK values of the owning tables
// (migrations 000170, 000197, 000198).
var (
	assetStates    = []string{"DRAFT", "ACTIVE", "INACTIVE", "DISPOSED", "ARCHIVED"}
	assetKinds     = []string{"VEHICLE", "MACHINE", "IMPLEMENT"}
	meterKinds     = []string{"ODOMETER", "HOUR_METER"}
	readingStates  = []string{"VALID", "SUSPECT", "VOIDED"}
	employeeStates = []string{"ACTIVE", "INACTIVE", "TERMINATED"}
)

func text(name string) domain.Column { return domain.Column{Name: name, Type: domain.TextColumn} }
func note(c domain.Column, n string) domain.Column {
	c.Note = n
	return c
}
func integer(name, unit, n string) domain.Column {
	return domain.Column{Name: name, Type: domain.IntegerColumn, Unit: unit, Note: n}
}
func instant(name, n string) domain.Column {
	return domain.Column{Name: name, Type: domain.InstantColumn, Note: n}
}

// exact joins a stored microsecond instant and its sub-microsecond digits;
// a corrupt stored value is a retryable failure.
func exact(at pgtype.Timestamptz, sub int16) (domain.Cell, error) {
	if !at.Valid {
		if sub != 0 {
			return domain.Cell{}, app.ErrUnavailable
		}
		return domain.Empty(), nil
	}
	if at.InfinityModifier != pgtype.Finite || sub < 0 || sub > 999 || at.Time.Nanosecond()%1000 != 0 {
		return domain.Cell{}, app.ErrUnavailable
	}
	return domain.Instant(at.Time.UTC().Add(time.Duration(sub) * time.Nanosecond)), nil
}

// canonicalInstant accepts only the canonical UTC form of the shared profile.
func canonicalInstant(raw string) (time.Time, bool) {
	at, err := orgtime.ParseInstant(raw)
	return at, err == nil && orgtime.FormatInstant(at) == raw
}

// bound splits an exact instant into the stored microsecond and digits, or
// a NULL bound when absent.
func bound(raw string) (pgtype.Timestamptz, int16) {
	if raw == "" {
		return pgtype.Timestamptz{}, 0
	}
	at, _ := canonicalInstant(raw)
	return pgtype.Timestamptz{Time: at.Truncate(time.Microsecond), Valid: true}, int16(at.Nanosecond() % 1000)
}

// assetList is the asset register (fleet.asset.read).
type assetList struct{}

func (assetList) spec() domain.ListSpec {
	return domain.ListSpec{
		Kind: AssetListKind, File: "assets.csv", ReadPermission: "fleet.asset.read",
		Columns: []domain.Column{
			text("asset_id"), text("kind"), text("name"), text("fleet_number"), text("plate"), text("serial_number"), text("usage_basis"),
			text("category_id"), text("category_label"), text("state"), integer("revision", "count", "Register revision of the asset."),
			note(text("unit_id"), "Unit allocation period valid at generated_at."), note(text("cost_centre_id"), "Cost-centre allocation period valid at generated_at."),
			instant("acquired_at", "Recorded acquisition instant."), text("disposal_kind"), instant("disposed_at", "Recorded disposal instant."), text("disposal_reason"),
		},
		Review: domain.NotApplicable, ReviewLabel: "cadastro atual (sem fluxo de aprovação)",
		ContentBasis: "Asset register as stored at generated_at, every state except ARCHIVED unless a state filter is given; tanks and expected consumption are not part of the list.",
		Ordering:     []string{"asset_id"},
	}
}

func (assetList) filter(key, value string) bool {
	switch key {
	case "state":
		return oneOf(value, assetStates...)
	case "kind":
		return oneOf(value, assetKinds...)
	}
	return false
}

func (assetList) consistent(filters) bool { return true }

const assetStatement = `SELECT a.id::text,a.kind,a.name,a.fleet_number,a.plate,a.serial_number,a.usage_basis,a.category_id::text,c.label,a.state,a.revision,
 COALESCE(u.unit_id::text,''),COALESCE(cc.cost_centre_id::text,''),a.acquired_at,a.acquired_submicro,COALESCE(a.disposal_kind,''),a.disposed_at,a.disposed_submicro,COALESCE(a.disposal_reason,'')
FROM org_assets a
JOIN org_asset_categories c ON c.org_id=a.org_id AND c.id=a.category_id
LEFT JOIN org_asset_unit_periods u ON u.org_id=a.org_id AND u.asset_id=a.id AND u.valid_from<=$2::timestamptz AND(u.valid_to IS NULL OR $2::timestamptz<u.valid_to)
LEFT JOIN org_asset_cost_centre_periods cc ON cc.org_id=a.org_id AND cc.asset_id=a.id AND cc.valid_from<=$2::timestamptz AND(cc.valid_to IS NULL OR $2::timestamptz<cc.valid_to)
WHERE a.org_id=$1::uuid AND(($3::text='' AND a.state<>'ARCHIVED') OR a.state=$3::text) AND($4::text='' OR a.kind=$4::text)
ORDER BY a.id LIMIT $5::bigint`

func (assetList) statement(org string, f filters, _, generated time.Time, limit int64) (string, []any) {
	return assetStatement, []any{org, generated, f["state"], f["kind"], limit}
}

func (assetList) row(rows pgx.Rows) ([]domain.Cell, error) {
	var id, kind, name, fleet, plate, serial, basis, category, label, state, unit, centre, disposal, reason string
	var revision int64
	var acquired, disposed pgtype.Timestamptz
	var acquiredSub, disposedSub int16
	if err := rows.Scan(&id, &kind, &name, &fleet, &plate, &serial, &basis, &category, &label, &state, &revision, &unit, &centre, &acquired, &acquiredSub, &disposal, &disposed, &disposedSub, &reason); err != nil {
		return nil, err
	}
	acquiredAt, err := exact(acquired, acquiredSub)
	if err != nil {
		return nil, err
	}
	disposedAt, err := exact(disposed, disposedSub)
	if err != nil {
		return nil, err
	}
	return []domain.Cell{domain.Text(id), domain.Text(kind), domain.Text(name), domain.Text(fleet), domain.Text(plate), domain.Text(serial), domain.Text(basis),
		domain.Text(category), domain.Text(label), domain.Text(state), domain.Integer(revision), domain.Text(unit), domain.Text(centre), acquiredAt,
		domain.Text(disposal), disposedAt, domain.Text(reason)}, nil
}

// readingList is the accepted meter readings (fleet.asset.read, as every
// meter read).
type readingList struct{}

func (readingList) spec() domain.ListSpec {
	return domain.ListSpec{
		Kind: ReadingListKind, File: "meter_readings.csv", ReadPermission: "fleet.asset.read",
		Columns: []domain.Column{
			text("reading_id"), text("asset_id"), text("meter_id"), text("meter_kind"),
			integer("value_m", "m", "Odometer readings only, exact metres."), integer("value_s", "s", "Hour-meter readings only, exact seconds."),
			instant("occurred_at", "Declared occurrence instant with its stored sub-microsecond digits."), note(text("entry_zone"), "IANA time zone declared at entry."),
			integer("device_offset_s", "s", "Device clock offset from UTC declared at entry."), instant("received_at", "Server receipt instant."),
			text("origin"), text("method"), note(text("state"), "State as of as_of: the latest classification history revision recorded at or before as_of, else the recorded classification (EMP-METER-004)."), text("author_membership_id"), text("capture_kind"),
		},
		Omitted: []domain.Omission{{Column: "capture_locator", Reason: "CAPTURE_LOCATOR",
			Note: "The web session or device enrolment that captured the reading is server-side provenance and is not exported (BR-PRIV-008)."}},
		Review: domain.ApprovedOnly, ReviewLabel: "somente aprovados",
		ContentBasis: "Server-accepted meter readings received at or before as_of; readings held for review are not included.",
		Ordering:     []string{"occurred_at", "reading_id"},
	}
}

func (readingList) filter(key, value string) bool {
	switch key {
	case "asset_id":
		return domain.ValidID(value)
	case "meter_kind":
		return oneOf(value, meterKinds...)
	case "state":
		return oneOf(value, readingStates...)
	case "occurred_from", "occurred_to":
		_, ok := canonicalInstant(value)
		return ok
	}
	return false
}

func (readingList) consistent(f filters) bool {
	if f["occurred_from"] == "" || f["occurred_to"] == "" {
		return true
	}
	from, _ := canonicalInstant(f["occurred_from"])
	to, _ := canonicalInstant(f["occurred_to"])
	return from.Before(to)
}

// The state is the reading's classification as of as_of (EMP-METER-004):
// the latest history revision recorded at or before as_of, else the
// recorded classification.
const readingStatement = `SELECT r.id::text,m.asset_id::text,r.meter_id::text,m.kind,r.value,r.occurred_at,r.occurred_submicro,r.entry_zone,r.device_offset_seconds,r.received_at,r.origin,r.method,c.state,r.author_membership_id::text,r.capture_kind
FROM org_meter_readings r JOIN org_meters m ON m.org_id=r.org_id AND m.id=r.meter_id
CROSS JOIN LATERAL(SELECT COALESCE((SELECT s.state FROM org_meter_reading_states s WHERE s.org_id=r.org_id AND s.meter_id=r.meter_id AND s.reading_id=r.id AND s.recorded_at<=$2::timestamptz ORDER BY s.revision DESC LIMIT 1),r.state) AS state) c
WHERE r.org_id=$1::uuid AND r.received_at<=$2::timestamptz
 AND($3::text='' OR m.asset_id=CAST(NULLIF($3::text,'') AS uuid)) AND($4::text='' OR m.kind=$4::text) AND($5::text='' OR c.state=$5::text)
 AND($6::timestamptz IS NULL OR(r.occurred_at,r.occurred_submicro)>=($6::timestamptz,$7::smallint))
 AND($8::timestamptz IS NULL OR(r.occurred_at,r.occurred_submicro)<($8::timestamptz,$9::smallint))
ORDER BY r.occurred_at,r.occurred_submicro,r.id LIMIT $10::bigint`

func (readingList) statement(org string, f filters, asOf, _ time.Time, limit int64) (string, []any) {
	from, fromSub := bound(f["occurred_from"])
	to, toSub := bound(f["occurred_to"])
	return readingStatement, []any{org, asOf, f["asset_id"], f["meter_kind"], f["state"], from, fromSub, to, toSub, limit}
}

func (readingList) row(rows pgx.Rows) ([]domain.Cell, error) {
	var id, asset, meterID, kind, zone, origin, method, state, author, capture string
	var value, offset int64
	var occurred, received pgtype.Timestamptz
	var occurredSub int16
	if err := rows.Scan(&id, &asset, &meterID, &kind, &value, &occurred, &occurredSub, &zone, &offset, &received, &origin, &method, &state, &author, &capture); err != nil {
		return nil, err
	}
	occurredAt, err := exact(occurred, occurredSub)
	if err != nil || !occurred.Valid {
		return nil, app.ErrUnavailable
	}
	receivedAt, err := exact(received, 0)
	if err != nil || !received.Valid {
		return nil, app.ErrUnavailable
	}
	metres, seconds := domain.Empty(), domain.Empty()
	switch kind {
	case "ODOMETER":
		metres = domain.Integer(value)
	case "HOUR_METER":
		seconds = domain.Integer(value)
	default:
		return nil, app.ErrUnavailable
	}
	return []domain.Cell{domain.Text(id), domain.Text(asset), domain.Text(meterID), domain.Text(kind), metres, seconds, occurredAt, domain.Text(zone), domain.Integer(offset), receivedAt,
		domain.Text(origin), domain.Text(method), domain.Text(state), domain.Text(author), domain.Text(capture)}, nil
}

// employeeList is the employee register (idn.employee.read) without work
// contact.
type employeeList struct{}

func (employeeList) spec() domain.ListSpec {
	return domain.ListSpec{
		Kind: EmployeeListKind, File: "employees.csv", ReadPermission: "idn.employee.read",
		Columns: []domain.Column{
			text("employee_id"), text("state"), note(text("merged_into"), "Surviving employee of a merge."),
			note(text("access_state"), "Current non-ended membership state, or NONE."), text("display_name"), text("registration_number"), text("job_title"), text("home_unit_id"),
			integer("identity_revision", "count", "Revision of the employee identity."), integer("profile_revision", "count", "Empty while the profile is pending."),
		},
		Omitted: []domain.Omission{
			{Column: "work_email", Reason: "PURPOSE_LIMITED", Note: "Work contact is collected only to hand over invitations and codes (BR-PRIV-005); no export permission covers it."},
			{Column: "work_phone", Reason: "PURPOSE_LIMITED", Note: "Work contact is collected only to hand over invitations and codes (BR-PRIV-005); no export permission covers it."},
		},
		Review: domain.NotApplicable, ReviewLabel: "cadastro atual (sem fluxo de aprovação)",
		ContentBasis: "Employee register as stored at generated_at, every state unless a state filter is given.",
		Ordering:     []string{"employee_id"},
	}
}

func (employeeList) filter(key, value string) bool {
	return key == "state" && oneOf(value, employeeStates...)
}

func (employeeList) consistent(filters) bool { return true }

const employeeStatement = `SELECT e.id::text,e.state,COALESCE(e.merged_into::text,''),
 COALESCE((SELECT m.state FROM org_memberships m WHERE m.org_id=e.org_id AND m.employee_id=e.id AND m.state<>'ENDED'),'NONE'),
 p.employee_id IS NOT NULL,COALESCE(p.display_name,''),COALESCE(p.registration_number,''),COALESCE(p.job_title,''),COALESCE(p.home_unit_id::text,''),e.revision,COALESCE(p.revision,0)
FROM org_employee_identities e LEFT JOIN org_employee_profiles p ON p.org_id=e.org_id AND p.employee_id=e.id
WHERE e.org_id=$1::uuid AND($2::text='' OR e.state=$2::text)
ORDER BY e.id LIMIT $3::bigint`

func (employeeList) statement(org string, f filters, _, _ time.Time, limit int64) (string, []any) {
	return employeeStatement, []any{org, f["state"], limit}
}

func (employeeList) row(rows pgx.Rows) ([]domain.Cell, error) {
	var id, state, merged, access, name, registration, title, unit string
	var profiled bool
	var identityRevision, profileRevision int64
	if err := rows.Scan(&id, &state, &merged, &access, &profiled, &name, &registration, &title, &unit, &identityRevision, &profileRevision); err != nil {
		return nil, err
	}
	revision := domain.Empty()
	if profiled {
		revision = domain.Integer(profileRevision)
	}
	return []domain.Cell{domain.Text(id), domain.Text(state), domain.Text(merged), domain.Text(access), domain.Text(name), domain.Text(registration), domain.Text(title),
		domain.Text(unit), domain.Integer(identityRevision), revision}, nil
}
