package domain

// Config defines feature gates and operational policy for Forum Abastevo.
type Config struct {
	ReadsEnabled           bool
	WritesEnabled          bool
	NewsCollectionEnabled  bool
	AutoPublicationEnabled bool
	TaxonomyVersion        int
}

// DefaultConfig returns safe production defaults with all forum subsystems disabled.
func DefaultConfig() Config {
	return Config{
		ReadsEnabled:           false,
		WritesEnabled:          false,
		NewsCollectionEnabled:  false,
		AutoPublicationEnabled: false,
		TaxonomyVersion:        1,
	}
}

// ReadsAllowed returns true if public forum read operations are enabled.
func (c Config) ReadsAllowed() bool {
	return c.ReadsEnabled
}

// WritesAllowed returns true if authenticated user writes are enabled.
func (c Config) WritesAllowed() bool {
	return c.WritesEnabled
}

// NewsCollectionAllowed returns true if periodic news fetch worker is enabled.
func (c Config) NewsCollectionAllowed() bool {
	return c.NewsCollectionEnabled
}

// AutoPublicationAllowed returns true if approved news items can be auto-published.
func (c Config) AutoPublicationAllowed() bool {
	return c.AutoPublicationEnabled
}
