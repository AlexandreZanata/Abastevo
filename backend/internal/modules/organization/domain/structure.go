package domain

// Counts are resolved from source-owned, current transactional guards. A pure
// value cannot prove that an adapter observed actual usage or held its locks.
type StructureUsage struct{ Assets, Employees, Budgets, Operations int64 }

func (u StructureUsage) Validate() error {
	if u.Assets < 0 || u.Employees < 0 || u.Budgets < 0 || u.Operations < 0 {
		return ErrConditions
	}
	return nil
}
func (u StructureUsage) InUse() bool {
	return u.Assets > 0 || u.Employees > 0 || u.Budgets > 0 || u.Operations > 0
}
