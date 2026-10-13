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

type Conflict struct {
	OperationID      string          `json:"operation_id"`
	Dataset          string          `json:"dataset"`
	ID               string          `json:"id"`
	ExpectedRevision int64           `json:"expected_revision"`
	CurrentRevision  int64           `json:"current_revision"`
	CurrentDeleted   bool            `json:"current_deleted"`
	CurrentPayload   json.RawMessage `json:"current_payload"`
	ProposedPayload  json.RawMessage `json:"proposed_payload"`
	Erased           bool            `json:"erased"`
}
type ConflictError struct {
	Conflict Conflict
	Epoch    int64
	Revision int64
}

func (e *ConflictError) Error() string { return "cloud: revision conflict" }
func (e *ConflictError) Unwrap() error { return domain.ErrConflict }

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

type PreviewItem struct {
	OperationID      string `json:"operation_id"`
	Dataset          string `json:"dataset"`
	ID               string `json:"id"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
	CurrentRevision  int64  `json:"current_revision"`
	CurrentDeleted   bool   `json:"current_deleted"`
	Erased           bool   `json:"erased"`
}
type PreviewResult struct {
	Epoch           int64         `json:"epoch"`
	Revision        int64         `json:"revision"`
	ConsentRevision int64         `json:"consent_revision"`
	Items           []PreviewItem `json:"items"`
}
type ImportStore interface {
	Preview(context.Context, Principal, Batch) (PreviewResult, error)
	SetConsent(context.Context, Principal, []string, bool) (Status, error)
}
type SnapshotQuery struct {
	Dataset         string `json:"dataset"`
	After           string `json:"after"`
	Through         int64  `json:"through"`
	Epoch           int64  `json:"epoch"`
	ConsentRevision int64  `json:"consent_revision"`
}
type SnapshotResult struct {
	Records         []Record `json:"records"`
	Through         int64    `json:"through"`
	Epoch           int64    `json:"epoch"`
	ConsentRevision int64    `json:"consent_revision"`
}
type SnapshotStore interface {
	Snapshot(context.Context, Principal, SnapshotQuery, int) (SnapshotResult, error)
}

type BackupManifest struct {
	Version         int64    `json:"version"`
	Revision        int64    `json:"revision"`
	Epoch           int64    `json:"epoch"`
	ConsentRevision int64    `json:"consent_revision"`
	VehicleCount    int64    `json:"vehicle_count"`
	FavoriteCount   int64    `json:"favorite_count"`
	PreferenceCount int64    `json:"preference_count"`
	Checksum        string   `json:"checksum"`
	Datasets        []string `json:"datasets"`
	Excluded        []string `json:"excluded"`
}
type BackupStore interface {
	Manifest(context.Context, Principal) (BackupManifest, error)
}

type SnapshotObjectStore interface {
	Put(ctx context.Context, key string, data []byte) error
	Head(ctx context.Context, key string) (int64, error)
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
}
type StoredSnapshot struct {
	SnapshotID      string `json:"snapshot_id"`
	Revision        int64  `json:"revision"`
	Epoch           int64  `json:"epoch"`
	ConsentRevision int64  `json:"consent_revision"`
	ByteSize        int64  `json:"byte_size"`
	Checksum        string `json:"checksum"`
	Status          string `json:"status"`
	ObjectKey       string `json:"object_key"`
}
type SnapshotStorageStore interface {
	StoreSnapshot(ctx context.Context, p Principal, snapshotID string, revision, epoch, consentRevision int64, data []byte) (StoredSnapshot, error)
	ReadSnapshot(ctx context.Context, p Principal, snapshotID string) ([]byte, error)
	ListSnapshots(ctx context.Context, p Principal, limit int) ([]StoredSnapshot, error)
}
