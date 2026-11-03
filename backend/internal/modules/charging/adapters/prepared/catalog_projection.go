package prepared

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
)

// NationalPlace is the source-backed projection of one verified catalog row.
type NationalPlace struct {
	Identity       CatalogIdentity
	Name           *string
	Brand          *string
	Operator       *string
	Network        *string
	Address        map[string]string
	PointSourceKey string
	Latitude       string
	Longitude      string
	Access         string
	Confidence     string
	Sources        []NationalSource
	Groups         []NationalGroup
	Combined       *CatalogOSMObject
}

// NationalSource retains the lineage behind a root or attached equipment.
type NationalSource struct {
	domain.SourceAttribution
	Key           string
	Kind          string
	Access        string
	Status        string
	RecordSHA256  string             `json:"record_sha256"`
	ObjectVersion *int64             `json:"object_version"`
	Clocks        map[string]*string `json:"clocks"`
}

type CatalogOSMObject struct {
	Type string
	ID   int64
}

// NationalGroup keeps the owner of each source assertion; quantities from
// different sources are never combined.
type NationalGroup struct {
	SourceKey string
	Key       string `json:"key"`
	Family    string `json:"family"`
	RawSocket string `json:"raw_socket"`
	Quantity  *int64 `json:"reported_quantity"`
	PowerW    *int64 `json:"power_w"`
	Current   string `json:"current"`
	RawStatus *int64 `json:"raw_status"`
}

type nationalPoint struct {
	Latitude  string `json:"latitude_decimal"`
	Longitude string `json:"longitude_decimal"`
}

type nationalSourceRow struct {
	Key    string          `json:"key"`
	Kind   string          `json:"kind"`
	Point  nationalPoint   `json:"point"`
	Access string          `json:"access"`
	Status string          `json:"status"`
	Source NationalSource  `json:"source"`
	Groups []NationalGroup `json:"groups"`
}

type nationalSiteRow struct {
	Key          string            `json:"key"`
	Kind         string            `json:"kind"`
	Point        nationalPoint     `json:"point"`
	Name         *string           `json:"name"`
	Operator     *string           `json:"operator"`
	Network      *string           `json:"network"`
	Address      map[string]string `json:"address"`
	Brand        *string           `json:"brand"`
	Confidence   string            `json:"confidence"`
	FieldSources map[string]struct {
		SourceKey string `json:"source_key"`
		Field     string `json:"field"`
	} `json:"field_sources"`
	Sources   []nationalSourceRow `json:"sources"`
	Equipment []nationalSourceRow `json:"equipment"`
}

var errNationalProjection = errors.New("prepared: national catalog projection refused")

// ProjectNationalCatalog maps the verified bytes to storage inputs. A single
// malformed row refuses the entire result.
func ProjectNationalCatalog(bundle NationalBundle) ([]NationalPlace, error) {
	if bundle.Sites < 1 || bundle.Sites > 205000 || len(bundle.Catalog) > offerLimit ||
		fmt.Sprintf("%x", sha256.Sum256(bundle.Catalog)) != bundle.OfferSHA256 {
		return nil, errNationalProjection
	}
	out := make([]NationalPlace, 0, bundle.Sites)
	seenSources := map[string]bool{}
	seenGroups := map[string]bool{}
	identities := make([]CatalogIdentity, 0, bundle.Sites)
	lines := bufio.NewScanner(bytes.NewReader(bundle.Catalog))
	lines.Buffer(make([]byte, 4096), 1<<20)
	for lines.Scan() {
		var row nationalSiteRow
		if int64(len(out)) >= bundle.Sites || json.Unmarshal(lines.Bytes(), &row) != nil {
			return nil, errNationalProjection
		}
		place, err := projectNationalPlace(row, seenSources, seenGroups)
		if err != nil {
			return nil, err
		}
		out = append(out, place)
		identities = append(identities, place.Identity)
	}
	if lines.Err() != nil || int64(len(out)) != bundle.Sites {
		return nil, errNationalProjection
	}
	if _, err := ResolveCatalogIdentities(identities, nil); err != nil {
		return nil, err
	}
	return out, nil
}

func projectNationalPlace(row nationalSiteRow, seenSources, seenGroups map[string]bool) (NationalPlace, error) {
	if (row.Kind != "CHARGING_SITE" && row.Kind != "COMBINED_SITE") ||
		(len(row.Sources) != 1 && len(row.Sources) != 2) ||
		(row.Confidence != "single-source" && row.Confidence != "two-source") ||
		(row.Confidence == "two-source") != (len(row.Sources) == 2) ||
		!validNationalPoint(row.Point) || !validNationalAddress(row.Address) {
		return NationalPlace{}, errNationalProjection
	}
	for _, label := range []*string{row.Name, row.Brand, row.Operator, row.Network} {
		if label != nil && !nationalText(*label, 512) {
			return NationalPlace{}, errNationalProjection
		}
	}
	place := NationalPlace{
		Identity: CatalogIdentity{Key: row.Key}, Name: row.Name, Brand: row.Brand,
		Operator: row.Operator, Network: row.Network, Address: row.Address,
		Latitude: row.Point.Latitude, Longitude: row.Point.Longitude, Confidence: row.Confidence,
	}
	pointOwner := row.FieldSources["point"]
	for _, source := range row.Sources {
		if source.Kind != "CHARGING_SITE" && source.Kind != "COMBINED_SITE" {
			return NationalPlace{}, errNationalProjection
		}
		if source.Key == pointOwner.SourceKey && pointOwner.Field == "point" && source.Point == row.Point {
			place.Access, place.PointSourceKey = source.Access, source.Key
		}
		if source.Kind == "COMBINED_SITE" && source.Source.Source == "OSM" {
			kind, id, _ := strings.Cut(source.Source.ExternalID, ":")
			number, _ := strconv.ParseInt(id, 10, 64)
			place.Combined = &CatalogOSMObject{Type: kind, ID: number}
		}
		place.Identity.SourceKeys = append(place.Identity.SourceKeys, source.Key)
	}
	if place.Access == "" || (row.Kind == "COMBINED_SITE") != (place.Combined != nil) {
		return NationalPlace{}, errNationalProjection
	}
	for _, equipment := range row.Equipment {
		if equipment.Kind != "CHARGE_POINT" {
			return NationalPlace{}, errNationalProjection
		}
	}
	for _, records := range [][]nationalSourceRow{row.Sources, row.Equipment} {
		for _, source := range records {
			if seenSources[source.Key] || !validNationalSource(source) {
				return NationalPlace{}, errNationalProjection
			}
			seenSources[source.Key] = true
			lineage := source.Source
			lineage.Key, lineage.Kind, lineage.Access, lineage.Status = source.Key, source.Kind, source.Access, source.Status
			place.Sources = append(place.Sources, lineage)
			for _, group := range source.Groups {
				if seenGroups[group.Key] || !validNationalGroup(group, source) {
					return NationalPlace{}, errNationalProjection
				}
				seenGroups[group.Key] = true
				group.SourceKey = source.Key
				place.Groups = append(place.Groups, group)
			}
		}
	}
	return place, nil
}

func validNationalSource(row nationalSourceRow) bool {
	namespace, ok := catalogSourceNamespace(row.Key)
	source := row.Source
	return ok && strings.ToUpper(namespace) == source.Source && row.Key == namespace+":"+source.ExternalID &&
		(source.Source == "OCM" && source.ProviderID != nil && *source.ProviderID > 0 ||
			source.Source == "OSM" && source.ProviderID == nil) &&
		nationalText(source.Revision, 128) && nationalText(source.License, 512) &&
		nationalText(source.Attribution, 512) && validHash(source.RecordSHA256) && validNationalPoint(row.Point) &&
		validNationalAccess(row.Access)
}

func validNationalAddress(address map[string]string) bool {
	for key, value := range address {
		switch key {
		case "street", "number", "complement", "district", "municipio", "postal_code", "uf":
			if !nationalText(value, 256) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validNationalAccess(access string) bool {
	switch access {
	case "PUBLIC", "UNKNOWN", "PERMISSIVE", "CUSTOMERS", "DESTINATION", "MEMBERSHIP", "NOTICE_REQUIRED":
		return true
	default:
		return false
	}
}

func validNationalGroup(group NationalGroup, owner nationalSourceRow) bool {
	prefix := owner.Key + ":socket:"
	if owner.Source.Source == "OCM" {
		prefix = "ocm:group:" + owner.Source.ExternalID + ":"
	}
	suffix, ok := strings.CutPrefix(group.Key, prefix)
	if !ok || !nationalText(suffix, 128) || strings.Contains(suffix, ":") || len(group.RawSocket) > 256 ||
		strings.ContainsFunc(group.RawSocket, unicode.IsControl) ||
		(group.Current != "AC" && group.Current != "DC" && group.Current != "UNKNOWN") ||
		(group.PowerW != nil && (*group.PowerW < 1 || *group.PowerW > 1000000000)) ||
		(group.Quantity != nil && (*group.Quantity < 1 || *group.Quantity > 500000)) {
		return false
	}
	switch group.Family {
	case "TYPE1", "TYPE2", "CCS1", "CCS2", "CHADEMO", "GBT_AC", "GBT_DC", "NACS", "NBR14136", "TESLA_LEGACY", "UNKNOWN":
		return true
	default:
		return false
	}
}

func validNationalPoint(point nationalPoint) bool {
	return nationalCoordinate(point.Latitude, 90) && nationalCoordinate(point.Longitude, 180)
}

func nationalCoordinate(raw string, maximum float64) bool {
	value, err := strconv.ParseFloat(raw, 64)
	return err == nil && len(raw) <= 64 && !math.IsNaN(value) && math.Abs(value) <= maximum
}

func nationalText(value string, maximum int) bool {
	return len(value) <= maximum && strings.TrimSpace(value) != "" && !strings.ContainsFunc(value, unicode.IsControl)
}
