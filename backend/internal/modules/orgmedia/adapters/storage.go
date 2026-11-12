// Package adapters supplies bounded private S3, access-token and audit ports.
package adapters

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

type StorageConfig struct {
	Endpoint, Bucket, Region string
	Credentials              storage.Credentials
	Client                   *http.Client
	AllowLoopback            bool
}
type S3 struct {
	input       storage.PresignInput
	credentials storage.Credentials
	client      *http.Client
}

var _ application.Objects = (*S3)(nil)

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
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	if copy.Timeout <= 0 || copy.Timeout > 20*time.Second {
		copy.Timeout = 20 * time.Second
	}
	return &S3{input: input, credentials: config.Credentials, client: &copy}, nil
}
func (store *S3) inputFor(scope kernel.OrgScope, object domain.Object) (storage.PresignInput, error) {
	key, err := object.Key(scope)
	if err != nil {
		return storage.PresignInput{}, domain.ErrInvalid
	}
	input := store.input
	input.Key = key
	input.Namespace = "org/" + object.OrgID + "/doc/" + object.DocumentID + "/"
	input.ContentType = object.MIME
	input.MaxBytes = object.Bytes
	return input, nil
}
func (store *S3) Put(ctx context.Context, scope kernel.OrgScope, object domain.Object, raw []byte) error {
	input, err := store.inputFor(scope, object)
	if err != nil {
		return err
	}
	if err := object.CheckSnapshot(raw); err != nil {
		return err
	}
	signed, err := storage.PresignServerPUT(input, store.credentials)
	if err != nil {
		return domain.ErrUnavailable
	}
	if _, err := storage.PutIfAbsent(ctx, store.client, signed.URL, object.MIME, raw); err != nil {
		return domain.ErrUnavailable
	}
	// A successful PUT or a 412 is not sufficient proof of stored identity.
	_, err = store.Get(ctx, scope, object)
	return err
}
func (store *S3) Get(ctx context.Context, scope kernel.OrgScope, object domain.Object) ([]byte, error) {
	input, err := store.inputFor(scope, object)
	if err != nil {
		return nil, err
	}
	signed, err := storage.PresignGET(input, store.credentials)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	raw, err := storage.Get(ctx, store.client, signed.URL, object.Bytes)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	if err := object.CheckSnapshot(raw); err != nil {
		return nil, domain.ErrIntegrity
	}
	return raw, nil
}
