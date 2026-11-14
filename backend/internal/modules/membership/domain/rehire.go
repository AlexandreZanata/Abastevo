package domain

import "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"

// PlanRehire reopens the employee only. Access provisioning is a separate action.
func PlanRehire(scope kernel.OrgScope, employee Employee, expected, livePeriods int64, reason string) (Employee, error) {
	if e := ValidateAccessChangeReason(reason); e != nil {
		return Employee{}, e
	}
	v, e := employee.Snapshot(scope)
	if e != nil {
		return Employee{}, e
	}
	if v.MergedInto != "" {
		return Employee{}, ErrMerged
	}
	if v.State != EmployeeTerminated {
		return Employee{}, ErrTransition
	}
	if livePeriods < 0 {
		return Employee{}, ErrInvalid
	}
	if livePeriods > 0 {
		return Employee{}, ErrConflict
	}
	return employee.Transition(scope, expected, EmployeeActive, EmployeeConditions{})
}
