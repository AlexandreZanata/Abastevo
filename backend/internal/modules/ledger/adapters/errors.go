package adapters

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errInvalidValue   = errors.New("ledger: invalid input")
	errConflictValue  = errors.New("ledger: revision conflict")
	errUnavailableVal = errors.New("ledger: service unavailable")
)

func errInvalid() error     { return errInvalidValue }
func errConflict() error    { return errConflictValue }
func errUnavailable() error { return errUnavailableVal }

func errInvalidSentinel() error { return errInvalidValue }

func validLedgerID(s string) bool {
	if s == "00000000-0000-0000-0000-000000000000" || len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' || strings.ToLower(s) != s {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil
}

func kernelProduct(s string) kernel.Product { return kernel.Product(s) }

func kernelUnit(s string) kernel.Unit { return kernel.Unit(s) }

func pgTimestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
