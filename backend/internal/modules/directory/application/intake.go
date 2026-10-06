package application

import (
	"errors"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// MaxEvidenceRef bounds the opaque private-evidence reference carried
// on a suggestion (a pointer, never bytes).
const MaxEvidenceRef = 256

// MaxSuggestionsPerDay bounds intake per account ([CALIBRATE] with
// measured abuse/need data in P29).
const MaxSuggestionsPerDay = 20

var (
	ErrProposalDisplay   = errors.New("directory: display name is required")
	ErrProposalMunicipal = errors.New("directory: municipality code must be 7 digits")
	ErrProposalState     = errors.New("directory: state must be 2 letters")
	ErrProposalCoords    = errors.New("directory: coordinates must be both present and in range")
	ErrProposalEvidence  = errors.New("directory: evidence reference too long")
)

// ProposalInput is the structured suggestion/correction body.
type ProposalInput struct {
	DisplayName      string
	MunicipalityCode string
	State            string
	CNPJ             string
	Latitude         float64
	Longitude        float64
	HasCoords        bool
	EvidenceRef      string
}

// Proposal is a validated suggestion body.
type Proposal struct {
	DisplayName      string
	MunicipalityCode string
	State            string
	CNPJ             string
	Latitude         float64
	Longitude        float64
	HasCoords        bool
	EvidenceRef      string
}

// ParseProposal validates structured input before any storage. Empty
// display, malformed municipality/state, invalid CNPJ text, partial or
// out-of-range coordinates and oversize evidence references fail here;
// unknown municipalities stay honest data (no geocoder invention).
func ParseProposal(in ProposalInput) (Proposal, error) {
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return Proposal{}, ErrProposalDisplay
	}
	ibge := strings.TrimSpace(in.MunicipalityCode)
	if len(ibge) != 7 {
		return Proposal{}, ErrProposalMunicipal
	}
	for _, r := range ibge {
		if r < '0' || r > '9' {
			return Proposal{}, ErrProposalMunicipal
		}
	}
	uf := strings.ToUpper(strings.TrimSpace(in.State))
	if len(uf) != 2 {
		return Proposal{}, ErrProposalState
	}
	cnpj := ""
	if strings.TrimSpace(in.CNPJ) != "" {
		parsed, err := kernel.ParseCNPJ(in.CNPJ)
		if err != nil {
			return Proposal{}, err
		}
		cnpj = parsed.Normalized()
	}
	if in.HasCoords {
		if in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 {
			return Proposal{}, ErrProposalCoords
		}
		if in.Latitude == 0 && in.Longitude == 0 {
			// Null Island with an explicit flag is contradictory input,
			// not a station: refuse instead of storing a fake point.
			return Proposal{}, ErrProposalCoords
		}
	} else if in.Latitude != 0 || in.Longitude != 0 {
		return Proposal{}, ErrProposalCoords
	}
	if len(in.EvidenceRef) > MaxEvidenceRef {
		return Proposal{}, ErrProposalEvidence
	}
	return Proposal{
		DisplayName: name, MunicipalityCode: ibge, State: uf, CNPJ: cnpj,
		Latitude: in.Latitude, Longitude: in.Longitude, HasCoords: in.HasCoords,
		EvidenceRef: strings.TrimSpace(in.EvidenceRef),
	}, nil
}
