package adapters

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
)

// TrainingObjects lands and removes derivative bytes in the private
// bucket under the training namespace (CS10C, AT04): minimized JPEGs
// only, written by the promotion job and deleted by the purge
// sweeper. Every transfer runs over short-lived SigV4 presigned URLs
// minted with the server's storage identity — keys outside the
// namespace refuse before signing, and no object is ever public.
// Compile-time implements the purge ObjectStore port.
type TrainingObjects struct {
	endpoint string
	bucket   string
	region   string
	cred     evidencestorage.Credentials
	client   *http.Client
}

// TrainingObjectConfig carries the private-storage identity. Empty
// values refuse at call time instead of minting broken URLs; the
// worker passes the validated R2/MinIO section through.
type TrainingObjectConfig struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

// NewTrainingObjects wires the private bucket transfers.
func NewTrainingObjects(cfg TrainingObjectConfig) *TrainingObjects {
	return &TrainingObjects{
		endpoint: cfg.Endpoint, bucket: cfg.Bucket, region: cfg.Region,
		cred:   evidencestorage.Credentials{AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey},
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

var _ privacyapp.ObjectStore = (*TrainingObjects)(nil)

func (s *TrainingObjects) presign(method, key string) (string, error) {
	if !strings.HasPrefix(key, privacyapp.TrainingNamespace) {
		return "", errors.New("training objects: key outside namespace")
	}
	in := evidencestorage.PresignInput{
		Endpoint: s.endpoint, Bucket: s.bucket, Key: key,
		Namespace: privacyapp.TrainingNamespace, Region: s.region,
		TTL: 5 * time.Minute, Now: time.Now(),
	}
	switch method {
	case http.MethodPut:
		in.ContentType = "image/jpeg"
		in.MaxBytes = 3 << 20
		pre, err := evidencestorage.PresignPUT(in, s.cred)
		if err != nil {
			return "", err
		}
		return pre.URL, nil
	case http.MethodGet:
		pre, err := evidencestorage.PresignGET(in, s.cred)
		if err != nil {
			return "", err
		}
		return pre.URL, nil
	case http.MethodDelete:
		pre, err := evidencestorage.PresignDELETE(in, s.cred)
		if err != nil {
			return "", err
		}
		return pre.URL, nil
	default:
		return "", errors.New("training objects: unsupported method")
	}
}

// Put lands one minimized derivative under its inventoried key.
func (s *TrainingObjects) Put(ctx context.Context, key string, body []byte) error {
	rawURL, err := s.presign(http.MethodPut, key)
	if err != nil {
		return err
	}
	return evidencestorage.Put(ctx, s.client, rawURL, "image/jpeg", body)
}

// Exists reports whether one copy is still present. It reads at most
// one content byte: success proves presence, and a bound overflow
// proves it too (a body larger than the bound is still a body).
// Only a 404 reads as absent; anything else errors for retry.
func (s *TrainingObjects) Exists(ctx context.Context, key string) (bool, error) {
	rawURL, err := s.presign(http.MethodGet, key)
	if err != nil {
		return false, err
	}
	_, err = evidencestorage.Get(ctx, s.client, rawURL, 1)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, evidencestorage.ErrTooLarge) {
		return true, nil
	}
	var status *evidencestorage.StatusError
	if errors.As(err, &status) && status.Status == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// Delete removes one copy. Missing keys converge silently (S3 deletes
// are idempotent), so replayed sweeps never fail on absent objects.
func (s *TrainingObjects) Delete(ctx context.Context, key string) error {
	rawURL, err := s.presign(http.MethodDelete, key)
	if err != nil {
		return err
	}
	return evidencestorage.Delete(ctx, s.client, rawURL)
}
