package main

import (
	"bytes"
	"errors"
	"image/jpeg"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSyntheticTransferIsBoundedDecodableAndDistinct(t *testing.T) {
	a, e := syntheticJPEG(0)
	if e != nil {
		t.Fatal(e)
	}
	b, e := syntheticJPEG(1)
	if e != nil {
		t.Fatal(e)
	}
	if len(a) != 3<<20 || len(b) != 3<<20 || bytes.Equal(a, b) {
		t.Fatal("invalid bounded fixtures")
	}
	if _, e = jpeg.Decode(bytes.NewReader(a)); e != nil {
		t.Fatal(e)
	}
}
func TestManifestRefusesForeignAndOversizedScope(t *testing.T) {
	now := time.Now()
	m := manifest{Purpose: prefix, SeededAt: now}
	for i := 0; i < 5; i++ {
		id := string(rune('a'+i)) + "0000000-0000-4000-8000-000000000000"
		kind := "abandoned"
		if i == 0 {
			kind = "elapsed"
		}
		m.Copies = append(m.Copies, copyRef{id, "q/" + prefix + id, "f/" + string(bytes.Repeat([]byte{'a'}, 64)), kind, now})
	}
	if e := validate(m); e != nil {
		t.Fatal(e)
	}
	m.Copies[2].Original = "q/foreign"
	if e := validate(m); e == nil {
		t.Fatal("foreign key accepted")
	}
	m.Copies = m.Copies[:4]
	if e := validate(m); e == nil {
		t.Fatal("partial scope accepted")
	}
}

func TestFailureClassificationNeverLogsSignedURL(t *testing.T) {
	e := &url.Error{Op: "Put", URL: "https://test.invalid/?X-Amz-Signature=private-material", Err: errors.New("provider-body-private")}
	got := safeFailure(e)
	if strings.Contains(got, "private") || strings.Contains(got, "test.invalid") {
		t.Fatal("private diagnostic leaked")
	}
}
func TestAllFiveSyntheticObjectsHaveDistinctBytes(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 5; i++ {
		raw, err := syntheticJPEG(i)
		if err != nil {
			t.Fatal(err)
		}
		sum := string(raw)
		if seen[sum] {
			t.Fatal("fixture content key reused")
		}
		seen[sum] = true
	}
}
