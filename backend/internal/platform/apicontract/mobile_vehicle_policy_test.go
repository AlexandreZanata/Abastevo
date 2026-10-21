package apicontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// These are staged contract examples, not runtime slot or transaction tests.
func TestMobileVehicleRegistrationContract(t *testing.T) {
	var policy struct {
		Version      int `json:"version"`
		Ceiling      int `json:"max_vehicles_per_account"`
		Registration struct {
			Version    int    `json:"version"`
			State      string `json:"state"`
			Free       int    `json:"free_vehicle_limit"`
			Premium    int    `json:"premium_vehicle_limit"`
			Capability string `json:"additional_vehicle_capability"`
			Count      string `json:"count_basis"`
			Retained   bool   `json:"preserve_held_data"`
			Runtime    bool   `json:"runtime_enforced"`
		} `json:"vehicle_registration_amendment"`
	}
	read := func(path string, into any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(contractsDir(t), path))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, into); err != nil {
			t.Fatal(err)
		}
	}
	read("premium/policy-v1.json", &policy)
	r := policy.Registration
	if policy.Version != 1 || policy.Ceiling != 3 || r.Version != 2 || r.Free != 1 || r.Premium != policy.Ceiling || r.Capability != "vehicles.multiple" || r.Count != "distinct_non_deleted_vehicle_ids_in_owner_partition" || !r.Retained || r.Runtime || r.State != "CONTRACT_FROZEN_RUNTIME_PENDING" {
		t.Fatal("missing staged one-free amendment or changed legacy ceiling/retained rights")
	}
	var vectors struct {
		Version int `json:"version"`
		Cases   []struct {
			ID        string `json:"id"`
			Before    int    `json:"registered_before"`
			Added     int    `json:"new_distinct_ids"`
			Scope     string `json:"scope"`
			Authority string `json:"authority"`
			Outcome   string `json:"outcome"`
			After     int    `json:"registered_after"`
			Held      bool   `json:"held_data_preserved"`
		} `json:"cases"`
	}
	read("testdata/premium/vehicle-registration-v2.json", &vectors)
	if vectors.Version != r.Version || len(vectors.Cases) < 15 {
		t.Fatal("missing worked examples")
	}
	seen := map[string]bool{}
	for _, v := range vectors.Cases {
		t.Run(v.ID, func(t *testing.T) {
			if v.ID == "" || seen[v.ID] || v.Before < 0 || v.Added < 0 || !v.Held {
				t.Fatal("invalid/duplicate example or data loss")
			}
			seen[v.ID] = true
			// Independent arithmetic oracle for the declared examples only. Hosted
			// admission always needs live cloud authority; local leases never grant it.
			want := "ACCEPT"
			if v.Scope == "HOSTED" && v.Authority != "SERVER_VERIFIED" {
				want = "HOSTED_AUTHORITY_REQUIRED"
			} else if v.Added > 0 && v.Before+v.Added > r.Premium {
				want = "CAPACITY_EXCEEDED"
			} else if v.Added > 0 && v.Before+v.Added > r.Free && v.Authority != "SERVER_VERIFIED" && v.Authority != "LOCAL_VALID_LEASE" {
				want = "PREMIUM_REQUIRED"
			}
			if v.Scope != "LOCAL" && v.Scope != "HOSTED" {
				t.Fatal("unknown scope")
			}
			after := v.Before
			if want == "ACCEPT" {
				after += v.Added
			}
			if v.Outcome != want || v.After != after {
				t.Fatalf("contract example: want %s/%d, got %s/%d", want, after, v.Outcome, v.After)
			}
		})
	}
	for _, id := range []string{"free-first", "free-second", "legacy-two-correct", "legacy-three-read", "expired-three-new", "premium-third", "premium-fourth", "concurrent-free-first", "concurrent-free-second", "import-two-atomic-refusal", "restore-same-ids", "offline-valid", "offline-expired", "clock-rollback", "offline-hosted-refused"} {
		if !seen[id] {
			t.Errorf("missing required example %s", id)
		}
	}
}
