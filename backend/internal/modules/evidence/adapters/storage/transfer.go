package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var (
	// ErrTooLarge marks bodies beyond the caller bound: the transport
	// refuses instead of buffering unbounded memory.
	ErrTooLarge = errors.New("storage: body exceeds bound")
)

// StatusError reports a non-2xx object response with a truncated body
// hint field retained for compatibility. Remote error bodies are not retained.
type StatusError struct {
	Status int
	Hint   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("storage: unexpected status %d", e.Status)
}

// Get downloads one object with a hard byte bound: at most limit bytes
// are buffered, and anything beyond fails instead of truncating
// silently, so oversize objects can never shrink into verified bytes.
func Get(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, error) {
	if limit < 1 {
		return nil, ErrTooLarge
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := privateClient(client).Do(req)
	if err != nil {
		return nil, errors.New("storage: transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, statusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, ErrTooLarge
	}
	return raw, nil
}

// Put uploads one object with the exact content type the presigned URL
// covers. A signature mismatch fails server-side; anything else surfaces
// as a status error for retry decisions upstream.
func Put(ctx context.Context, client *http.Client, rawURL, contentType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	// Fixed length avoids chunked encoding, which unsigned-payload query
	// auth does not cover on strict S3 implementations.
	req.ContentLength = int64(len(body))
	resp, err := privateClient(client).Do(req)
	if err != nil {
		return errors.New("storage: transport unavailable")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
	if resp.StatusCode/100 != 2 {
		return statusError(resp)
	}
	return nil
}

// Delete removes one object by presigned URL. A missing key succeeds:
// idempotent deletion converges repeated sweeps, and the store marker is
// the source of truth, not the transport response.
func Delete(ctx context.Context, client *http.Client, rawURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := privateClient(client).Do(req)
	if err != nil {
		return errors.New("storage: transport unavailable")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if resp.StatusCode/100 != 2 {
		return statusError(resp)
	}
	return nil
}

func statusError(resp *http.Response) error {
	return &StatusError{Status: resp.StatusCode}
}

// Object storage is a fixed configured origin. Never follow redirects to
// another service or expose credential-bearing URL errors to job logs.
func privateClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clone
}

// Exists distinguishes confirmed absence from provider/access/transport failure.
// Redirects are refused and HEAD transfers no object body.
func Exists(ctx context.Context, client *http.Client, rawURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return false, err
	}
	resp, err := privateClient(client).Do(req)
	if err != nil {
		return false, errors.New("storage: transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	return false, statusError(resp)
}

// DeleteConfirmed requires a distinct authenticated absence observation. A
// successful DELETE alone is not a byte-erasure receipt.
func DeleteConfirmed(ctx context.Context, client *http.Client, in PresignInput, cred Credentials) error {
	del, err := PresignDELETE(in, cred)
	if err != nil {
		return err
	}
	if err := Delete(ctx, client, del.URL); err != nil {
		return err
	}
	head, err := PresignHEAD(in, cred)
	if err != nil {
		return err
	}
	present, err := Exists(ctx, client, head.URL)
	if err != nil {
		return err
	}
	if present {
		return errors.New("storage: deleted copy still present")
	}
	return nil
}
