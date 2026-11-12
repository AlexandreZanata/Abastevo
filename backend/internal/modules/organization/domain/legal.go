package domain

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var ErrInvalidCPF = errors.New("organization: invalid CPF")
var ErrInvalidCNPJ = errors.New("organization: invalid CNPJ")

type privateLegal struct{}

func (privateLegal) Format(f fmt.State, _ rune)   { redact(f, "LegalDeclaration(redacted)") }
func (privateLegal) MarshalJSON() ([]byte, error) { return json.Marshal("LegalDeclaration(redacted)") }
func (privateLegal) LogValue() slog.Value         { return slog.StringValue("LegalDeclaration(redacted)") }

// CPF is a format-checked identifier, not registry existence or legal evidence.
// Primary formula and constant-sequence refusal are recorded in 18-references.
type CPF struct {
	privateLegal
	value string
}

func ParseCPF(raw string) (CPF, error) {
	var b strings.Builder
	for _, ch := range raw {
		switch {
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
		case ch == '.' || ch == '-' || ch == ' ':
		default:
			return CPF{}, ErrInvalidCPF
		}
	}
	value := b.String()
	if len(value) != 11 || strings.Count(value, value[:1]) == 11 {
		return CPF{}, ErrInvalidCPF
	}
	for end := 9; end <= 10; end++ {
		sum := 0
		for i := range end {
			sum += int(value[i]-'0') * (end + 1 - i)
		}
		digit := 0
		if rest := sum % 11; rest >= 2 {
			digit = 11 - rest
		}
		if int(value[end]-'0') != digit {
			return CPF{}, ErrInvalidCPF
		}
	}
	return CPF{value: value}, nil
}
func (c CPF) ForStorage() string { return c.value }
func (c CPF) Masked() string {
	if len(c.value) != 11 {
		return ""
	}
	return "***.***.***-" + c.value[9:]
}

type LegalKind string

const (
	Company       LegalKind = "COMPANY"
	NaturalPerson LegalKind = "NATURAL_PERSON"
)

type LegalState string

const (
	LegalDeclared LegalState = "DECLARED"
	LegalRevoked  LegalState = "REVOKED"
)

type LegalPurpose string

const VerificationAndAgreements LegalPurpose = "VERIFICATION_AND_AGREEMENTS"

type EstablishmentRole string

const (
	HeadOffice  EstablishmentRole = "HEAD_OFFICE"
	Branch      EstablishmentRole = "BRANCH"
	NotInformed EstablishmentRole = "NOT_INFORMED"
)

type EstablishmentInput struct {
	privateLegal
	CNPJ                        string
	Role                        EstablishmentRole
	TradeName, MunicipalityCode string
}
type LegalDeclarationInput struct {
	privateLegal
	Kind                  LegalKind
	LegalName, Identifier string
	Purpose               LegalPurpose
	Establishment         EstablishmentInput
}
type LegalDeclaration struct {
	privateLegal
	operation string
	value     LegalDeclarationInput
}

func canonicalLegalID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}
func legalName(raw string, required bool) (string, error) {
	value := strings.TrimSpace(raw)
	if !validName(value) || (required && blankName(value)) {
		return "", ErrInvalid
	}
	return value, nil
}
func canonicalEstablishment(v EstablishmentInput) (EstablishmentInput, error) {
	parsed, err := kernel.ParseCNPJ(v.CNPJ)
	if err != nil {
		return EstablishmentInput{}, ErrInvalidCNPJ
	}
	v.CNPJ = parsed.Normalized()
	if v.Role == "" {
		v.Role = NotInformed
	}
	switch v.Role {
	case HeadOffice, Branch, NotInformed:
	default:
		return EstablishmentInput{}, ErrInvalid
	}
	v.TradeName, err = legalName(v.TradeName, false)
	if err != nil {
		return EstablishmentInput{}, err
	}
	if v.MunicipalityCode != "" {
		if len(v.MunicipalityCode) != 7 || v.MunicipalityCode[0] < '1' || v.MunicipalityCode[0] > '9' {
			return EstablishmentInput{}, ErrInvalid
		}
		for _, ch := range v.MunicipalityCode {
			if ch < '0' || ch > '9' {
				return EstablishmentInput{}, ErrInvalid
			}
		}
	}
	return v, nil
}
func NewLegalDeclaration(operation string, v LegalDeclarationInput) (LegalDeclaration, error) {
	if !canonicalLegalID(operation) {
		return LegalDeclaration{}, ErrInvalid
	}
	name, err := legalName(v.LegalName, true)
	if err != nil {
		return LegalDeclaration{}, err
	}
	v.LegalName = name
	switch v.Kind {
	case Company:
		if v.Purpose != "" {
			return LegalDeclaration{}, ErrInvalid
		}
		identifier, err := kernel.ParseCNPJ(v.Identifier)
		if err != nil {
			return LegalDeclaration{}, ErrInvalidCNPJ
		}
		v.Identifier = identifier.Normalized()
		if v.Establishment.CNPJ == "" {
			v.Establishment.CNPJ = v.Identifier
		}
		v.Establishment, err = canonicalEstablishment(v.Establishment)
		if err != nil {
			return LegalDeclaration{}, err
		}
		if v.Establishment.CNPJ != v.Identifier {
			return LegalDeclaration{}, ErrInvalidCNPJ
		}
	case NaturalPerson:
		if v.Purpose != VerificationAndAgreements || v.Establishment != (EstablishmentInput{}) {
			return LegalDeclaration{}, ErrInvalid
		}
		cpf, err := ParseCPF(v.Identifier)
		if err != nil {
			return LegalDeclaration{}, err
		}
		v.Identifier = cpf.ForStorage()
	default:
		return LegalDeclaration{}, ErrInvalid
	}
	return LegalDeclaration{operation: operation, value: v}, nil
}
func (c LegalDeclaration) Validate() error {
	_, err := NewLegalDeclaration(c.operation, c.value)
	return err
}
func (c LegalDeclaration) OperationID() string                    { return c.operation }
func (c LegalDeclaration) InputForStorage() LegalDeclarationInput { return c.value }
func (c LegalDeclaration) Digest(scope kernel.OrgScope) string {
	if scope.Validate() != nil || c.Validate() != nil {
		return ""
	}
	v := c.value
	e := v.Establishment
	// Bind to scope and random operation: never create a cross-organization CPF
	// lookup digest. Only the bounded hash, not this content, enters audit.
	raw, _ := json.Marshal([]any{1, "org.legal.declare", scope.OrgID().String(), c.operation, v.Kind, v.LegalName, v.Identifier, v.Purpose, e.CNPJ, e.Role, e.TradeName, e.MunicipalityCode})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type LegalEntitySnapshot struct {
	privateLegal
	OrgID                 kernel.OrgID
	ID                    string
	Kind                  LegalKind
	LegalName, Identifier string
	Purpose               LegalPurpose
	State                 LegalState
	Revision              int64
	VerificationHistory   bool
}
type LegalEntity struct {
	privateLegal
	value LegalEntitySnapshot
}

func RehydrateLegalEntity(scope kernel.OrgScope, v LegalEntitySnapshot) (LegalEntity, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return LegalEntity{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || v.Revision < 1 || (v.State != LegalDeclared && v.State != LegalRevoked) {
		return LegalEntity{}, ErrInvalid
	}
	name, err := legalName(v.LegalName, true)
	if err != nil || name != v.LegalName {
		return LegalEntity{}, ErrInvalid
	}
	switch v.Kind {
	case Company:
		if len(v.Identifier) != 8 || v.Purpose != "" {
			return LegalEntity{}, ErrInvalid
		}
		for _, ch := range v.Identifier {
			if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z') {
				return LegalEntity{}, ErrInvalidCNPJ
			}
		}
	case NaturalPerson:
		cpf, err := ParseCPF(v.Identifier)
		if err != nil || cpf.ForStorage() != v.Identifier || v.Purpose != VerificationAndAgreements {
			return LegalEntity{}, ErrInvalidCPF
		}
	default:
		return LegalEntity{}, ErrInvalid
	}
	return LegalEntity{value: v}, nil
}
func (e LegalEntity) Snapshot(scope kernel.OrgScope) (LegalEntitySnapshot, error) {
	if _, err := RehydrateLegalEntity(scope, e.value); err != nil {
		return LegalEntitySnapshot{}, err
	}
	return e.value, nil
}
func (e LegalEntity) MaskedIdentifier(scope kernel.OrgScope) string {
	v, err := e.Snapshot(scope)
	if err != nil {
		return ""
	}
	if v.Kind == Company {
		return v.Identifier
	}
	cpf, err := ParseCPF(v.Identifier)
	if err != nil {
		return ""
	}
	return cpf.Masked()
}
func (c LegalDeclaration) Create(scope kernel.OrgScope, entityID, establishmentID string) (LegalEntity, Establishment, error) {
	if err := c.Validate(); err != nil {
		return LegalEntity{}, Establishment{}, err
	}
	identifier := c.value.Identifier
	if c.value.Kind == Company {
		identifier = identifier[:8]
	}
	entity, err := RehydrateLegalEntity(scope, LegalEntitySnapshot{OrgID: scope.OrgID(), ID: entityID, Kind: c.value.Kind, LegalName: c.value.LegalName, Identifier: identifier, Purpose: c.value.Purpose, State: LegalDeclared, Revision: 1})
	if err != nil {
		return LegalEntity{}, Establishment{}, err
	}
	if c.value.Kind == NaturalPerson {
		if establishmentID != "" {
			return LegalEntity{}, Establishment{}, ErrInvalid
		}
		return entity, Establishment{}, nil
	}
	if entityID == establishmentID {
		return LegalEntity{}, Establishment{}, ErrInvalid
	}
	establishment, err := NewEstablishment(scope, establishmentID, entity, c.value.Establishment)
	if err != nil {
		return LegalEntity{}, Establishment{}, err
	}
	return entity, establishment, nil
}

type EstablishmentSnapshot struct {
	privateLegal
	OrgID                       kernel.OrgID
	ID, EntityID, CNPJ          string
	Role                        EstablishmentRole
	TradeName, MunicipalityCode string
	Revision                    int64
}
type Establishment struct {
	privateLegal
	value EstablishmentSnapshot
}

func NewEstablishment(scope kernel.OrgScope, id string, entity LegalEntity, input EstablishmentInput) (Establishment, error) {
	parent, err := entity.Snapshot(scope)
	if err != nil {
		return Establishment{}, err
	}
	if parent.Kind != Company || parent.State != LegalDeclared {
		return Establishment{}, ErrTransition
	}
	v, err := canonicalEstablishment(input)
	if err != nil {
		return Establishment{}, err
	}
	if v.CNPJ[:8] != parent.Identifier {
		return Establishment{}, ErrInvalidCNPJ
	}
	return RehydrateEstablishment(scope, EstablishmentSnapshot{OrgID: scope.OrgID(), ID: id, EntityID: parent.ID, CNPJ: v.CNPJ, Role: v.Role, TradeName: v.TradeName, MunicipalityCode: v.MunicipalityCode, Revision: 1})
}
func RehydrateEstablishment(scope kernel.OrgScope, v EstablishmentSnapshot) (Establishment, error) {
	if scope.Validate() != nil || v.OrgID != scope.OrgID() {
		return Establishment{}, ErrScope
	}
	if !canonicalLegalID(v.ID) || !canonicalLegalID(v.EntityID) || v.ID == v.EntityID || v.Revision < 1 {
		return Establishment{}, ErrInvalid
	}
	input, err := canonicalEstablishment(EstablishmentInput{CNPJ: v.CNPJ, Role: v.Role, TradeName: v.TradeName, MunicipalityCode: v.MunicipalityCode})
	if err != nil {
		return Establishment{}, err
	}
	if input.CNPJ != v.CNPJ || input.Role != v.Role || input.TradeName != v.TradeName {
		return Establishment{}, ErrInvalid
	}
	return Establishment{value: v}, nil
}
func (e Establishment) Snapshot(scope kernel.OrgScope) (EstablishmentSnapshot, error) {
	if _, err := RehydrateEstablishment(scope, e.value); err != nil {
		return EstablishmentSnapshot{}, err
	}
	return e.value, nil
}

type LegalReferences struct {
	Current             int64
	VerificationHistory bool
}
type LegalWithdrawal struct {
	privateLegal
	Remove bool
	Entity LegalEntity
}

func (e LegalEntity) Withdraw(scope kernel.OrgScope, expected int64, facts LegalReferences) (LegalWithdrawal, error) {
	v, err := e.Snapshot(scope)
	if err != nil {
		return LegalWithdrawal{}, err
	}
	if expected != v.Revision {
		return LegalWithdrawal{}, ErrConflict
	}
	if v.Revision == math.MaxInt64 {
		return LegalWithdrawal{}, ErrRevision
	}
	if facts.Current < 0 {
		return LegalWithdrawal{}, ErrInvalid
	}
	if v.State != LegalDeclared {
		return LegalWithdrawal{}, ErrTransition
	}
	if facts.Current == 0 && !facts.VerificationHistory && !v.VerificationHistory {
		return LegalWithdrawal{Remove: true}, nil
	}
	v.State = LegalRevoked
	v.Revision++
	entity, err := RehydrateLegalEntity(scope, v)
	if err != nil {
		return LegalWithdrawal{}, err
	}
	return LegalWithdrawal{Entity: entity}, nil
}

// CanonicalEstablishmentInput validates a declaration before a parent is read.
// NewEstablishment still checks that actual parent's scope, state and root.
func CanonicalEstablishmentInput(input EstablishmentInput) (EstablishmentInput, error) {
	return canonicalEstablishment(input)
}

type EstablishmentMetadata struct {
	privateLegal
	Role                        EstablishmentRole
	TradeName, MunicipalityCode string
}

func CanonicalEstablishmentMetadata(v EstablishmentMetadata) (EstablishmentMetadata, error) {
	if v.Role == "" {
		v.Role = NotInformed
	}
	switch v.Role {
	case HeadOffice, Branch, NotInformed:
	default:
		return EstablishmentMetadata{}, ErrInvalid
	}
	name, err := legalName(v.TradeName, false)
	if err != nil {
		return EstablishmentMetadata{}, err
	}
	v.TradeName = name
	if v.MunicipalityCode != "" {
		if len(v.MunicipalityCode) != 7 || v.MunicipalityCode[0] < '1' || v.MunicipalityCode[0] > '9' {
			return EstablishmentMetadata{}, ErrInvalid
		}
		for _, ch := range v.MunicipalityCode {
			if ch < '0' || ch > '9' {
				return EstablishmentMetadata{}, ErrInvalid
			}
		}
	}
	return v, nil
}
func (e Establishment) ChangeMetadata(scope kernel.OrgScope, expected int64, metadata EstablishmentMetadata) (Establishment, error) {
	v, err := e.Snapshot(scope)
	if err != nil {
		return Establishment{}, err
	}
	if expected != v.Revision {
		return Establishment{}, ErrConflict
	}
	if v.Revision == math.MaxInt64 {
		return Establishment{}, ErrRevision
	}
	m, err := CanonicalEstablishmentMetadata(metadata)
	if err != nil {
		return Establishment{}, err
	}
	v.Role = m.Role
	v.TradeName = m.TradeName
	v.MunicipalityCode = m.MunicipalityCode
	v.Revision++
	return RehydrateEstablishment(scope, v)
}
