package charging

const NationalCatalogProfile = "charging-br-national-v1"

// SourceAttribution contains only public source identity and admitted terms.
// An OCM assertion carries its own provider ID, including in a combined site.
type SourceAttribution struct {
	Source      string `json:"source"`
	ExternalID  string `json:"external_id"`
	Revision    string `json:"revision"`
	ProviderID  *int64 `json:"provider_id,omitempty"`
	License     string `json:"license"`
	Attribution string `json:"attribution"`
}
