package http

import (
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"strconv"
)

type meterInstallRequest struct {
	privateRequest
	assetRevisionCommand
	OpeningReadingID    string       `json:"opening_reading_id"`
	Kind                meter.Kind   `json:"kind"`
	Label               string       `json:"label"`
	Modulus             string       `json:"modulus,omitempty"`
	OpeningValue        string       `json:"opening_value"`
	OccurredAt          string       `json:"occurred_at"`
	EntryZone           string       `json:"entry_zone"`
	DeviceOffsetSeconds *int64       `json:"device_offset_seconds"`
	Method              meter.Method `json:"method"`
}
type meterListRequest struct {
	companyRequest
	orgapi.Page
}
type meterReadRequest struct{ companyRequest }

func (v meterInstallRequest) command(assetID string) (app.MeterInstall, error) {
	if v.Revision == nil || v.DeviceOffsetSeconds == nil {
		return app.MeterInstall{}, orgapi.Malformed
	}
	var modulus *int64
	if v.Modulus != "" {
		n, e := strconv.ParseInt(v.Modulus, 10, 64)
		if e != nil || n <= 0 || strconv.FormatInt(n, 10) != v.Modulus {
			return app.MeterInstall{}, meter.ErrInvalid
		}
		modulus = &n
	}
	value, e := meter.ParseBase(v.Kind, v.OpeningValue, modulus)
	if e != nil {
		return app.MeterInstall{}, e
	}
	at, e := orgtime.ParseInstant(v.OccurredAt)
	if e != nil {
		return app.MeterInstall{}, meter.ErrInvalid
	}
	return app.NewMeterInstall(app.MeterInstallInput{OperationID: v.OperationID, AssetID: assetID, OpeningReadingID: v.OpeningReadingID, ExpectedAssetRevision: *v.Revision, Kind: v.Kind, Label: v.Label, Modulus: modulus, Value: value, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: *v.DeviceOffsetSeconds}, Method: v.Method})
}
func (v *meterInstallRequest) UnmarshalJSON(raw []byte) error {
	type plain meterInstallRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterInstallRequest(input)
	return nil
}
func (v *meterReadRequest) UnmarshalJSON(raw []byte) error {
	type plain meterReadRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterReadRequest(input)
	return nil
}
func (v *meterListRequest) UnmarshalJSON(raw []byte) error {
	type plain meterListRequest
	var input plain
	if e := decodeAsset(raw, &input); e != nil {
		return e
	}
	*v = meterListRequest(input)
	return nil
}
