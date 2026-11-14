package adapters_test

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

// withSegment inserts one length-delimited marker segment right after SOI.
func withSegment(t *testing.T, raw []byte, marker byte, payload []byte) []byte {
	t.Helper()
	if len(payload)+2 > 0xffff {
		t.Fatal("segment too long")
	}
	n := len(payload) + 2
	out := append([]byte{0xff, 0xd8, 0xff, marker, byte(n >> 8), byte(n)}, payload...)
	return append(out, raw[2:]...)
}

// scanStart returns the offset of the first entropy-coded byte.
func scanStart(t *testing.T, raw []byte) int {
	t.Helper()
	for i := 2; i+3 < len(raw); {
		if raw[i] != 0xff {
			t.Fatal("unexpected fixture structure")
		}
		marker := raw[i+1]
		length := int(raw[i+2])<<8 | int(raw[i+3])
		if marker == 0xda {
			return i + 2 + length
		}
		i += 2 + length
	}
	t.Fatal("fixture has no scan")
	return 0
}

// withScanBytes places bytes (without 0xFF) at the start of the scan data.
// The structure stays valid; content inspection must refuse before decoding.
func withScanBytes(t *testing.T, raw, payload []byte) []byte {
	t.Helper()
	at := scanStart(t, raw)
	out := append(append(bytes.Clone(raw[:at]), payload...), raw[at:]...)
	return out
}

// withDimensions rewrites the baseline frame header.
func withDimensions(t *testing.T, raw []byte, width, height int) []byte {
	t.Helper()
	out := bytes.Clone(raw)
	for i := 2; i+8 < len(out); {
		marker := out[i+1]
		length := int(out[i+2])<<8 | int(out[i+3])
		if marker == 0xc0 || marker == 0xc2 {
			out[i+5], out[i+6] = byte(height>>8), byte(height)
			out[i+7], out[i+8] = byte(width>>8), byte(width)
			return out
		}
		i += 2 + length
	}
	t.Fatal("fixture has no frame header")
	return nil
}

// EMP-DOC-014 (B-BR-EMP-252): a structurally valid JPEG that carries a
// trailing payload or an active-content signature is unsafe; content that is
// not an admissible JPEG is unsupported; neither verdict changes the bytes.
func TestJPEGInspectorSeparatesUnsafeFromUnsupportedContent(t *testing.T) {
	inspector := adapters.JPEGInspector{}
	clean := originalJPEG(t)
	xmp := []byte(`http://ns.adobe.com/xap/1.0/` + "\x00" + `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`)
	for name, raw := range map[string][]byte{
		"synthetic-original": clean,
		"xmp-metadata":       withSegment(t, clean, 0xe1, xmp),
		"encoder-comment":    withSegment(t, clean, 0xfe, []byte("CREATOR: synthetic encoder, quality = 90")),
		"icc-like-binary":    withSegment(t, clean, 0xe2, append([]byte("ICC_PROFILE\x00\x01\x01"), bytes.Repeat([]byte{0x00, 0x10, 0x7f, 0x3c}, 64)...)),
	} {
		before := bytes.Clone(raw)
		if mime, err := inspector.Inspect(t.Context(), raw); err != nil || mime != "image/jpeg" {
			t.Fatal("clean JPEG refused", name, err)
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("inspection changed the original", name)
		}
	}
	unsafe := map[string][]byte{
		"trailing-archive":   append(bytes.Clone(clean), []byte("PK\x03\x04synthetic")...),
		"trailing-document":  append(bytes.Clone(clean), []byte("%PDF-1.7 synthetic")...),
		"trailing-text":      append(bytes.Clone(clean), []byte("synthetic")...),
		"comment-script":     withSegment(t, clean, 0xfe, []byte("<script>synthetic()</script>")),
		"comment-upper":      withSegment(t, clean, 0xfe, []byte("<SCRIPT>synthetic()</SCRIPT>")),
		"app-markup":         withSegment(t, clean, 0xe1, []byte("synthetic<html><body>synthetic")),
		"app-svg":            withSegment(t, clean, 0xed, []byte("<svg onload=synthetic>")),
		"app-php":            withSegment(t, clean, 0xec, []byte("<?php synthetic(); ?>")),
		"comment-entity":     withSegment(t, clean, 0xfe, []byte(`<!DOCTYPE x [<!ENTITY e SYSTEM "file:///synthetic">]>`)),
		"comment-url-script": withSegment(t, clean, 0xfe, []byte("JavaScript:synthetic")),
		"app-document":       withSegment(t, clean, 0xef, []byte("%PDF-1.7 synthetic")),
		"app-archive":        withSegment(t, clean, 0xe3, []byte("PK\x03\x04synthetic")),
		"app-elf":            withSegment(t, clean, 0xe4, []byte("\x7fELFsynthetic")),
		"app-pe":             withSegment(t, clean, 0xe5, []byte("MZ\x90\x00synthetic")),
		"app-rar":            withSegment(t, clean, 0xe6, []byte("Rar!\x1a\x07\x00synthetic")),
		"app-7z":             withSegment(t, clean, 0xe7, []byte("7z\xbc\xaf\x27\x1csynthetic")),
		"scan-iframe":        withScanBytes(t, clean, []byte("<iframe src=synthetic>")),
		"scan-url-script":    withScanBytes(t, clean, []byte("javascript:synthetic")),
	}
	for name, raw := range unsafe {
		before := bytes.Clone(raw)
		if _, err := inspector.Inspect(t.Context(), raw); !errors.Is(err, domain.ErrUnsafe) {
			t.Fatal("unsafe JPEG not refused as unsafe", name, err)
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("inspection changed the refused bytes", name)
		}
	}
	// Short signatures are metadata-only: inside entropy-coded data they
	// occur by chance in honest photos and must not quarantine them.
	if _, err := inspector.Inspect(t.Context(), withScanBytes(t, clean, []byte("<svg"))); errors.Is(err, domain.ErrUnsafe) {
		t.Fatal("short signature inside scan data quarantined", err)
	}
	unsupported := map[string][]byte{
		"pdf":        []byte("%PDF-1.7 synthetic /JavaScript"),
		"png":        append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 64)...),
		"html":       []byte("<html><script>synthetic()</script></html>"),
		"archive":    []byte("PK\x03\x04synthetic"),
		"executable": []byte("MZ\x90\x00synthetic"),
		"truncated":  clean[:len(clean)-2],
		"bomb":       withDimensions(t, clean, 20000, 20000),
		"empty":      nil,
	}
	for name, raw := range unsupported {
		if _, err := inspector.Inspect(t.Context(), raw); !errors.Is(err, domain.ErrUnsupported) {
			t.Fatal("inadmissible content not refused as unsupported", name, err)
		}
	}
}

// An oversized stored object is an integrity mismatch, not an outage, and a
// confirmed delete needs a distinct absence observation.
func TestS3OversizedReadsAreIntegrityFailuresAndDeleteConfirmsAbsence(t *testing.T) {
	scope, _, object, _ := tokenFixture(t)
	credentials := storage.Credentials{AccessKeyID: "synthetic", SecretAccessKey: "synthetic-secret"}
	var deletes, heads int
	present := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write(bytes.Repeat([]byte{'x'}, int(object.Bytes)+1))
		case http.MethodDelete:
			deletes++
			w.WriteHeader(http.StatusNoContent)
		case http.MethodHead:
			heads++
			if present {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	store, err := adapters.NewS3(adapters.StorageConfig{Endpoint: server.URL, Bucket: "private-fixture", Credentials: credentials, AllowLoopback: true, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := store.Get(t.Context(), scope, object); !errors.Is(err, domain.ErrIntegrity) || raw != nil {
		t.Fatal("oversized stored object treated as an outage", err)
	}
	if err := store.Delete(t.Context(), scope, object); !errors.Is(err, domain.ErrUnavailable) || deletes != 1 || heads != 1 {
		t.Fatal("delete confirmed while the copy is still present", err)
	}
	present = false
	if err := store.Delete(t.Context(), scope, object); err != nil || deletes != 2 || heads != 2 {
		t.Fatal("confirmed delete refused", err)
	}
	foreign := object
	foreign.OrgID = strings.Replace(object.OrgID, "7b0e2a54", "7b0e2a55", 1)
	if err := store.Delete(t.Context(), scope, foreign); !errors.Is(err, domain.ErrInvalid) || deletes != 2 {
		t.Fatal("delete reached another organization's key", err)
	}
}
