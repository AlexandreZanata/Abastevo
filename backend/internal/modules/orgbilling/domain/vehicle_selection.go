package domain

import "errors"

var ErrVehicleSelection = errors.New("organization billing: vehicle selection does not match the current fleet and limit")

// SelectPlanArchives applies one complete keep-active selection to the current
// fleet snapshot. The caller supplies the unique active vehicle IDs in the
// required audit order while holding the owning fleet lock.
func SelectPlanArchives(active, keep []string, limit int64) ([]string, error) {
	if limit < 0 || int64(len(keep)) != limit || int64(len(active)) <= limit {
		return nil, ErrVehicleSelection
	}
	selected := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		selected[id] = struct{}{}
	}
	if len(selected) != len(keep) {
		return nil, ErrVehicleSelection
	}
	archive := make([]string, 0, len(active)-len(keep))
	for _, id := range active {
		if _, found := selected[id]; !found {
			archive = append(archive, id)
		}
	}
	if len(archive) != len(active)-len(keep) {
		return nil, ErrVehicleSelection
	}
	return archive, nil
}
