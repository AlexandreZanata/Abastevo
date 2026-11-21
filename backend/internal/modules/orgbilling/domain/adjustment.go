package domain

import (
	"errors"

	payments "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

type AdjustmentKind string

const (
	ServiceCredit   AdjustmentKind = "SERVICE_CREDIT"
	DebitCorrection AdjustmentKind = "DEBIT_CORRECTION"
)

var ErrInvalidAdjustment = errors.New("organization billing: invalid adjustment")

func AdjustmentDelta(kind AdjustmentKind, amount payments.Money) (int64, error) {
	if amount.Positive() != nil {
		return 0, ErrInvalidAdjustment
	}
	switch kind {
	case ServiceCredit:
		return -amount.Centavos, nil
	case DebitCorrection:
		return amount.Centavos, nil
	default:
		return 0, ErrInvalidAdjustment
	}
}
