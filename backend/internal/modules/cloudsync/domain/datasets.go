package domain

// Cloud datasets are explicit opt-ins. Driver datasets contain only derived
// owner data; raw GPS/credentials are rejected by payload normalization.
var cloudDatasets = []string{
	"vehicles", "favorites", "preferences",
	"driver_earnings", "driver_distance", "driver_costs", "driver_maintenance",
}

func Datasets() []string { return append([]string(nil), cloudDatasets...) }

func KnownDataset(dataset string) bool {
	for _, candidate := range cloudDatasets {
		if candidate == dataset {
			return true
		}
	}
	return false
}
