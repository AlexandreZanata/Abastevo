package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var (
	// ErrTooLarge marks bodies beyond the caller bound: the transport
	// refuses instead of buffering unbounded memory.
	ErrTooLarge = errors.New("storage: body exceeds bound")
)

// StatusError reports a non-2xx object response with a truncated body
// hint. Bodies stay small and carry no credentials by construction.
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
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
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return statusError(resp)
	}
	return nil
}

func statusError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 128))
	return &StatusError{Status: resp.StatusCode, Hint: strings.TrimSpace(string(raw))}
}
