package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

type Limits struct {
	redacted
	operations, batchBytes, operationBytes, depth int
}

func NewLimits(operations, batchBytes, operationBytes, depth int) (Limits, error) {
	if operations < 1 || operations > MaxOperations || batchBytes < 1 || batchBytes > MaxBatchBytes || operationBytes < 1 || operationBytes > MaxOperationBytes || operationBytes > batchBytes || depth < 4 || depth > MaxTransportDepth {
		return Limits{}, ErrInvalid
	}
	return Limits{operations: operations, batchBytes: batchBytes, operationBytes: operationBytes, depth: depth}, nil
}
func DefaultLimits() Limits {
	return Limits{operations: MaxOperations, batchBytes: MaxBatchBytes, operationBytes: MaxOperationBytes, depth: MaxTransportDepth}
}
func selectedLimits(options []Limits) (Limits, error) {
	if len(options) == 0 {
		return DefaultLimits(), nil
	}
	if len(options) != 1 {
		return Limits{}, ErrInvalid
	}
	value := options[0]
	return NewLimits(value.operations, value.batchBytes, value.operationBytes, value.depth)
}

type Binding struct {
	redacted
	owner, origin, organization string
}

func NewBinding(owner, origin, organization string) (Binding, error) {
	if !uuid(owner) || !uuid(organization) || ValidateOrigin(origin) != nil {
		return Binding{}, ErrBinding
	}
	return Binding{owner: owner, origin: origin, organization: organization}, nil
}
func (binding Binding) valid() bool {
	return uuid(binding.owner) && uuid(binding.organization) && ValidateOrigin(binding.origin) == nil
}
func uuid(raw string) bool {
	value, err := kernel.ParseOrgID(raw)
	return err == nil && value.String() == raw
}
func shape(value *node, keys ...string) bool {
	if value == nil || value.kind != 'o' || len(value.fields) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := value.fields[key]; !ok {
			return false
		}
	}
	return true
}
func stringValue(value *node) (string, bool) {
	if value == nil || value.kind != 's' {
		return "", false
	}
	return value.value, true
}
func integer(value *node) (int64, bool) {
	if value == nil || value.kind != 'i' {
		return 0, false
	}
	number, err := strconv.ParseInt(value.value, 10, 64)
	return number, err == nil
}
func positive(value *node) (int64, bool) {
	number, ok := integer(value)
	return number, ok && number > 0
}
func optionalRevision(value *node) (*int64, bool) {
	if value != nil && value.kind == 'n' {
		return nil, true
	}
	number, ok := positive(value)
	if !ok {
		return nil, false
	}
	return &number, true
}
func canonicalInstant(raw string, receipt bool) bool {
	at, err := orgtime.ParseInstant(raw)
	return err == nil && orgtime.FormatInstant(at) == raw && (!receipt || !at.IsZero())
}
func commandType(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if !snake(part) {
			return false
		}
	}
	return true
}
func digestText(raw string) bool {
	return len(raw) == 71 && strings.HasPrefix(raw, "sha256:") && bareDigest(raw[7:])
}
func bareDigest(raw string) bool {
	if len(raw) != 64 {
		return false
	}
	for _, c := range []byte(raw) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

var mimePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+\-]*/[a-z0-9][a-z0-9!#$&^_.+\-]*$`)

type Entity struct {
	redacted
	Type, ID         string
	ExpectedRevision *int64
}
type SoftReference struct {
	redacted
	Type, ID string
}
type Attachment struct {
	redacted
	ID, SHA256 string
	Bytes      int64
	MIME       string
}
type Envelope struct {
	redacted
	SchemaVersion                                                                   int64
	OperationID, OwnerAccountID, APIOrigin, OrgID, MembershipID, EnrollmentID, Type string
	OperationSchema                                                                 int64
	Entity                                                                          Entity
	OccurredAt, EntryZone                                                           string
	DeviceOffsetSeconds                                                             int64
	DependsOn                                                                       []string
	SoftReferences                                                                  []SoftReference
	Attachments                                                                     []Attachment
	PayloadDigest                                                                   string
}
type Operation struct {
	redacted
	value              Envelope
	canonical, payload []byte
}

func (operation Operation) CanonicalBytes() []byte {
	return append([]byte(nil), operation.canonical...)
}
func (operation Operation) PayloadBytes() []byte  { return append([]byte(nil), operation.payload...) }
func (operation Operation) PayloadDigest() string { return operation.value.PayloadDigest }
func (operation Operation) ID() string            { return operation.value.OperationID }
func (operation Operation) SupportsSchema() bool {
	return len(operation.canonical) != 0 && operation.value.OperationSchema == 1
}
func (operation Operation) Snapshot() Envelope {
	value := operation.value
	if value.Entity.ExpectedRevision != nil {
		revision := *value.Entity.ExpectedRevision
		value.Entity.ExpectedRevision = &revision
	}
	value.DependsOn = append([]string(nil), value.DependsOn...)
	value.SoftReferences = append([]SoftReference(nil), value.SoftReferences...)
	value.Attachments = append([]Attachment(nil), value.Attachments...)
	return value
}

var operationKeys = []string{"schema_version", "operation_id", "owner_account_id", "api_origin", "org_id", "membership_id", "enrollment_id", "type", "operation_schema", "entity", "occurred_at", "entry_zone", "device_offset_seconds", "depends_on", "soft_references", "attachments", "payload"}

func decodeOperation(root *node, binding Binding, withDigest bool) (Envelope, error) {
	keys := append([]string(nil), operationKeys...)
	if withDigest {
		keys = append(keys, "payload_digest")
	}
	if !shape(root, keys...) {
		return Envelope{}, ErrInvalid
	}
	version, ok := integer(root.fields["schema_version"])
	if !ok || version != 1 {
		return Envelope{}, ErrInvalid
	}
	value := Envelope{SchemaVersion: version}
	stringsToRead := []struct {
		key    string
		target *string
	}{{"operation_id", &value.OperationID}, {"owner_account_id", &value.OwnerAccountID}, {"api_origin", &value.APIOrigin}, {"org_id", &value.OrgID}, {"membership_id", &value.MembershipID}, {"enrollment_id", &value.EnrollmentID}, {"type", &value.Type}, {"occurred_at", &value.OccurredAt}, {"entry_zone", &value.EntryZone}}
	for _, field := range stringsToRead {
		raw, ok := stringValue(root.fields[field.key])
		if !ok {
			return Envelope{}, ErrInvalid
		}
		*field.target = raw
	}
	for _, id := range []string{value.OperationID, value.OwnerAccountID, value.OrgID, value.MembershipID, value.EnrollmentID} {
		if !uuid(id) {
			return Envelope{}, ErrInvalid
		}
	}
	if ValidateOrigin(value.APIOrigin) != nil || !commandType(value.Type) || !canonicalInstant(value.OccurredAt, false) {
		return Envelope{}, ErrInvalid
	}
	if _, ok := kernel.LoadTimeZone(value.EntryZone); !ok {
		return Envelope{}, ErrInvalid
	}
	value.OperationSchema, ok = positive(root.fields["operation_schema"])
	if !ok {
		return Envelope{}, ErrInvalid
	}
	value.DeviceOffsetSeconds, ok = integer(root.fields["device_offset_seconds"])
	if !ok || value.DeviceOffsetSeconds < -64800 || value.DeviceOffsetSeconds > 64800 {
		return Envelope{}, ErrInvalid
	}
	entity := root.fields["entity"]
	if !shape(entity, "type", "id", "expected_revision") {
		return Envelope{}, ErrInvalid
	}
	value.Entity.Type, ok = stringValue(entity.fields["type"])
	if !ok || !snake(value.Entity.Type) {
		return Envelope{}, ErrInvalid
	}
	value.Entity.ID, ok = stringValue(entity.fields["id"])
	if !ok || !uuid(value.Entity.ID) {
		return Envelope{}, ErrInvalid
	}
	value.Entity.ExpectedRevision, ok = optionalRevision(entity.fields["expected_revision"])
	if !ok {
		return Envelope{}, ErrInvalid
	}
	if root.fields["payload"].kind != 'o' {
		return Envelope{}, ErrInvalid
	}
	hard := root.fields["depends_on"]
	if hard.kind != 'a' {
		return Envelope{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range hard.list {
		id, ok := stringValue(item)
		if !ok || !uuid(id) || id == value.OperationID || seen[id] {
			return Envelope{}, ErrInvalid
		}
		seen[id] = true
		value.DependsOn = append(value.DependsOn, id)
	}
	sort.Strings(value.DependsOn)
	sort.Slice(hard.list, func(i, j int) bool { return hard.list[i].value < hard.list[j].value })
	soft := root.fields["soft_references"]
	if soft.kind != 'a' {
		return Envelope{}, ErrInvalid
	}
	seen = map[string]bool{}
	for _, item := range soft.list {
		if !shape(item, "type", "id") {
			return Envelope{}, ErrInvalid
		}
		kind, good := stringValue(item.fields["type"])
		id, valid := stringValue(item.fields["id"])
		key := kind + ":" + id
		if !good || !valid || !snake(kind) || !uuid(id) || seen[key] {
			return Envelope{}, ErrInvalid
		}
		seen[key] = true
		value.SoftReferences = append(value.SoftReferences, SoftReference{Type: kind, ID: id})
	}
	sort.Slice(value.SoftReferences, func(i, j int) bool {
		a, b := value.SoftReferences[i], value.SoftReferences[j]
		return a.Type < b.Type || a.Type == b.Type && a.ID < b.ID
	})
	sort.Slice(soft.list, func(i, j int) bool {
		a, b := soft.list[i], soft.list[j]
		at, bt := a.fields["type"].value, b.fields["type"].value
		return at < bt || at == bt && a.fields["id"].value < b.fields["id"].value
	})
	files := root.fields["attachments"]
	if files.kind != 'a' {
		return Envelope{}, ErrInvalid
	}
	seen = map[string]bool{}
	for _, item := range files.list {
		if !shape(item, "id", "sha256", "bytes", "mime") {
			return Envelope{}, ErrInvalid
		}
		id, good := stringValue(item.fields["id"])
		hash, valid := stringValue(item.fields["sha256"])
		mime, hasMime := stringValue(item.fields["mime"])
		size, hasBytes := positive(item.fields["bytes"])
		if !good || !valid || !hasMime || !hasBytes || !uuid(id) || !bareDigest(hash) || !mimePattern.MatchString(mime) || seen[id] {
			return Envelope{}, ErrInvalid
		}
		seen[id] = true
		value.Attachments = append(value.Attachments, Attachment{ID: id, SHA256: hash, Bytes: size, MIME: mime})
	}
	sort.Slice(value.Attachments, func(i, j int) bool { return value.Attachments[i].ID < value.Attachments[j].ID })
	sort.Slice(files.list, func(i, j int) bool { return files.list[i].fields["id"].value < files.list[j].fields["id"].value })
	if withDigest {
		value.PayloadDigest, ok = stringValue(root.fields["payload_digest"])
		if !ok || !digestText(value.PayloadDigest) {
			return Envelope{}, ErrInvalid
		}
	}
	if value.OwnerAccountID != binding.owner || value.APIOrigin != binding.origin || value.OrgID != binding.organization {
		return Envelope{}, ErrBinding
	}
	return value, nil
}
func operation(raw []byte, binding Binding, withDigest bool, options []Limits) (Operation, error) {
	if !binding.valid() {
		return Operation{}, ErrBinding
	}
	limits, err := selectedLimits(options)
	if err != nil {
		return Operation{}, err
	}
	if len(raw) > limits.operationBytes {
		return Operation{}, ErrLimit
	}
	root, err := parseJSON(raw, limits.depth-2)
	if err != nil {
		return Operation{}, err
	}
	value, err := decodeOperation(root, binding, withDigest)
	if err != nil {
		return Operation{}, err
	}
	delete(root.fields, "payload_digest")
	intent := canonical(root)
	hash := sha256.Sum256(intent)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	if withDigest && value.PayloadDigest != digest {
		return Operation{}, ErrDigest
	}
	value.PayloadDigest = digest
	root.fields["payload_digest"] = &node{kind: 's', value: digest}
	complete := canonical(root)
	if len(complete) > limits.operationBytes {
		return Operation{}, ErrLimit
	}
	return Operation{value: value, canonical: complete, payload: canonical(root.fields["payload"])}, nil
}
func Seal(rawWithoutDigest []byte, binding Binding, options ...Limits) (Operation, error) {
	return operation(rawWithoutDigest, binding, false, options)
}
func ParseOperation(raw []byte, binding Binding, options ...Limits) (Operation, error) {
	return operation(raw, binding, true, options)
}
func SameIntent(first, second Operation) (bool, error) {
	if len(first.canonical) == 0 || len(second.canonical) == 0 {
		return false, ErrInvalid
	}
	if first.ID() != second.ID() {
		return false, nil
	}
	if first.PayloadDigest() != second.PayloadDigest() {
		return false, ErrReused
	}
	return true, nil
}
