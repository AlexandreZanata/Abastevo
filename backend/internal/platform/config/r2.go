package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

// R2 environment variable names. Endpoint, bucket and both key parts are
// all-or-nothing: any one set requires the rest, so a half-configured
// process fails at startup instead of minting broken authorizations.
const (
	keyR2Endpoint = "ANPFUEL_R2_ENDPOINT"
	keyR2Bucket   = "ANPFUEL_R2_BUCKET"
	keyR2Region   = "ANPFUEL_R2_REGION"
	keyR2Access   = "ANPFUEL_R2_ACCESS_KEY_ID"
	keyR2Secret   = "ANPFUEL_R2_SECRET_ACCESS_KEY"
)

// R2Config is the validated private-storage identity: endpoint, bucket
// and least-privilege key pair. Nil means storage is unconfigured and
// upload issuance refuses with 503 instead of misbehaving.
type R2Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// r2LogValue renders endpoint and bucket for operations. Key material
// never appears here, in errors or anywhere else in logs.
func (r *R2Config) r2LogValue() slog.Value {
	if r == nil {
		return slog.GroupValue(slog.Bool("configured", false))
	}
	return slog.GroupValue(
		slog.Bool("configured", true),
		slog.String("endpoint", r.Endpoint),
		slog.String("bucket", r.Bucket),
		slog.String("region", r.Region),
	)
}

// loadR2 validates the storage section. Absent means disabled; present
// means complete, with TLS except loopback emulators and no credentials
// in endpoint URLs. Errors name variables, never values.
func loadR2(getenv func(string) (string, bool)) (*R2Config, error) {
	endpoint, endpointSet := getenv(keyR2Endpoint)
	bucket, bucketSet := getenv(keyR2Bucket)
	access, accessSet := getenv(keyR2Access)
	secret, _ := getenv(keyR2Secret)
	present := endpointSet && endpoint != "" || bucketSet && bucket != "" ||
		accessSet && access != "" || secret != ""
	if !present {
		return nil, nil
	}
	if endpoint == "" {
		return nil, fmt.Errorf("%s is required with any R2 setting", keyR2Endpoint)
	}
	if bucket == "" {
		return nil, fmt.Errorf("%s is required with any R2 setting", keyR2Bucket)
	}
	if access == "" {
		return nil, fmt.Errorf("%s is required with any R2 setting", keyR2Access)
	}
	if secret == "" {
		return nil, fmt.Errorf("%s is required with any R2 setting", keyR2Secret)
	}
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("%s must be an https URL without credentials", keyR2Endpoint)
	}
	switch u.Scheme {
	case "https":
	case "http":
		host := u.Hostname()
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			return nil, fmt.Errorf("%s must be https outside loopback emulators", keyR2Endpoint)
		}
	default:
		return nil, fmt.Errorf("%s must be an https URL without credentials", keyR2Endpoint)
	}
	region, _ := getenv(keyR2Region)
	if strings.TrimSpace(region) == "" {
		region = "auto"
	}
	return &R2Config{
		Endpoint: strings.TrimRight(strings.TrimSpace(endpoint), "/"),
		Bucket:   bucket, Region: region,
		AccessKeyID: access, SecretAccessKey: secret,
	}, nil
}
