package oidc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnknownKidBurstHasBoundedFetches(t *testing.T) {
	v, _, _ := testSetup(t, "RS256")
	source := &countingSource{inner: v.Keys}
	v.Keys = &CachedKeys{Source: source, TTL: time.Hour, Now: v.Clock}
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	raw := enc(`{"alg":"RS256","kid":"synthetic-never-issued"}`) + "." + enc(`{"iss":"https://accounts.google.com","sub":"synthetic","aud":"anpfuel-backend","exp":1900000000,"nonce":"synthetic"}`) + "." + enc("invalid-signature")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := v.Verify(context.Background(), "google", raw, testAudience, "synthetic"); err == nil {
				t.Error("invalid token admitted")
			}
		}()
	}
	wg.Wait()
	if got := source.count(); got > 2 {
		t.Fatalf("unverified token amplification: fetches=%d", got)
	}
	for i := 0; i < 20; i++ {
		unique := enc(fmt.Sprintf(`{"alg":"RS256","kid":"unique-%d"}`, i)) + "." + enc(`{"nonce":"synthetic"}`) + "." + enc("invalid")
		if _, err := v.Verify(context.Background(), "google", unique, testAudience, "synthetic"); err == nil {
			t.Fatal("unique fake kid admitted")
		}
	}
	if got := source.count(); got > 2 {
		t.Fatalf("unique kids amplified fetches: %d", got)
	}
}

type reviewKeySource func(context.Context, string) ([]JSONWebKey, error)

func (f reviewKeySource) Fetch(ctx context.Context, issuer string) ([]JSONWebKey, error) {
	return f(ctx, issuer)
}

func TestIssuerNetworkIsolationAndCancelledWait(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	cache := &CachedKeys{TTL: time.Hour, Source: reviewKeySource(func(ctx context.Context, issuer string) ([]JSONWebKey, error) {
		if issuer == "slow" {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []JSONWebKey{{Kid: issuer}}, nil
	})}
	done := make(chan error, 1)
	go func() { _, err := cache.Fetch(context.Background(), "slow"); done <- err }()
	<-started
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.Fetch(ctx, "slow"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled wait did not stop", err)
	}
	fastDone := make(chan error, 1)
	go func() { _, err := cache.Fetch(context.Background(), "fast"); fastDone <- err }()
	select {
	case err := <-fastDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("one issuer blocked another")
	}
}

func TestProviderOutageHasRetryAdmission(t *testing.T) {
	now := time.Now()
	var fetches atomic.Int32
	cache := &CachedKeys{TTL: time.Hour, Now: func() time.Time { return now }, Source: reviewKeySource(func(context.Context, string) ([]JSONWebKey, error) {
		fetches.Add(1)
		return nil, errors.New("synthetic outage")
	})}
	for i := 0; i < 20; i++ {
		if _, err := cache.Fetch(context.Background(), "configured"); err == nil {
			t.Fatal("outage authorized")
		}
	}
	if fetches.Load() != 1 {
		t.Fatal("outage fetch storm", fetches.Load())
	}
	now = now.Add(ForcedRefreshInterval + time.Second)
	if _, err := cache.Fetch(context.Background(), "configured"); err == nil {
		t.Fatal("outage authorized")
	}
	if fetches.Load() != 2 {
		t.Fatal("recovery admission missing", fetches.Load())
	}
}
