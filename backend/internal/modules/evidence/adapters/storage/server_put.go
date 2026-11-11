package storage

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
)

// PresignServerPUT reuses the SigV4 core for a server-only transfer. Unlike
// community client intake, the caller owns type/size inspection and namespace.
// The URL stays internal. MaxBytes is enforced by the caller, not by presigning.
func PresignServerPUT(in PresignInput, cred Credentials) (PresignedPUT, error) {
	mediaType, params, err := mime.ParseMediaType(in.ContentType)
	if err != nil || mediaType != in.ContentType || len(params) != 0 || len(mediaType) > 128 {
		return PresignedPUT{}, ErrBadContentType
	}
	if in.MaxBytes < 1 {
		return PresignedPUT{}, ErrBadSize
	}
	raw, expires, err := presign(httpMethodPut, in, cred)
	if err != nil {
		return PresignedPUT{}, err
	}
	return PresignedPUT{URL: raw, RequiredHeaders: map[string]string{"Content-Type": in.ContentType}, ExpiresAt: expires, MaxBytes: in.MaxBytes}, nil
}

// PutIfAbsent never replaces a stored object. Existing state is not proof of
// matching content: the caller must retrieve and verify that private snapshot.
func PutIfAbsent(ctx context.Context, client *http.Client, rawURL, contentType string, body []byte) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, rawURL, bytes.NewReader(body))
	if err != nil {
		return false, ErrBadEndpoint
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("If-None-Match", "*")
	req.ContentLength = int64(len(body))
	response, err := privateClient(client).Do(req)
	if err != nil {
		return false, &StatusError{Status: http.StatusServiceUnavailable}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32<<10))
	if response.StatusCode == http.StatusPreconditionFailed {
		return false, nil
	}
	if response.StatusCode/100 != 2 {
		return false, statusError(response)
	}
	return true, nil
}
