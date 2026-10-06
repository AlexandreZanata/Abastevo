//go:build integration

package adapters

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func freshVerifyDB(t *testing.T) (*pgxpool.Pool, ClaimStore, ProofStore, string) {
	t.Helper()
	adminDSN := os.Getenv("ANPFUEL_TEST_DATABASE_URL")
	if adminDSN == "" {
		adminDSN = "postgres://anpfuel:anpfuel@127.0.0.1:5434/anpfuel?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer admin.Close(ctx)
	name := fmt.Sprintf("verify_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		admin, err := pgx.Connect(ctx, adminDSN)
		if err != nil {
			t.Errorf("admin connect for drop: %v", err)
			return
		}
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Errorf("drop database: %v", err)
		}
	})
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	parsed.Path = "/" + name
	dsn := parsed.String()
	if _, err := migrate.Apply(ctx, dsn, dbmigrations.Files); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := stationprofile.New(pool)
	var stationID string
	if err := pool.QueryRow(ctx, "INSERT INTO directory_stations (id, display_name) VALUES (gen_random_uuid(), 'Posto Verifica') RETURNING id::text").Scan(&stationID); err != nil {
		t.Fatalf("seed station: %v", err)
	}
	return pool, ClaimStore{Q: queries}, ProofStore{Q: queries}, stationID
}

type memoryBytes struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (m *memoryBytes) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.objects == nil {
		m.objects = map[string][]byte{}
	}
	m.objects[key] = body
	return nil
}

func (m *memoryBytes) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memoryBytes) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("missing object")
	}
	return body, nil
}

func syntheticPKI(t *testing.T) (leafPEM, caPEM []byte, roots *x509.CertPool, at time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(21), Subject: pkix.Name{CommonName: "INT-TEST Root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0),
		IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(22), Subject: pkix.Name{CommonName: "INT-TEST Signer"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(0, 6, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf: %v", err)
	}
	roots = x509.NewCertPool()
	roots.AddCert(ca)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		roots, now
}

type knownGoodRevocation struct{}

func (knownGoodRevocation) Revoked(string) (bool, bool) { return false, true }

func TestVerifyIntegrationValidatesAndConsumes(t *testing.T) {
	_, claimStore, proofStore, stationID := freshVerifyDB(t)
	ctx := context.Background()
	n := 0
	newID := func() (string, error) {
		n++
		return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil
	}
	clock := func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }
	claimPorts := application.ClaimPorts{
		Store: claimStore, Clock: clock, NewID: newID,
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
	claim, created, err := application.OpenClaim(ctx, claimPorts, "11111111-1111-4111-8111-111111111111", stationID, "administrator", []string{"profile.edit"}, "verify-int-1")
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	declaration, err := claimStore.ActiveDeclaration(ctx, claim.ID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	// The signed document embeds the exact declaration text, exactly
	// as operator extraction would deliver it.
	doc := append([]byte("%PDF-1.7\n"), []byte(declaration.Declaration)...)
	doc = append(doc, make([]byte, 128)...)
	leafPEM, caPEM, roots, at := syntheticPKI(t)
	_ = at
	proofPorts := application.ProofPorts{
		Proofs: proofStore, Claims: claimStore,
		Clock: clock, NewID: newID, Bytes: &memoryBytes{},
	}
	proof, created, err := application.SubmitProof(ctx, proofPorts, "11111111-1111-4111-8111-111111111111", claim.ID, declaration.ID, "autorizacao.pdf", "application/pdf", "authorization", doc)
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", proof, created, err)
	}
	verifyPorts := application.VerifyPorts{
		Proofs: proofStore, Claims: claimStore,
		Bytes:   proofPorts.Bytes.(*memoryBytes),
		Roots:   func() (*x509.CertPool, error) { return roots, nil },
		Revoker: knownGoodRevocation{},
		Clock:   clock,
		Certs: func(context.Context, string) ([][]byte, error) {
			return [][]byte{leafPEM, caPEM}, nil
		},
	}
	result, err := application.VerifyProof(ctx, verifyPorts, proof.ID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Outcome != verify.OutcomeValid {
		t.Fatalf("result = %+v (synthetic PKI proves logic, not real trust)", result)
	}
	stored, err := proofStore.GetProof(ctx, proof.ID)
	if err != nil || stored.Status != "verified" {
		t.Fatalf("stored = %+v, err = %v", stored, err)
	}
}
