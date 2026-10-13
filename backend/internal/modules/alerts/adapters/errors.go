package adapters

import (
	"encoding/hex"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	errInvalidValue   = errors.New("alerts: invalid input")
	errConflictValue  = errors.New("alerts: revision conflict")
	errQuotaValue     = errors.New("alerts: quota exceeded")
	errUnavailableVal = errors.New("alerts: service unavailable")
)

func errInvalid() error     { return errInvalidValue }
func errConflict() error    { return errConflictValue }
func errQuota() error       { return errQuotaValue }
func errUnavailable() error { return errUnavailableVal }

func kernelProduct(s string) kernel.Product { return kernel.Product(s) }

func kernelUnit(s string) kernel.Unit { return kernel.Unit(s) }

func kernelCondition(s string) kernel.ConditionKind { return kernel.ConditionKind(s) }

func idString(id pgtype.UUID) string {
	s := hex.EncodeToString(id.Bytes[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}
