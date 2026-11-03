package prepared

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
)

var errCatalogGeography = errors.New("prepared: national catalog geography refused")

// BindCatalogGeography joins a pinned IBGE classification to every projected
// site. The returned census is derived from those rows, not from a sidecar total.
func BindCatalogGeography(bundle NationalBundle, places []NationalPlace, raw []byte, digest, boundaryDigest string) ([]NationalPlace, map[string]int64, error) {
	if !validHash(digest) || !validHash(boundaryDigest) || !validHash(bundle.OfferSHA256) ||
		len(raw) > 64<<20 || bundle.Sites < 1 || bundle.Sites > 205000 || int64(len(places)) != bundle.Sites {
		return nil, nil, errCatalogGeography
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, nil, errCatalogGeography
	}
	var receipt struct {
		Format         string `json:"format"`
		CatalogSHA256  string `json:"catalog_sha256"`
		BoundarySHA256 string `json:"boundary_sha256"`
		Sites          []struct {
			Key    string `json:"key"`
			UF     string `json:"uf"`
			Reason string `json:"reason"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil ||
		receipt.Format != "abastevo.ev-catalog-geography-v1" || receipt.CatalogSHA256 != bundle.OfferSHA256 ||
		receipt.BoundarySHA256 != boundaryDigest || len(receipt.Sites) != len(places) {
		return nil, nil, errCatalogGeography
	}
	states := make(map[string]string, len(places))
	counts := map[string]int64{}
	for _, site := range receipt.Sites {
		if _, repeated := states[site.Key]; repeated || site.Key == "" || !validGeographyReason(site.UF, site.Reason) {
			return nil, nil, errCatalogGeography
		}
		states[site.Key] = site.UF
		counts[site.UF]++
	}
	if _, err := domain.CoverageDrops(nil, counts); err != nil {
		return nil, nil, errCatalogGeography
	}
	out := make([]NationalPlace, len(places))
	for i, place := range places {
		uf, found := states[place.Identity.Key]
		if !found {
			return nil, nil, errCatalogGeography
		}
		place.UF = uf
		out[i] = place
		delete(states, place.Identity.Key)
	}
	return out, counts, nil
}

func validGeographyReason(uf, reason string) bool {
	if uf != "UNASSIGNED" {
		return reason == "ibge_polygon"
	}
	switch reason {
	case "invalid_point", "boundary_ambiguous", "no_boundary_match", "ambiguous_uf":
		return true
	default:
		return false
	}
}
