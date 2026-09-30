package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

var ErrBodyTooLarge = errors.New("httpapi: body too large")

// ReadBody never silently truncates a signed command at its business cap.
func ReadBody(r *http.Request, max int64) ([]byte, error) {
	if r.Body == nil {
		return nil, errors.New("httpapi: body required")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, ErrBodyTooLarge
	}
	return body, nil
}

// DecodeJSON permits exactly one non-null JSON object, with no duplicate
// fields (including case aliases), unknown DTO fields or trailing data.
func DecodeJSON(raw []byte, dst any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("httpapi: object required")
	}
	scan := json.NewDecoder(bytes.NewReader(raw))
	scan.UseNumber()
	if err := uniqueValue(scan, 0); err != nil {
		return err
	}
	if _, err := scan.Token(); err != io.EOF {
		return errors.New("httpapi: trailing JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func uniqueValue(dec *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("httpapi: JSON depth exceeded")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := tok.(string)
			if !ok {
				return errors.New("httpapi: malformed key")
			}
			key = strings.ToLower(key)
			if seen[key] {
				return errors.New("httpapi: duplicate field")
			}
			seen[key] = true
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for dec.More() {
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("httpapi: unexpected delimiter")
	}
	_, err = dec.Token()
	return err
}
