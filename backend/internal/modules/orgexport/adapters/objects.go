package adapters

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// transferTimeout bounds one private transfer, as for documents.
const transferTimeout = 20 * time.Second

// StorageConfig is the private bucket of the existing storage provider.
type StorageConfig struct {
	Endpoint, Bucket, Region string
	Credentials              storage.Credentials
	Client                   *http.Client
	AllowLoopback            bool
}

func (StorageConfig) String() string   { return "orgexport: storage configuration redacted" }
func (StorageConfig) GoString() string { return "orgexport: storage configuration redacted" }
func (StorageConfig) LogValue() slog.Value {
	return slog.StringValue("orgexport: storage configuration redacted")
}

// S3 stores export files with the existing SigV4 core and the
// redirect-refusing bounded transport, server-side only. Keys are derived
// from validated server facts under org/<org_id>/export/<export_id>/.
type S3 struct {
	input       storage.PresignInput
	credentials storage.Credentials
	client      *http.Client
}

var _ app.Objects = (*S3)(nil)

func (S3) String() string       { return "orgexport: storage handle redacted" }
func (S3) GoString() string     { return "orgexport: storage handle redacted" }
func (S3) LogValue() slog.Value { return slog.StringValue("orgexport: storage handle redacted") }

// NewS3 requires HTTPS, except an explicit loopback-only fixture option.
func NewS3(config StorageConfig) (*S3, error) {
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.ForceQuery {
		return nil, domain.ErrInvalid
	}
	if endpoint.Scheme != "https" {
		ip := net.ParseIP(endpoint.Hostname())
		if endpoint.Scheme != "http" || !config.AllowLoopback || ip == nil || !ip.IsLoopback() {
			return nil, domain.ErrInvalid
		}
	}
	input := storage.PresignInput{Endpoint: config.Endpoint, Bucket: config.Bucket, Region: config.Region, Namespace: "org/", Key: "org/configuration-check", TTL: 5 * time.Minute}
	if _, err := storage.PresignGET(input, config.Credentials); err != nil {
		return nil, domain.ErrInvalid
	}
	client := config.Client
	if client == nil {
		client = &http.Client{Timeout: transferTimeout}
	}
	bounded := *client
	if bounded.Timeout <= 0 || bounded.Timeout > transferTimeout {
		bounded.Timeout = transferTimeout
	}
	return &S3{input: input, credentials: config.Credentials, client: &bounded}, nil
}

func (store *S3) inputFor(scope kernel.OrgScope, object domain.Object) (storage.PresignInput, error) {
	if store == nil {
		return storage.PresignInput{}, app.ErrUnavailable
	}
	key, err := object.Key(scope)
	if err != nil {
		return storage.PresignInput{}, domain.ErrInvalid
	}
	prefix, _ := object.Prefix(scope)
	input := store.input
	input.Key, input.Namespace, input.ContentType, input.MaxBytes, input.Now = key, prefix, object.MediaType, object.Bytes, time.Now()
	return input, nil
}

// Put never replaces a stored object: a successful or refused create is
// followed by a bounded, digest-checked read. A stored copy that differs is
// ErrIntegrity; every transport failure is ErrUnavailable.
func (store *S3) Put(ctx context.Context, scope kernel.OrgScope, object domain.Object, raw []byte) error {
	input, err := store.inputFor(scope, object)
	if err != nil {
		return err
	}
	if err := object.Check(raw); err != nil {
		return domain.ErrIntegrity
	}
	signed, err := storage.PresignServerPUT(input, store.credentials)
	if err != nil {
		return app.ErrUnavailable
	}
	if _, err := storage.PutIfAbsent(ctx, store.client, signed.URL, object.MediaType, raw); err != nil {
		return app.ErrUnavailable
	}
	_, err = store.Get(ctx, scope, object)
	return err
}

// Get reads at most the declared size plus one byte and verifies it.
func (store *S3) Get(ctx context.Context, scope kernel.OrgScope, object domain.Object) ([]byte, error) {
	input, err := store.inputFor(scope, object)
	if err != nil {
		return nil, err
	}
	signed, err := storage.PresignGET(input, store.credentials)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	raw, err := storage.Get(ctx, store.client, signed.URL, object.Bytes)
	if errors.Is(err, storage.ErrTooLarge) {
		return nil, domain.ErrIntegrity
	}
	if err != nil {
		return nil, app.ErrUnavailable
	}
	if err := object.Check(raw); err != nil {
		return nil, domain.ErrIntegrity
	}
	return raw, nil
}

// Delete removes the object and confirms its absence.
func (store *S3) Delete(ctx context.Context, scope kernel.OrgScope, object domain.Object) error {
	input, err := store.inputFor(scope, object)
	if err != nil {
		return err
	}
	if err := storage.DeleteConfirmed(ctx, store.client, input, store.credentials); err != nil {
		return app.ErrUnavailable
	}
	return nil
}

// Unconfigured is the object port when no private storage is configured:
// every transfer is unavailable, so nothing is ever produced or served.
type Unconfigured struct{}

var _ app.Objects = Unconfigured{}

func (Unconfigured) Put(context.Context, kernel.OrgScope, domain.Object, []byte) error {
	return app.ErrUnavailable
}
func (Unconfigured) Get(context.Context, kernel.OrgScope, domain.Object) ([]byte, error) {
	return nil, app.ErrUnavailable
}
func (Unconfigured) Delete(context.Context, kernel.OrgScope, domain.Object) error {
	return app.ErrUnavailable
}
