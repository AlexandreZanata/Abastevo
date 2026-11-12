package adapters

import "log/slog"

func (StorageConfig) String() string   { return "orgmedia: storage configuration redacted" }
func (StorageConfig) GoString() string { return "orgmedia: storage configuration redacted" }
func (StorageConfig) LogValue() slog.Value {
	return slog.StringValue("orgmedia: storage configuration redacted")
}
func (S3) String() string           { return "orgmedia: storage handle redacted" }
func (S3) GoString() string         { return "orgmedia: storage handle redacted" }
func (S3) LogValue() slog.Value     { return slog.StringValue("orgmedia: storage handle redacted") }
func (Tokens) String() string       { return "orgmedia: token handle redacted" }
func (Tokens) GoString() string     { return "orgmedia: token handle redacted" }
func (Tokens) LogValue() slog.Value { return slog.StringValue("orgmedia: token handle redacted") }
