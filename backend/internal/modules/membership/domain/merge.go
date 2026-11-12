package domain

import (
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"math"
)

var ErrMergeConflict = errors.New("membership: employee merge conflict")

const MaxEmployeeMergeTeams = 500
const MaxEmployeeMergeDepth = 64

func PlanEmployeeMerge(scope kernel.OrgScope, source, survivor Employee, expectedSource, expectedSurvivor, sourceLive, survivorLive, teams int64, reason string) (Employee, error) {
	if e := ValidateAccessChangeReason(reason); e != nil {
		return Employee{}, e
	}
	a, e := source.Snapshot(scope)
	if e != nil {
		return Employee{}, e
	}
	b, e := survivor.Snapshot(scope)
	if e != nil {
		return Employee{}, e
	}
	if a.ID == b.ID || a.MergedInto != "" || b.MergedInto != "" || sourceLive != 0 || survivorLive < 0 || survivorLive > 1 || teams < 0 || teams > MaxEmployeeMergeTeams || teams > 0 && b.State != EmployeeActive {
		return Employee{}, ErrMergeConflict
	}
	if a.Revision != expectedSource || b.Revision != expectedSurvivor {
		return Employee{}, ErrConflict
	}
	if a.Revision == math.MaxInt64 {
		return Employee{}, ErrRevision
	}
	a.State = EmployeeTerminated
	a.MergedInto = b.ID
	a.Revision++
	return RehydrateEmployee(scope, a)
}
