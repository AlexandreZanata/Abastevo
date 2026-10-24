package http

import (
	"encoding/json"
	"math/big"
	"net/http"

	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// RankHandler serves comparable tariff ranking without flattening time,
// session or parking into an invented universal rate. Callers supply
// tariffs and one scenario; the EE-11 engine prices each exactly;
// results rank by total with stable ties. Unknown prices rank last with
// explicit state (never zero/free defaults). Member tariffs isolate by
// target: mixed-operator member comparisons refuse outright (never leak
// cross-operator discounts).
type RankHandler struct {
	Enabled func() bool
}

type rankRequest struct {
	Tariffs  []chargingdomain.Tariff `json:"tariffs"`
	Scenario chargingdomain.Scenario `json:"scenario"`
}

type rankedEstimate struct {
	Index int    `json:"index"`
	State string `json:"state"`
	Value *struct {
		Numerator   string `json:"numerator"`
		Denominator string `json:"denominator"`
	} `json:"value"`
	Reason string `json:"reason,omitempty"`
}

// RegisterRankRoutes mounts tariff ranking without disturbing charging
// reads or other surfaces.
func (h RankHandler) RegisterRankRoutes(r chi.Router) {
	r.Post("/v1/charging/tariffs/rank", h.rank)
}

func (h RankHandler) rank(w http.ResponseWriter, r *http.Request) {
	if h.Enabled != nil && !h.Enabled() {
		httpapi.WriteError(w, r, http.StatusNotFound, "charging.disabled", "charging reads disabled", nil)
		return
	}
	var request rankRequest
	if httpapi.DecodeJSON(mustReadBody(r), &request) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-rank", "ranking refused", nil)
		return
	}
	if len(request.Tariffs) == 0 || len(request.Tariffs) > 50 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-rank", "tariff count out of bounds", nil)
		return
	}
	// Member isolation by target: member tariffs must share one target ID
	// in a single ranking; mixed targets refuse (no cross-operator leak).
	memberTarget := ""
	for _, tariff := range request.Tariffs {
		if tariff.Condition == "MEMBER" {
			if memberTarget == "" {
				memberTarget = tariff.TargetID
			} else if tariff.TargetID != memberTarget {
				httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-rank", "member isolation",
					[]httpapi.Detail{{Field: "tariffs", Code: "scope"}})
				return
			}
		}
	}
	type outcome struct {
		index    int
		value    *big.Rat
		state    string
		reason   string
		original chargingdomain.Estimate
	}
	outcomes := make([]outcome, 0, len(request.Tariffs))
	for i, tariff := range request.Tariffs {
		estimate, err := chargingdomain.Calculate(tariff, request.Scenario)
		if err != nil || estimate.Value == nil {
			reason := ""
			if err != nil {
				reason = err.Error()
			}
			outcomes = append(outcomes, outcome{index: i, state: "UNKNOWN", reason: reason, original: estimate})
			continue
		}
		numerator, okN := new(big.Int).SetString(estimate.Value.Numerator, 10)
		denominator, okD := new(big.Int).SetString(estimate.Value.Denominator, 10)
		if !okN || !okD || denominator.Sign() == 0 {
			outcomes = append(outcomes, outcome{index: i, state: "UNKNOWN", reason: "unrepresentable", original: estimate})
			continue
		}
		outcomes = append(outcomes, outcome{index: i, value: new(big.Rat).SetFrac(numerator, denominator), state: "KNOWN", original: estimate})
	}
	// Stable sort: known totals ascending, unknowns last by input order.
	// Exact rational comparison; no float approximation for ordering.
	for a := 1; a < len(outcomes); a++ {
		for b := a; b > 0; b-- {
			less := false
			if outcomes[b].state != outcomes[b-1].state {
				less = outcomes[b].state == "KNOWN"
			} else if outcomes[b].state == "KNOWN" {
				comparison := outcomes[b].value.Cmp(outcomes[b-1].value)
				less = comparison < 0 || (comparison == 0 && outcomes[b].index < outcomes[b-1].index)
			} else {
				less = outcomes[b].index < outcomes[b-1].index
			}
			if !less {
				break
			}
			outcomes[b], outcomes[b-1] = outcomes[b-1], outcomes[b]
		}
	}
	wire := make([]rankedEstimate, 0, len(outcomes))
	for _, o := range outcomes {
		item := rankedEstimate{Index: o.index, State: o.state, Reason: o.reason}
		if o.value != nil && o.original.Value != nil {
			item.Value = &struct {
				Numerator   string `json:"numerator"`
				Denominator string `json:"denominator"`
			}{Numerator: o.original.Value.Numerator, Denominator: o.original.Value.Denominator}
		}
		wire = append(wire, item)
	}
	body, _ := json.Marshal(map[string]any{"items": wire})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func mustReadBody(r *http.Request) []byte {
	raw := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			raw = append(raw, buf[:n]...)
			if len(raw) > 65536 {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return raw
}
