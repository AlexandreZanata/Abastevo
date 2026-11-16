package application

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	exact "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgvalue/domain"
)

// payloadKeys is the exact schema 1 payload (EDR-289). Identity, author,
// competence, receipt, state and scope belong to the envelope or server.
var payloadKeys = []string{"asset_id", "category", "paid_total", "company_amount", "payer", "trip_id"}

// categoryProfile is the EMP-EXP-001 key profile of a category code.
var categoryProfile = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type decoder struct {
	fields      map[string]json.RawMessage
	malformed   bool
	unsupported bool
}

func isNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func (d *decoder) text(key string) string {
	var value string
	if raw, ok := d.fields[key]; !ok || isNull(raw) || json.Unmarshal(raw, &value) != nil {
		d.malformed = true
	}
	return value
}

func (d *decoder) centavos(key string) int64 {
	value, err := exact.ParseCentavos(d.text(key))
	if err != nil {
		d.malformed = true
	}
	return value
}

// ParseOperation decodes one expense.submit schema 1 operation
// (B-BR-EMP-447). ErrMalformed is final; ErrUnsupported keeps a recognised
// future form unavailable.
func ParseOperation(op protocol.Operation) (Command, error) {
	v := op.Snapshot()
	if v.Type != OperationType || !op.SupportsSchema() || v.Entity.Type != EntityType || v.Entity.ExpectedRevision != nil || !canonicalID(v.Entity.ID) {
		return Command{}, ErrMalformed
	}
	org, err := kernel.ParseOrgID(v.OrgID)
	if err != nil {
		return Command{}, ErrMalformed
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return Command{}, ErrMalformed
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(op.PayloadBytes(), &fields) != nil || fields == nil || len(fields) != len(payloadKeys) {
		return Command{}, ErrMalformed
	}
	for _, key := range payloadKeys {
		if _, ok := fields[key]; !ok {
			return Command{}, ErrMalformed
		}
	}
	d := &decoder{fields: fields}
	asset, category, payer := d.text("asset_id"), d.text("category"), d.text("payer")
	paid, company := d.centavos("paid_total"), d.centavos("company_amount")
	if !canonicalID(asset) || !categoryProfile.MatchString(category) || category == FuelCategory || payer != string(fueling.Company) && payer != string(fueling.Employee) {
		d.malformed = true
	}
	if raw := fields["trip_id"]; !isNull(raw) {
		var trip string
		if json.Unmarshal(raw, &trip) != nil || !canonicalID(trip) {
			d.malformed = true
		} else {
			d.unsupported = true
		}
	}
	if !slices.Contains(DirectCategories(), category) {
		d.unsupported = true
	}
	if len(v.Attachments) != 0 || len(v.DependsOn) != 0 || len(v.SoftReferences) != 0 {
		d.unsupported = true
	}
	if d.malformed {
		return Command{}, ErrMalformed
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return Command{}, ErrMalformed
	}
	declared := orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds}
	// The pure expense value proves scope, exact amounts (company <= paid)
	// and the declaration; the reporting day is attributed at write time
	// from the server's zones.
	value, err := expenseValue(scope, v.Entity.ID, string(domain.KindDirect), v.Entity.ID, category, paid, company, declared, orgtime.ReportingZones{Organization: "UTC"})
	if err != nil {
		return Command{}, ErrMalformed
	}
	if d.unsupported {
		return Command{}, ErrUnsupported
	}
	validated := value.Competence()
	return Command{valid: true, value: CommandInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), ExpenseID: v.Entity.ID, AssetID: asset,
		AuthorMembershipID: v.MembershipID, Declared: validated, Category: category, Payer: payer, Paid: paid, Company: company}}, nil
}
