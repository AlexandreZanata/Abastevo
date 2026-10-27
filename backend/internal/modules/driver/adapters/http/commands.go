package http

import (
	"context"
	"encoding/json"
	"net/http"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

type CommandStore interface {
	Save(context.Context, string, app.SaveEarning) (app.Receipt, error)
	ApplyShift(context.Context, string, app.ApplyShift) (domain.ShiftTransition, error)
	SaveCostProfile(context.Context, string, app.SaveCostProfile) (app.CostProfileReceipt, error)
	EraseRecord(context.Context, string, string, int64, int64) error
	EraseCostProfile(context.Context, string, string, int64, int64) error
}

func (h Handler) commands(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		httpapi.WriteError(w, r, 404, "driver.disabled", "private driver routes are disabled", nil)
		return
	}
	var dto struct {
		session
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := h.read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session)
	if err != nil {
		fail(w, r, err)
		return
	}
	if h.Commands == nil || len(dto.Payload) == 0 {
		fail(w, r, app.ErrUnavailable)
		return
	}
	switch dto.Kind {
	case "EARNING_SAVE":
		var in app.SaveEarning
		if json.Unmarshal(dto.Payload, &in) != nil {
			fail(w, r, domain.ErrInvalid)
			return
		}
		got, err := h.Commands.Save(r.Context(), owner, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.write(w, r, got)
	case "EARNING_ERASE":
		var in struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
			Epoch    int64  `json:"epoch"`
		}
		if json.Unmarshal(dto.Payload, &in) != nil {
			fail(w, r, domain.ErrInvalid)
			return
		}
		if err := h.Commands.EraseRecord(r.Context(), owner, in.ID, in.Revision, in.Epoch); err != nil {
			fail(w, r, err)
			return
		}
		h.write(w, r, map[string]bool{"deleted": true})
	case "SHIFT_APPLY":
		var in app.ApplyShift
		if json.Unmarshal(dto.Payload, &in) != nil {
			fail(w, r, domain.ErrInvalid)
			return
		}
		got, err := h.Commands.ApplyShift(r.Context(), owner, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.write(w, r, got)
	case "COST_SAVE":
		var in app.SaveCostProfile
		if json.Unmarshal(dto.Payload, &in) != nil {
			fail(w, r, domain.ErrInvalid)
			return
		}
		got, err := h.Commands.SaveCostProfile(r.Context(), owner, in)
		if err != nil {
			fail(w, r, err)
			return
		}
		h.write(w, r, got)
	case "COST_ERASE":
		var in struct {
			VehicleID string `json:"vehicle_id"`
			Version   int64  `json:"version"`
			Epoch     int64  `json:"epoch"`
		}
		if json.Unmarshal(dto.Payload, &in) != nil {
			fail(w, r, domain.ErrInvalid)
			return
		}
		if err := h.Commands.EraseCostProfile(r.Context(), owner, in.VehicleID, in.Version, in.Epoch); err != nil {
			fail(w, r, err)
			return
		}
		h.write(w, r, map[string]bool{"deleted": true})
	default:
		fail(w, r, domain.ErrInvalid)
	}
}
