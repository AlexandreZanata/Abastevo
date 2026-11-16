package application

import (
	"regexp"
	"strconv"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fueling "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// expenseValue builds the EMP-EXP-001 value of one expense fact and returns
// it with its reporting attribution; any refusal is unavailable.
func expenseValue(scope kernel.OrgScope, id, kind, source, category string, paid, company int64, declared orgtime.Declaration, zones orgtime.ReportingZones) (domain.Expense, error) {
	reference, err := domain.NewExpenseRef(scope, id)
	if err != nil {
		return domain.Expense{}, ErrUnavailable
	}
	originKind, err := domain.ParseOriginKind(kind)
	if err != nil {
		return domain.Expense{}, ErrUnavailable
	}
	var origin domain.Origin
	if originKind == domain.KindFueling {
		ref, err := fueling.NewFuelingRef(scope, source)
		if err != nil {
			return domain.Expense{}, ErrUnavailable
		}
		origin, err = domain.NewFuelingOrigin(scope, ref)
		if err != nil {
			return domain.Expense{}, ErrUnavailable
		}
	} else if origin, err = domain.NewOrigin(scope, originKind, nil); err != nil || source != id {
		return domain.Expense{}, ErrUnavailable
	}
	value, err := domain.ParseCategory(category)
	if err != nil {
		return domain.Expense{}, ErrUnavailable
	}
	paidText, companyText := strconv.FormatInt(paid, 10), strconv.FormatInt(company, 10)
	amounts, err := domain.NewAmounts(&paidText, &companyText)
	if err != nil {
		return domain.Expense{}, ErrUnavailable
	}
	expense, err := domain.NewExpense(scope, domain.ExpenseInput{Reference: reference, Origin: origin, Category: value, Amounts: amounts, Competence: &declared, Zones: zones})
	if err != nil {
		return domain.Expense{}, ErrUnavailable
	}
	return expense, nil
}

// categoryFits applies the v1 catalogue of EDR-288 to an origin kind.
func categoryFits(kind, category string) bool {
	switch kind {
	case string(domain.KindFueling):
		return category == FuelCategory
	case string(domain.KindDirect):
		for _, c := range DirectCategories() {
			if c == category {
				return true
			}
		}
	}
	return false
}

func payer(raw string) bool { return raw == string(fueling.Company) || raw == string(fueling.Employee) }

// ProjectFueling decides the one FUELING expense of a stored fueling
// (B-BR-EMP-445): category FUEL, paid and company equal to the fueling's paid
// total, its payer, asset, author, enrollment, operation, digest, declaration
// and snapshot, and the reporting day in the supplied zones. It reads no
// clock and trusts no device value.
func ProjectFueling(scope kernel.OrgScope, expenseID string, source FuelingSource, zones orgtime.ReportingZones) (Record, error) {
	if scope.Validate() != nil || source.Revision != SourceRevision || source.ReviewState != Submitted || !payer(source.Payer) || !digestPattern.MatchString(source.PayloadDigest) ||
		source.CostCentreID != "" && source.UnitID == "" || (source.UnitID == "") != (zones.Unit == nil) {
		return Record{}, ErrUnavailable
	}
	for _, id := range []string{source.ID, source.OperationID, source.AssetID, source.AuthorMembershipID, source.AuthorEmployeeID, source.EnrollmentID} {
		if !canonicalID(id) {
			return Record{}, ErrUnavailable
		}
	}
	for _, id := range []string{source.UnitID, source.CostCentreID} {
		if id != "" && !canonicalID(id) {
			return Record{}, ErrUnavailable
		}
	}
	value, err := expenseValue(scope, expenseID, string(domain.KindFueling), source.ID, FuelCategory, source.Paid, source.Paid, source.Declared, zones)
	if err != nil {
		return Record{}, err
	}
	return Record{ID: expenseID, OriginKind: string(domain.KindFueling), SourceID: source.ID, FuelingID: source.ID, SourceRevision: source.Revision,
		OperationID: source.OperationID, PayloadDigest: source.PayloadDigest, AssetID: source.AssetID, AuthorMembershipID: source.AuthorMembershipID,
		AuthorEmployeeID: source.AuthorEmployeeID, EnrollmentID: source.EnrollmentID, Category: FuelCategory, Payer: source.Payer,
		Paid: source.Paid, Company: source.Paid, Declared: value.Competence(), Reporting: value.Reporting(), UnitID: source.UnitID,
		CostCentreID: source.CostCentreID, ReviewState: Submitted, Revision: 1}, nil
}

// sameCost compares every recognized fact of two FUELING records except the
// stored identity and receipt time (EDR-290).
func sameCost(a, b Record) bool {
	return a.OriginKind == b.OriginKind && a.SourceID == b.SourceID && a.FuelingID == b.FuelingID && a.SourceRevision == b.SourceRevision &&
		a.OperationID == b.OperationID && a.PayloadDigest == b.PayloadDigest && a.AssetID == b.AssetID && a.AuthorMembershipID == b.AuthorMembershipID &&
		a.AuthorEmployeeID == b.AuthorEmployeeID && a.EnrollmentID == b.EnrollmentID && a.Category == b.Category && a.Payer == b.Payer &&
		a.Paid == b.Paid && a.Company == b.Company && a.Declared.OccurredAt.Equal(b.Declared.OccurredAt) && a.Declared.EntryZone == b.Declared.EntryZone &&
		a.Declared.DeviceOffsetSeconds == b.Declared.DeviceOffsetSeconds && a.Reporting == b.Reporting && a.UnitID == b.UnitID &&
		a.CostCentreID == b.CostCentreID && a.ReviewState == b.ReviewState && a.Revision == b.Revision
}

// DecideProjection compares a requested projection with the stored expense
// of its origin (EDR-290): none stored inserts; the same source revision
// with equal facts is unchanged; a stored newer revision is stale; anything
// else is unavailable and never overwritten.
func DecideProjection(requested Record, stored *Record) (ProjectionDecision, error) {
	if requested.OriginKind != string(domain.KindFueling) || !canonicalID(requested.FuelingID) || requested.SourceID != requested.FuelingID || requested.SourceRevision < 1 {
		return "", ErrUnavailable
	}
	if stored == nil {
		return Insert, nil
	}
	if stored.OriginKind != requested.OriginKind || stored.FuelingID != requested.FuelingID || stored.SourceID != requested.SourceID {
		return "", ErrUnavailable
	}
	switch {
	case stored.SourceRevision > requested.SourceRevision:
		return "", ErrStale
	case stored.SourceRevision < requested.SourceRevision || !sameCost(requested, *stored):
		return "", ErrUnavailable
	}
	return Unchanged, nil
}

// Expense re-proves a stored record as an EMP-EXP-001 expense value: its
// origin, category, exact amounts and competence with the stored reporting
// zone (B-BR-EMP-444). Totals and settlement owners consume this value.
func (r Record) Expense(scope kernel.OrgScope) (domain.Expense, error) {
	if r.Revision != 1 || r.SourceRevision != SourceRevision || r.ReviewState != Submitted || !payer(r.Payer) || !categoryFits(r.OriginKind, r.Category) ||
		r.OriginKind == string(domain.KindFueling) && r.SourceID != r.FuelingID || r.OriginKind == string(domain.KindDirect) && (r.FuelingID != "" || r.SourceID != r.ID) {
		return domain.Expense{}, ErrUnavailable
	}
	value, err := expenseValue(scope, r.ID, r.OriginKind, r.SourceID, r.Category, r.Paid, r.Company, r.Declared, orgtime.ReportingZones{Organization: r.Reporting.Zone})
	if err != nil || value.Reporting().Day != r.Reporting.Day || r.Reporting.Source != "organization" && r.Reporting.Source != "unit" || (r.Reporting.Source == "unit") != (r.UnitID != "") {
		return domain.Expense{}, ErrUnavailable
	}
	return value, nil
}
