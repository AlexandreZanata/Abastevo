// Package mapcontract validates the frozen map assertion and native map
// manifest wire formats before any producer or consumer publishes them.
// It is intentionally independent of storage and of the native engine.
package mapcontract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	AssertionFormat = "abastevo.map-assertions-v1"
	ManifestFormat  = "abastevo.native-map-manifest-v1"
	Version         = 1
)

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hex40       = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hex64       = regexp.MustCompile(`^[0-9a-f]{64}$`)
	cnpjPattern = regexp.MustCompile(`^[0-9A-Za-z]{14}$`)
	regionID    = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	packagePath = regexp.MustCompile(`^[A-Za-z0-9._/-]+( [A-Za-z0-9._/-]+)*$`)
)

type AssertionCorpus struct {
	Format     string      `json:"format"`
	Version    int         `json:"version"`
	Generation string      `json:"generation"`
	Assertions []Assertion `json:"assertions"`
}

type Assertion struct {
	ID           string       `json:"id"`
	Source       string       `json:"source"`
	Kind         string       `json:"kind"`
	Name         string       `json:"name"`
	Identities   Identities   `json:"identities"`
	Location     Location     `json:"location"`
	Services     []string     `json:"services"`
	Prices       []Price      `json:"prices"`
	Charging     *Charging    `json:"charging"`
	Clocks       Clocks       `json:"clocks"`
	Rights       Rights       `json:"rights"`
	Capabilities Capabilities `json:"capabilities"`
}

type Identities struct {
	StationUUID *string `json:"station_uuid"`
	CNPJ        *string `json:"cnpj"`
	PlaceID     *string `json:"place_id"`
	EquipmentID *string `json:"equipment_id"`
	ConnectorID *string `json:"connector_id"`
	OSMType     *string `json:"osm_type"`
	OSMID       *int64  `json:"osm_id"`
}

type Location struct {
	LatitudeDeg          *string `json:"latitude_deg"`
	LongitudeDeg         *string `json:"longitude_deg"`
	Quality              string  `json:"quality"`
	DisplayPoint         *string `json:"display_point"`
	EntranceLatitudeDeg  *string `json:"entrance_latitude_deg"`
	EntranceLongitudeDeg *string `json:"entrance_longitude_deg"`
}

type Price struct {
	Source         string  `json:"source"`
	StationUUID    string  `json:"station_uuid"`
	Product        string  `json:"product"`
	Unit           string  `json:"unit"`
	AmountMilliBRL int64   `json:"amount_milli_brl"`
	ObservedAt     string  `json:"observed_at"`
	PeriodStart    *string `json:"period_start"`
	PeriodEnd      *string `json:"period_end"`
}

type Charging struct {
	PlaceID              string  `json:"place_id"`
	EquipmentID          string  `json:"equipment_id"`
	ConnectorID          string  `json:"connector_id"`
	ConnectorCode        string  `json:"connector_code"`
	PowerW               *int64  `json:"power_w"`
	Access               string  `json:"access"`
	Status               string  `json:"status"`
	TariffMilliBRLPerKWh *int64  `json:"tariff_milli_brl_per_kwh"`
	TariffCondition      *string `json:"tariff_condition"`
	ObservedAt           string  `json:"observed_at"`
}

type Clocks struct {
	ObservedAt  string `json:"observed_at"`
	PublishedAt string `json:"published_at"`
	FetchedAt   string `json:"fetched_at"`
}

type Rights struct {
	Source       string `json:"source"`
	License      string `json:"license"`
	Attribution  string `json:"attribution"`
	SourceSHA256 string `json:"source_sha256"`
}

type Capabilities struct {
	PriceKnown    bool `json:"price_known"`
	PinEligible   bool `json:"pin_eligible"`
	EntranceKnown bool `json:"entrance_known"`
	LiveStatus    bool `json:"live_status"`
}

type NativeManifest struct {
	Format       string         `json:"format"`
	Version      int            `json:"version"`
	Generation   string         `json:"generation"`
	Engine       Engine         `json:"engine"`
	Region       Region         `json:"region"`
	Packages     []Package      `json:"packages"`
	Dependencies []string       `json:"dependencies"`
	Sources      []Source       `json:"sources"`
	Rights       ManifestRights `json:"rights"`
	GeneratedAt  string         `json:"generated_at"`
}

type Engine struct {
	Name          string `json:"name"`
	UpstreamSHA   string `json:"upstream_sha"`
	GeneratorSHA  string `json:"generator_sha"`
	FormatVersion string `json:"format_version"`
}

type Region struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BBox     BBox   `json:"bbox"`
	Overview string `json:"overview"`
}

type BBox struct {
	MinLonDeg float64 `json:"min_lon_deg"`
	MinLatDeg float64 `json:"min_lat_deg"`
	MaxLonDeg float64 `json:"max_lon_deg"`
	MaxLatDeg float64 `json:"max_lat_deg"`
}

type Package struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Source struct {
	ID          string `json:"id"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
	SHA256      string `json:"sha256"`
}

type ManifestRights struct {
	License        string `json:"license"`
	Attribution    string `json:"attribution"`
	Commercial     bool   `json:"commercial"`
	Offline        bool   `json:"offline"`
	Redistribution bool   `json:"redistribution"`
}

type Error struct {
	Path   string
	Reason string
}

func (e *Error) Error() string { return e.Path + ": " + e.Reason }

func fail(path, reason string) error { return &Error{Path: path, Reason: reason} }

func decode(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fail("$", "invalid json: "+err.Error())
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fail("$", "trailing bytes")
	}
	return nil
}

// ParseAssertions validates one complete corpus. No partial result is
// returned: any invalid assertion rejects the whole generation.
func ParseAssertions(data []byte) (*AssertionCorpus, error) {
	var corpus AssertionCorpus
	if err := decode(data, &corpus); err != nil {
		return nil, err
	}
	if corpus.Format != AssertionFormat {
		return nil, fail("$.format", "unsupported format")
	}
	if corpus.Version != Version {
		return nil, fail("$.version", "unsupported version")
	}
	if strings.TrimSpace(corpus.Generation) == "" || len(corpus.Generation) > 128 {
		return nil, fail("$.generation", "generation required")
	}
	if len(corpus.Assertions) < 1 || len(corpus.Assertions) > 10000 {
		return nil, fail("$.assertions", "count out of bounds")
	}
	seen := make(map[string]bool, len(corpus.Assertions))
	for i := range corpus.Assertions {
		path := fmt.Sprintf("$.assertions[%d]", i)
		if err := validateAssertion(&corpus.Assertions[i]); err != nil {
			var e *Error
			if errors.As(err, &e) {
				return nil, fail(path+"."+e.Path, e.Reason)
			}
			return nil, err
		}
		if seen[corpus.Assertions[i].ID] {
			return nil, fail(path+".id", "duplicate assertion id")
		}
		seen[corpus.Assertions[i].ID] = true
	}
	return &corpus, nil
}

func validateAssertion(a *Assertion) error {
	if !uuidPattern.MatchString(a.ID) {
		return fail("id", "canonical uuid required")
	}
	if !oneOf(a.Source, "ANP", "COMMUNITY", "OSM", "OCM") {
		return fail("source", "unknown source")
	}
	if !oneOf(a.Kind, "FUEL_STATION", "CHARGING_SITE", "COMBINED_SITE", "REFERENCE_POI") {
		return fail("kind", "unknown kind")
	}
	if strings.TrimSpace(a.Name) == "" || len([]rune(a.Name)) > 200 {
		return fail("name", "name out of bounds")
	}
	if err := validateIdentities(a); err != nil {
		return err
	}
	if err := validateLocation(a); err != nil {
		return err
	}
	if err := validateServices(a); err != nil {
		return err
	}
	if err := validatePrices(a); err != nil {
		return err
	}
	if err := validateCharging(a); err != nil {
		return err
	}
	for _, value := range []struct{ name, raw string }{
		{"clocks.observed_at", a.Clocks.ObservedAt},
		{"clocks.published_at", a.Clocks.PublishedAt},
		{"clocks.fetched_at", a.Clocks.FetchedAt},
	} {
		if _, err := time.Parse(time.RFC3339, value.raw); err != nil {
			return fail(value.name, "rfc3339 required")
		}
	}
	if a.Rights.Source != a.Source {
		return fail("rights.source", "must match assertion source")
	}
	if a.Rights.License != "ADMITTED" {
		return fail("rights.license", "rights not admitted")
	}
	if strings.TrimSpace(a.Rights.Attribution) == "" || len(a.Rights.Attribution) > 500 {
		return fail("rights.attribution", "attribution required")
	}
	if !hex64.MatchString(a.Rights.SourceSHA256) {
		return fail("rights.source_sha256", "sha256 required")
	}
	if a.Capabilities.PriceKnown != (len(a.Prices) > 0) {
		return fail("capabilities.price_known", "must match prices")
	}
	if a.Capabilities.LiveStatus {
		return fail("capabilities.live_status", "no admitted live feed")
	}
	pinEligible := a.Location.LatitudeDeg != nil && a.Location.LongitudeDeg != nil && a.Location.DisplayPoint != nil &&
		((a.Location.Quality == "reviewed" && (*a.Location.DisplayPoint == "station" || *a.Location.DisplayPoint == "entrance")) ||
			(a.Location.Quality == "osm-mapped" && (*a.Location.DisplayPoint == "osm_node" || *a.Location.DisplayPoint == "osm_centroid")))
	if a.Capabilities.PinEligible != pinEligible {
		return fail("capabilities.pin_eligible", "must match admitted geometry")
	}
	if a.Capabilities.EntranceKnown != (a.Location.EntranceLatitudeDeg != nil && a.Location.EntranceLongitudeDeg != nil) {
		return fail("capabilities.entrance_known", "must match entrance geometry")
	}
	return nil
}

func validateIdentities(a *Assertion) error {
	if a.Identities.StationUUID != nil && !uuidPattern.MatchString(*a.Identities.StationUUID) {
		return fail("identities.station_uuid", "canonical uuid required")
	}
	if a.Source == "ANP" || a.Source == "COMMUNITY" {
		if a.Identities.StationUUID == nil {
			return fail("identities.station_uuid", "station uuid required")
		}
	}
	for name, value := range map[string]*string{
		"place_id": a.Identities.PlaceID, "equipment_id": a.Identities.EquipmentID, "connector_id": a.Identities.ConnectorID,
	} {
		if value != nil && !uuidPattern.MatchString(*value) {
			return fail("identities."+name, "canonical uuid required")
		}
	}
	if a.Identities.CNPJ != nil && !cnpjPattern.MatchString(*a.Identities.CNPJ) {
		return fail("identities.cnpj", "14 alphanumeric characters required")
	}
	if (a.Identities.OSMType == nil) != (a.Identities.OSMID == nil) {
		return fail("identities", "osm type and id travel together")
	}
	if a.Identities.OSMType != nil && !oneOf(*a.Identities.OSMType, "node", "way", "relation") {
		return fail("identities.osm_type", "unknown osm type")
	}
	if a.Identities.OSMID != nil && *a.Identities.OSMID < 1 {
		return fail("identities.osm_id", "positive id required")
	}
	return nil
}

func validateLocation(a *Assertion) error {
	if !oneOf(a.Location.Quality, "reviewed", "osm-mapped", "unknown", "city-centroid") {
		return fail("location.quality", "unknown quality")
	}
	if (a.Location.LatitudeDeg == nil) != (a.Location.LongitudeDeg == nil) {
		return fail("location", "latitude and longitude travel together")
	}
	if a.Location.LatitudeDeg != nil {
		lat, err := strconv.ParseFloat(*a.Location.LatitudeDeg, 64)
		if err != nil || lat < -90 || lat > 90 {
			return fail("location.latitude_deg", "out of range")
		}
		lon, err := strconv.ParseFloat(*a.Location.LongitudeDeg, 64)
		if err != nil || lon < -180 || lon > 180 {
			return fail("location.longitude_deg", "out of range")
		}
	}
	if a.Location.Quality == "reviewed" {
		if a.Location.LatitudeDeg == nil || a.Location.DisplayPoint == nil ||
			(*a.Location.DisplayPoint != "station" && *a.Location.DisplayPoint != "entrance") {
			return fail("location", "reviewed geometry requires station/entrance display point")
		}
	} else if a.Location.Quality == "osm-mapped" {
		if a.Location.LatitudeDeg == nil || a.Location.DisplayPoint == nil ||
			(*a.Location.DisplayPoint != "osm_node" && *a.Location.DisplayPoint != "osm_centroid") {
			return fail("location", "osm geometry requires an osm display point")
		}
	} else if a.Location.Quality == "unknown" {
		if a.Location.LatitudeDeg != nil || a.Location.DisplayPoint != nil {
			return fail("location", "unknown geometry cannot carry a display point")
		}
	} else {
		if a.Location.DisplayPoint == nil || *a.Location.DisplayPoint != "city_centroid" {
			return fail("location.display_point", "city centroid is not an exact pin")
		}
	}
	if (a.Location.EntranceLatitudeDeg == nil) != (a.Location.EntranceLongitudeDeg == nil) {
		return fail("location", "entrance coordinates travel together")
	}
	if a.Location.EntranceLatitudeDeg != nil {
		if a.Location.Quality != "reviewed" {
			return fail("location.entrance", "entrance requires reviewed geometry")
		}
		lat, err := strconv.ParseFloat(*a.Location.EntranceLatitudeDeg, 64)
		if err != nil || lat < -90 || lat > 90 {
			return fail("location.entrance_latitude_deg", "out of range")
		}
		lon, err := strconv.ParseFloat(*a.Location.EntranceLongitudeDeg, 64)
		if err != nil || lon < -180 || lon > 180 {
			return fail("location.entrance_longitude_deg", "out of range")
		}
	}
	return nil
}

func validateServices(a *Assertion) error {
	if len(a.Services) < 1 || len(a.Services) > 2 {
		return fail("services", "one or two services required")
	}
	seen := map[string]bool{}
	for _, service := range a.Services {
		if !oneOf(service, "FUEL", "CHARGING") {
			return fail("services", "unknown service")
		}
		if seen[service] {
			return fail("services", "duplicate service")
		}
		seen[service] = true
	}
	return nil
}

func validatePrices(a *Assertion) error {
	if len(a.Prices) > 20 {
		return fail("prices", "too many prices")
	}
	seen := map[string]bool{}
	for i, p := range a.Prices {
		path := fmt.Sprintf("prices[%d]", i)
		if p.Source != a.Source || (p.Source != "ANP" && p.Source != "COMMUNITY") {
			return fail(path+".source", "price source must match assertion source")
		}
		if a.Identities.StationUUID == nil || p.StationUUID != *a.Identities.StationUUID {
			return fail(path+".station_uuid", "must match station identity")
		}
		if !validProductUnit(p.Product, p.Unit) {
			return fail(path+".unit", "incompatible product/unit")
		}
		if p.AmountMilliBRL < 1 || p.AmountMilliBRL > 1000000 {
			return fail(path+".amount_milli_brl", "out of range")
		}
		if _, err := time.Parse("2006-01-02", p.ObservedAt); err != nil {
			return fail(path+".observed_at", "date required")
		}
		if (p.PeriodStart == nil) != (p.PeriodEnd == nil) {
			return fail(path+".period", "period bounds travel together")
		}
		if p.Source == "ANP" && p.PeriodStart == nil {
			return fail(path+".period", "ANP survey period required")
		}
		if p.Source == "COMMUNITY" && p.PeriodStart != nil {
			return fail(path+".period", "community projection has no survey period")
		}
		if p.PeriodStart != nil {
			start, errStart := time.Parse("2006-01-02", *p.PeriodStart)
			end, errEnd := time.Parse("2006-01-02", *p.PeriodEnd)
			if errStart != nil || errEnd != nil || start.After(end) {
				return fail(path+".period", "valid ordered dates required")
			}
		}
		key := p.Source + "|" + p.Product + "|" + p.Unit
		if seen[key] {
			return fail(path, "duplicate source/product/unit")
		}
		seen[key] = true
	}
	return nil
}

func validateCharging(a *Assertion) error {
	if a.Charging == nil {
		if hasService(a.Services, "CHARGING") && a.Source == "OCM" {
			return fail("charging", "OCM charging site requires connector facts")
		}
		return nil
	}
	if a.Source != "OCM" && a.Source != "COMMUNITY" {
		return fail("charging", "source cannot publish charging facts")
	}
	c := a.Charging
	for name, value := range map[string]string{
		"place_id": c.PlaceID, "equipment_id": c.EquipmentID, "connector_id": c.ConnectorID,
	} {
		if !uuidPattern.MatchString(value) {
			return fail("charging."+name, "canonical uuid required")
		}
	}
	if a.Identities.PlaceID != nil && *a.Identities.PlaceID != c.PlaceID {
		return fail("charging.place_id", "must match identity")
	}
	if a.Identities.EquipmentID != nil && *a.Identities.EquipmentID != c.EquipmentID {
		return fail("charging.equipment_id", "must match identity")
	}
	if a.Identities.ConnectorID != nil && *a.Identities.ConnectorID != c.ConnectorID {
		return fail("charging.connector_id", "must match identity")
	}
	if strings.TrimSpace(c.ConnectorCode) == "" || len(c.ConnectorCode) > 64 {
		return fail("charging.connector_code", "code required")
	}
	if c.PowerW != nil && (*c.PowerW < 0 || *c.PowerW > 1000000000) {
		return fail("charging.power_w", "out of range")
	}
	if !oneOf(c.Access, "public", "private", "unknown") {
		return fail("charging.access", "unknown access")
	}
	if !oneOf(c.Status, "unknown", "available", "unavailable") {
		return fail("charging.status", "unknown status")
	}
	if c.TariffMilliBRLPerKWh != nil && (*c.TariffMilliBRLPerKWh < 1 || *c.TariffMilliBRLPerKWh > 1000000000) {
		return fail("charging.tariff_milli_brl_per_kwh", "out of range")
	}
	if c.TariffCondition != nil && !oneOf(*c.TariffCondition, "STANDARD", "MEMBER", "SESSION", "PARKING") {
		return fail("charging.tariff_condition", "unknown condition")
	}
	if c.TariffMilliBRLPerKWh == nil && c.TariffCondition != nil {
		return fail("charging.tariff_condition", "condition without tariff")
	}
	if _, err := time.Parse(time.RFC3339, c.ObservedAt); err != nil {
		return fail("charging.observed_at", "rfc3339 required")
	}
	return nil
}

// ParseNativeManifest validates engine/generation/dependency/rights metadata
// before a package set can be activated.
func ParseNativeManifest(data []byte) (*NativeManifest, error) {
	var m NativeManifest
	if err := decode(data, &m); err != nil {
		return nil, err
	}
	if m.Format != ManifestFormat || m.Version != Version {
		return nil, fail("$", "unsupported manifest format/version")
	}
	if strings.TrimSpace(m.Generation) == "" || len(m.Generation) > 128 {
		return nil, fail("generation", "generation required")
	}
	if m.Engine.Name != "comaps" || !hex40.MatchString(m.Engine.UpstreamSHA) || !hex40.MatchString(m.Engine.GeneratorSHA) ||
		strings.TrimSpace(m.Engine.FormatVersion) == "" {
		return nil, fail("engine", "pinned engine metadata required")
	}
	if !regionID.MatchString(m.Region.ID) || strings.TrimSpace(m.Region.Name) == "" ||
		strings.TrimSpace(m.Region.Overview) == "" {
		return nil, fail("region", "region metadata required")
	}
	if m.Region.BBox.MinLonDeg < -180 || m.Region.BBox.MaxLonDeg > 180 ||
		m.Region.BBox.MinLatDeg < -90 || m.Region.BBox.MaxLatDeg > 90 ||
		m.Region.BBox.MinLonDeg >= m.Region.BBox.MaxLonDeg || m.Region.BBox.MinLatDeg >= m.Region.BBox.MaxLatDeg {
		return nil, fail("region.bbox", "invalid bounds")
	}
	if m.Dependencies == nil {
		// Same rule as the Android parser: the key is a JSON array, never null.
		return nil, fail("dependencies", "array required")
	}
	if len(m.Packages) < 1 || len(m.Packages) > 200 {
		return nil, fail("packages", "count out of bounds")
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for i, p := range m.Packages {
		path := fmt.Sprintf("packages[%d]", i)
		if !regionID.MatchString(p.ID) || !oneOf(p.Role, "REGION", "OVERVIEW", "SEARCH", "ROUTING", "STYLE", "FONT") {
			return nil, fail(path, "invalid package id/role")
		}
		if !ValidPackagePath(p.Path) {
			return nil, fail(path+".path", "unsafe package path")
		}
		if p.Bytes < 1 || p.Bytes > 4294967296 {
			return nil, fail(path+".bytes", "out of range")
		}
		if !hex64.MatchString(p.SHA256) {
			return nil, fail(path+".sha256", "sha256 required")
		}
		if ids[p.ID] || paths[p.Path] {
			return nil, fail(path, "duplicate package id/path")
		}
		ids[p.ID] = true
		paths[p.Path] = true
	}
	seenDeps := map[string]bool{}
	for i, dep := range m.Dependencies {
		if !regionID.MatchString(dep) || seenDeps[dep] || !ids[dep] {
			return nil, fail(fmt.Sprintf("dependencies[%d]", i), "dependency must be a listed package")
		}
		seenDeps[dep] = true
	}
	if len(m.Sources) < 1 || len(m.Sources) > 50 {
		return nil, fail("sources", "source inventory required")
	}
	licenses := map[string]bool{}
	for i, s := range m.Sources {
		path := fmt.Sprintf("sources[%d]", i)
		if strings.TrimSpace(s.ID) == "" || len(s.ID) > 120 ||
			!oneOf(s.License, "ODbL-1.0", "Apache-2.0", "MIT", "CC-BY-4.0") ||
			strings.TrimSpace(s.Attribution) == "" || !hex64.MatchString(s.SHA256) {
			return nil, fail(path, "source inventory invalid")
		}
		licenses[s.License] = true
	}
	if !oneOf(m.Rights.License, "ODbL-1.0", "Apache-2.0", "MIT", "CC-BY-4.0") ||
		strings.TrimSpace(m.Rights.Attribution) == "" ||
		!m.Rights.Commercial || !m.Rights.Offline || !m.Rights.Redistribution {
		return nil, fail("rights", "commercial offline redistribution rights required")
	}
	if !licenses[m.Rights.License] {
		return nil, fail("rights.license", "rights license absent from sources")
	}
	if _, err := time.Parse(time.RFC3339, m.GeneratedAt); err != nil {
		return nil, fail("generated_at", "rfc3339 required")
	}
	return &m, nil
}

func validProductUnit(product, unit string) bool {
	switch product {
	case "CNG":
		return unit == "M3"
	case "LPG_P13":
		return unit == "KG_13"
	case "ETHANOL_HYDRATED", "GASOLINE_REGULAR", "GASOLINE_ADDITIVED", "GASOLINE_PREMIUM_GRADE", "DIESEL_S500", "DIESEL_S10":
		return unit == "L"
	default:
		return false
	}
}

// ValidPackagePath is the manifest package path rule shared with the Kotlin
// `isSafeMapPackagePath`. CoMaps country files are named after the exact
// country id, which may hold single interior spaces ("Brazil_Mato
// Grosso.mwm"). Spaces are admitted only between two path characters: never
// leading/trailing (also per "/" segment), never doubled. "..", absolute
// paths, control and non-ASCII characters stay refused; the 160-byte bound is
// unchanged.
func ValidPackagePath(path string) bool {
	return len(path) >= 1 && len(path) <= 160 &&
		packagePath.MatchString(path) &&
		!strings.Contains(path, "..") &&
		!strings.HasPrefix(path, "/") &&
		!strings.Contains(path, " /") &&
		!strings.Contains(path, "/ ")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func hasService(services []string, want string) bool {
	for _, service := range services {
		if service == want {
			return true
		}
	}
	return false
}
