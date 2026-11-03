package prepared

import (
	"crypto/sha256"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// CatalogIdentity contains the stable prepared site key and its root sources.
type CatalogIdentity struct {
	Key        string
	SourceKeys []string
}

// CatalogPublicIdentity is the current primary ID and all other reachable IDs.
type CatalogPublicIdentity struct {
	ID      string
	Aliases []string
}

// ResolveCatalogIdentities binds root source keys to the previous generation.
// All old IDs survive a merge. A split that would share an ID is refused.
func ResolveCatalogIdentities(sites []CatalogIdentity, previous map[string]CatalogPublicIdentity) ([]CatalogPublicIdentity, error) {
	if len(sites) < 1 || len(sites) > 205000 {
		return nil, errors.New("prepared: catalog identity count refused")
	}
	result := make([]CatalogPublicIdentity, 0, len(sites))
	claimedSites := make(map[string]bool, len(sites))
	claimedSources := make(map[string]bool, len(sites))
	claimedIDs := make(map[string]bool, len(sites))
	for _, site := range sites {
		digest, ok := strings.CutPrefix(site.Key, "ev-site-v1:")
		if !ok || !validHash(digest) || claimedSites[site.Key] || len(site.SourceKeys) < 1 || len(site.SourceKeys) > 2 {
			return nil, errors.New("prepared: invalid or duplicate catalog identity")
		}
		claimedSites[site.Key] = true
		primary := ""
		ids := map[string]bool{}
		namespaces := map[string]bool{}
		for _, source := range site.SourceKeys {
			namespace, ok := catalogSourceNamespace(source)
			if !ok || namespaces[namespace] || claimedSources[source] {
				return nil, errors.New("prepared: invalid or duplicate catalog source identity")
			}
			namespaces[namespace], claimedSources[source] = true, true
			prior, found := previous[source]
			if !found {
				continue
			}
			if !canonicalCatalogUUID(prior.ID) {
				return nil, errors.New("prepared: invalid previous public identity")
			}
			if primary == "" || prior.ID < primary {
				primary = prior.ID
			}
			ids[prior.ID] = true
			for _, alias := range prior.Aliases {
				if !canonicalCatalogUUID(alias) {
					return nil, errors.New("prepared: invalid previous public alias")
				}
				ids[alias] = true
			}
		}
		if primary == "" {
			primary = freshCatalogUUID(site.Key)
			ids[primary] = true
		}
		identity := CatalogPublicIdentity{ID: primary}
		for id := range ids {
			if claimedIDs[id] {
				return nil, errors.New("prepared: catalog identity split requires review")
			}
			claimedIDs[id] = true
			if id != primary {
				identity.Aliases = append(identity.Aliases, id)
			}
		}
		slices.Sort(identity.Aliases)
		result = append(result, identity)
	}
	return result, nil
}

func catalogSourceNamespace(key string) (string, bool) {
	namespace, identifier, found := strings.Cut(key, ":")
	if !found || (namespace != "osm" && namespace != "ocm") {
		return "", false
	}
	if namespace == "osm" {
		var kind string
		kind, identifier, found = strings.Cut(identifier, ":")
		if !found || (kind != "node" && kind != "way" && kind != "relation") {
			return "", false
		}
	}
	id, err := strconv.ParseInt(identifier, 10, 64)
	return namespace, err == nil && id > 0 && identifier == strconv.FormatInt(id, 10)
}

func canonicalCatalogUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil && id.Valid && hexUUID(id) == value
}

func freshCatalogUUID(siteKey string) string {
	sum := sha256.Sum256([]byte("abastevo:charging-public:v1\x00" + siteKey))
	var id pgtype.UUID
	copy(id.Bytes[:], sum[:16])
	id.Bytes[6] = (id.Bytes[6] & 0x0f) | 0x80 // Application-defined UUID version.
	id.Bytes[8] = (id.Bytes[8] & 0x3f) | 0x80
	id.Valid = true
	return hexUUID(id)
}
