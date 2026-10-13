package application

import (
	"context"
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"time"
)

type Principal struct{ AccountID, FamilyID, ContributorID, KeyID, DeviceID string }
type Record struct {
	Dataset  string          `json:"dataset"`
	ID       string          `json:"id"`
	Revision int64           `json:"revision"`
	Deleted  bool            `json:"deleted"`
	Payload  json.RawMessage `json:"payload"`
}
type VehicleStore interface {
	PutVehicle(context.Context, Principal, domain.Vehicle, int64) (Record, error)
}

type PortableStore interface {
	PutFavorite(context.Context, Principal, string, int64) (Record, error)
	PutPreferences(context.Context, Principal, domain.Preferences, int64) (Record, error)
}

type Device struct {
	ID          string     `json:"id"`
	Origin      string     `json:"origin"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSuccess *time.Time `json:"last_success_at"`
	Revoked     bool       `json:"revoked"`
}
type DeviceStore interface {
	RegisterDevice(context.Context, Principal) (Device, error)
	RevokeDevice(context.Context, Principal, string) error
	CheckDevice(context.Context, Principal) error
}

type Command struct {
	OperationID      string          `json:"operation_id"`
	Dataset          string          `json:"dataset"`
	ID               string          `json:"id"`
	ExpectedRevision int64           `json:"expected_revision"`
	Delete           bool            `json:"delete"`
	Payload          json.RawMessage `json:"payload"`
}
type Batch struct {
	Epoch           int64     `json:"epoch"`
	ConsentRevision int64     `json:"consent_revision"`
	Commands        []Command `json:"commands"`
}
type CommandResult struct {
	OperationID string `json:"operation_id"`
	Dataset     string `json:"dataset"`
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	Status      string `json:"status"`
	Replayed    bool   `json:"replayed"`
}
type BatchResult struct {
	Epoch    int64           `json:"epoch"`
	Revision int64           `json:"revision"`
	Results  []CommandResult `json:"results"`
}
type BatchStore interface {
	Apply(context.Context, Principal, Batch) (BatchResult, error)
}

type Status struct {
	Epoch           int64    `json:"epoch"`
	Revision        int64    `json:"revision"`
	ConsentRevision int64    `json:"consent_revision"`
	EnabledDatasets []string `json:"enabled_datasets"`
}
type ReadStore interface {
	Status(context.Context, Principal) (Status, error)
	List(context.Context, Principal, string, int, string) ([]Record, error)
	Devices(context.Context, Principal, int, string) ([]Device, error)
	Erase(context.Context, Principal, string, string) (int64, error)
}

type Position struct {
	Version         int64 `json:"version"`
	Epoch           int64 `json:"epoch"`
	ConsentRevision int64 `json:"consent_revision"`
	After           int64 `json:"after"`
	Through         int64 `json:"through"`
}
type Change struct {
	Sequence int64           `json:"sequence"`
	Epoch    int64           `json:"epoch"`
	Dataset  string          `json:"dataset"`
	ID       string          `json:"id"`
	Revision int64           `json:"revision"`
	Deleted  bool            `json:"deleted"`
	Payload  json.RawMessage `json:"payload"`
}
type Feed struct {
	Changes         []Change `json:"changes"`
	Through         int64    `json:"through"`
	Epoch           int64    `json:"epoch"`
	ConsentRevision int64    `json:"consent_revision"`
}
type PullStore interface {
	Pull(context.Context, Principal, Position, int) (Feed, error)
}
