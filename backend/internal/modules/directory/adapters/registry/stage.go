package registry

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
)

// ParserVersion freezes the staging parser identity recorded on every run.
const ParserVersion = "registry-csv-v1"

// SourceCSV is the frozen source key for the ANP registry CSV snapshot.
const SourceCSV = "registry-csv"

// Limits bounds one snapshot attempt. Provisional P25-T01 budgets marked
// [CALIBRATE]; reconfirm with measured data in P25-T02/P29.
type Limits struct {
	MaxBytes  int64
	MaxRows   int
	BatchSize int
}

// DefaultLimits returns the frozen provisional caps.
func DefaultLimits() Limits {
	return Limits{MaxBytes: 100 << 20, MaxRows: 500000, BatchSize: 100}
}

// Report accounts every input row: accepted + duplicates + rejected
// always sum the data rows seen. No silent drops, no invented rows.
type Report struct {
	RunID      string
	State      string
	Accepted   int64
	Duplicates int64
	Rejected   int64
	ErrorCode  string
}

// Store is the staging persistence boundary. The pg implementation
// wraps the generated directory queries; unit tests use a fake.
type Store interface {
	CreateRun(ctx context.Context, id, source, snapshot, checksum string) (runID string, created bool, err error)
	GetRun(ctx context.Context, source, snapshot string) (Report, error)
	FinishRun(ctx context.Context, runID, state string, accepted, duplicates, rejected int64, errorCode string) error
	StageAssertion(ctx context.Context, a Assertion) (accepted bool, err error)
	ListAssertions(ctx context.Context, runID string) ([]Assertion, error)
	SetAssertionStation(ctx context.Context, assertionID, stationID string) error
	SetAssertionSuperseded(ctx context.Context, assertionID, supersededBy string) error
}

// Assertion is one validated source row ready for staging.
type Assertion struct {
	ID               string
	Source           string
	SourceKey        string
	Checksum         string
	DisplayName      string
	Address          map[string]string
	MunicipalityCode string
	State            string
	AuthState        string
	Eligibility      string
	LocationQuality  string
	SourceReference  string
	EffectiveDate    pgtype.Date
	Latitude         float64
	Longitude        float64
	HasCoords        bool
	CRS              string
	runID            string
}

// PGStore implements Store with the generated directory queries.
type PGStore struct {
	Q *directory.Queries
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("registry: rand unavailable")
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func mustUUID(text string) (pgtype.UUID, error) {
	clean := strings.ReplaceAll(text, "-", "")
	raw, err := hex.DecodeString(clean)
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("registry: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func (s *PGStore) CreateRun(ctx context.Context, id, source, snapshot, checksum string) (string, bool, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return "", false, err
	}
	row, err := s.Q.CreateRegistryRun(ctx, directory.CreateRegistryRunParams{
		ID: uid, Source: source, SnapshotIdentity: snapshot,
		Checksum: checksum, ParserVersion: ParserVersion,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return uuidString(row.ID), true, nil
}

func (s *PGStore) GetRun(ctx context.Context, source, snapshot string) (Report, error) {
	row, err := s.Q.GetRegistryRun(ctx, directory.GetRegistryRunParams{
		Source: source, SnapshotIdentity: snapshot,
	})
	if err != nil {
		return Report{}, err
	}
	return Report{
		RunID: uuidString(row.ID), State: row.State,
		Accepted: row.Accepted, Duplicates: row.Duplicates,
		Rejected: row.Rejected, ErrorCode: row.ErrorCode,
	}, nil
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

func (s *PGStore) FinishRun(ctx context.Context, runID, state string, accepted, duplicates, rejected int64, errorCode string) error {
	uid, err := mustUUID(runID)
	if err != nil {
		return err
	}
	_, err = s.Q.FinishRegistryRun(ctx, directory.FinishRegistryRunParams{
		State: state, Accepted: accepted, Duplicates: duplicates,
		Rejected: rejected, ErrorCode: errorCode, ID: uid,
	})
	return err
}

func (s *PGStore) StageAssertion(ctx context.Context, a Assertion) (bool, error) {
	uid, err := mustUUID(newUUID())
	if err != nil {
		return false, err
	}
	runUID, err := mustUUID(a.RunID())
	if err != nil {
		return false, err
	}
	address, err := json.Marshal(a.Address)
	if err != nil {
		return false, err
	}
	id, err := s.Q.StageRegistryAssertion(ctx, directory.StageRegistryAssertionParams{
		ID: uid, RunID: runUID, Source: a.Source, SourceKey: a.SourceKey,
		Checksum: a.Checksum, DisplayName: a.DisplayName, Address: address,
		MunicipalityCode: textOrNull(a.MunicipalityCode), State: textOrNull(a.State),
		AuthState: a.AuthState, Operation: "unknown", Eligibility: a.Eligibility,
		LocationQuality: a.LocationQuality, SourceReference: a.SourceReference,
		EffectiveDate: a.EffectiveDate,
		Latitude:      floatOrNull(a.Latitude, a.HasCoords),
		Longitude:     floatOrNull(a.Longitude, a.HasCoords),
		Crs:           crsOrEmpty(a.CRS, a.HasCoords),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return id.Valid, nil
}

func (s *PGStore) ListAssertions(ctx context.Context, runID string) ([]Assertion, error) {
	uid, err := mustUUID(runID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Q.ListRegistryAssertions(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := make([]Assertion, 0, len(rows))
	for _, row := range rows {
		address := map[string]string{}
		if len(row.Address) > 0 {
			_ = json.Unmarshal(row.Address, &address)
		}
		out = append(out, Assertion{
			ID:               uuidString(row.ID),
			Source:           row.Source,
			SourceKey:        row.SourceKey,
			Checksum:         row.Checksum,
			DisplayName:      row.DisplayName,
			Address:          address,
			MunicipalityCode: row.MunicipalityCode.String,
			State:            row.State.String,
			AuthState:        row.AuthState,
			Eligibility:      row.Eligibility,
			LocationQuality:  row.LocationQuality,
			SourceReference:  row.SourceReference,
			Latitude:         row.Latitude.Float64,
			Longitude:        row.Longitude.Float64,
			HasCoords:        row.Latitude.Valid && row.Longitude.Valid,
			CRS:              row.Crs,
			runID:            uuidString(row.RunID),
		})
	}
	return out, nil
}

func (s *PGStore) SetAssertionStation(ctx context.Context, assertionID, stationID string) error {
	assertionUID, err := mustUUID(assertionID)
	if err != nil {
		return err
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return err
	}
	_, err = s.Q.SetAssertionStation(ctx, directory.SetAssertionStationParams{
		ID:        assertionUID,
		StationID: stationUID,
	})
	return err
}

// SetAssertionSuperseded links staged succession history: an older
// assertion points at its superseding checksum identity. History stays
// auditable; newer evidence never deletes older rows.
func (s *PGStore) SetAssertionSuperseded(ctx context.Context, assertionID, supersededBy string) error {
	assertionUID, err := mustUUID(assertionID)
	if err != nil {
		return err
	}
	supersededUID, err := mustUUID(supersededBy)
	if err != nil {
		return err
	}
	_, err = s.Q.SetAssertionSuperseded(ctx, directory.SetAssertionSupersededParams{
		ID:           assertionUID,
		SupersededBy: supersededUID,
	})
	return err
}

// WithRun attaches the staging run id carried out-of-band from the
// assertion columns.
func (a Assertion) WithRun(runID string) Assertion {
	a.runID = runID
	return a
}

func textOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func floatOrNull(value float64, present bool) pgtype.Float8 {
	if !present {
		return pgtype.Float8{}
	}
	return pgtype.Float8{Float64: value, Valid: true}
}

func crsOrEmpty(crs string, present bool) string {
	if !present {
		return ""
	}
	return crs
}

// StageCSV streams one bounded CSV snapshot into staging. Header
// detection is by name; every row is validated, checksummed and
// accounted. Incomplete/invalid snapshots quarantine or fail the run
// and preserve the last-good publication (staging never publishes).
// Identical replay and concurrent staging converge without duplicates
// via the (source, source_key, checksum) key.
func StageCSV(ctx context.Context, store Store, snapshot string, r io.Reader, limits Limits) (Report, error) {
	raw, err := io.ReadAll(io.LimitReader(r, limits.MaxBytes+1))
	if err != nil {
		return Report{}, err
	}
	if int64(len(raw)) > limits.MaxBytes {
		runID := newUUID()
		if _, _, err := store.CreateRun(ctx, runID, SourceCSV, snapshot, ""); err != nil {
			return Report{}, err
		}
		if err := store.FinishRun(ctx, runID, "failed", 0, 0, 0, "oversize"); err != nil {
			return Report{}, err
		}
		return Report{RunID: runID, State: "failed", ErrorCode: "oversize"}, nil
	}
	sum := sha256.Sum256(raw)
	checksum := hex.EncodeToString(sum[:])
	runID := newUUID()
	if _, created, err := store.CreateRun(ctx, runID, SourceCSV, snapshot, checksum); err != nil {
		return Report{}, err
	} else if !created {
		return store.GetRun(ctx, SourceCSV, snapshot)
	}
	report := Report{RunID: runID, State: "running"}
	finish := func(state, code string) (Report, error) {
		report.State = state
		report.ErrorCode = code
		if err := store.FinishRun(ctx, runID, state, report.Accepted, report.Duplicates, report.Rejected, code); err != nil {
			return Report{}, err
		}
		return report, nil
	}
	reader := csv.NewReader(strings.NewReader(string(raw)))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return finish("quarantined", "empty_snapshot")
	}
	missing, unknown := ValidateHeader(header)
	if QuarantineRun(missing, unknown) {
		code := "unknown_columns"
		if len(missing) > 0 {
			code = "missing_columns"
		}
		return finish("quarantined", code)
	}
	index := map[string]int{}
	for i, name := range header {
		index[strings.ToUpper(strings.TrimSpace(name))] = i
	}
	rows := 0
	for {
		if rows >= limits.MaxRows {
			return finish("failed", "row_cap")
		}
		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return finish("failed", "truncated")
		}
		rows++
		assertion, ok := parseRecord(index, record)
		if !ok {
			report.Rejected++
			continue
		}
		assertion.runID = runID
		accepted, err := store.StageAssertion(ctx, assertion)
		if err != nil {
			return finish("failed", "stage_error")
		}
		if accepted {
			report.Accepted++
		} else {
			report.Duplicates++
		}
	}
	if rows == 0 {
		return finish("quarantined", "empty_snapshot")
	}
	return finish("complete", "")
}

// parseRecord maps one CSV row by header names. It returns false for
// rejected rows (malformed CNPJ, missing identity/address, unknown
// status never rejects by itself — it maps to unknown).
func parseRecord(index map[string]int, record []string) (Assertion, bool) {
	at := func(name string) string {
		i, ok := index[name]
		if !ok || i >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[i])
	}
	cnpj, err := ValidCNPJText(at("CNPJ"))
	if err != nil {
		return Assertion{}, false
	}
	name := at("RAZAO_SOCIAL")
	if name == "" {
		return Assertion{}, false
	}
	ibge := at("COD_IBGE")
	uf := strings.ToUpper(at("UF"))
	if ibge == "" || len(uf) != 2 {
		return Assertion{}, false
	}
	auth := MapStatus(at("SITUACAO"))
	eligibility := DecideEligibility(Record{
		IdentityOK: true, AddressOK: true,
		Authorized: auth == AuthorizationAuthorized,
	})
	sum := sha256.Sum256([]byte(strings.Join(record, "\x1f")))
	return Assertion{
		Source:           SourceCSV,
		SourceKey:        cnpj,
		Checksum:         hex.EncodeToString(sum[:]),
		DisplayName:      name,
		Address:          map[string]string{"municipio_ibge": ibge, "uf": uf},
		MunicipalityCode: ibge,
		State:            uf,
		AuthState:        string(auth),
		Eligibility:      string(eligibility),
		LocationQuality:  "unknown",
		SourceReference:  at("ATO_AUTORIZACAO"),
	}, true
}

// rowRunID carries the staging run id alongside the assertion without
// changing the staged column set.
func (a Assertion) RunID() string { return a.runID }
