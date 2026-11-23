// Package appstore verifies App Store purchases with the App Store Server API
// and App Store signed data (JWS). Trust comes only from the x5c chain to the
// pinned Apple Root CA - G3; no key or URL is read from a payload.
package appstore

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"

	_ "embed"
)

// appleRootG3 is Apple Root CA - G3 from
// https://www.apple.com/certificateauthority/AppleRootCA-G3.cer, SHA-256
// 63343ABFB89A6A03EBB57E9B3F5FA7BE7C4F5C756F3017B3A8C488C3653E9179.
//
//go:embed AppleRootCA-G3.cer
var appleRootG3 []byte

var (
	// oidWWDRIntermediate marks Apple Worldwide Developer Relations CAs.
	oidWWDRIntermediate = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}
	// oidStoreReceiptLeaf marks the App Store receipt signing certificate,
	// as checked by Apple's App Store Server Library.
	oidStoreReceiptLeaf = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}

	errSignedData = errors.New("appstore: invalid signed data")
)

// maxSignedBytes bounds one compact JWS; Apple payloads are a few KiB.
const maxSignedBytes = 32 << 10

// AppleRoots returns a pool holding only the pinned Apple Root CA - G3.
func AppleRoots() (*x509.CertPool, error) {
	root, err := x509.ParseCertificate(appleRootG3)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return pool, nil
}

// SignedDataVerifier checks App Store compact JWS values.
type SignedDataVerifier struct {
	Roots *x509.CertPool
	Now   func() time.Time
}

func hasExtension(cert *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, ext := range cert.Extensions {
		if ext.Id.Equal(oid) {
			return true
		}
	}
	return false
}

// Verify returns the JSON payload of a compact JWS signed by an App Store
// leaf certificate that chains to the pinned root at verification time.
func (v SignedDataVerifier) Verify(compact string) ([]byte, error) {
	if v.Roots == nil || len(compact) == 0 || len(compact) > maxSignedBytes {
		return nil, errSignedData
	}
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, errSignedData
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errSignedData
	}
	var header struct {
		Alg string   `json:"alg"`
		X5C []string `json:"x5c"`
	}
	if json.Unmarshal(rawHeader, &header) != nil || header.Alg != "ES256" || len(header.X5C) != 3 {
		return nil, errSignedData
	}
	certs := make([]*x509.Certificate, 0, 3)
	for _, encoded := range header.X5C {
		der, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errSignedData
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, errSignedData
		}
		certs = append(certs, cert)
	}
	leaf, intermediate := certs[0], certs[1]
	if !hasExtension(leaf, oidStoreReceiptLeaf) || !hasExtension(intermediate, oidWWDRIntermediate) {
		return nil, errSignedData
	}
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	intermediates := x509.NewCertPool()
	intermediates.AddCert(intermediate)
	// The chain must end in the pinned root; the root sent in x5c is not trusted.
	if _, err = leaf.Verify(x509.VerifyOptions{
		Roots:         v.Roots,
		Intermediates: intermediates,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, errSignedData
	}
	key, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errSignedData
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(signature) != 64 {
		return nil, errSignedData
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		return nil, errSignedData
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errSignedData
	}
	return payload, nil
}

// Transaction is the subset of JWSTransactionDecodedPayload used to grant
// access. Dates are Unix milliseconds.
type Transaction struct {
	TransactionID         string `json:"transactionId"`
	OriginalTransactionID string `json:"originalTransactionId"`
	BundleID              string `json:"bundleId"`
	ProductID             string `json:"productId"`
	ExpiresDate           int64  `json:"expiresDate"`
	RevocationDate        int64  `json:"revocationDate"`
	Type                  string `json:"type"`
	AppAccountToken       string `json:"appAccountToken"`
	InAppOwnershipType    string `json:"inAppOwnershipType"`
	Environment           string `json:"environment"`
	SignedDate            int64  `json:"signedDate"`
}

// RenewalInfo is the subset of JWSRenewalInfoDecodedPayload used to map
// renewal and grace state.
type RenewalInfo struct {
	OriginalTransactionID  string `json:"originalTransactionId"`
	AutoRenewStatus        int    `json:"autoRenewStatus"`
	GracePeriodExpiresDate int64  `json:"gracePeriodExpiresDate"`
	Environment            string `json:"environment"`
}

// Notification is the subset of ResponseBodyV2DecodedPayload the server
// needs to schedule a refresh.
type Notification struct {
	NotificationType string `json:"notificationType"`
	Subtype          string `json:"subtype"`
	NotificationUUID string `json:"notificationUUID"`
	Version          string `json:"version"`
	SignedDate       int64  `json:"signedDate"`
	Data             *struct {
		AppAppleID            int64  `json:"appAppleId"`
		BundleID              string `json:"bundleId"`
		Environment           string `json:"environment"`
		SignedTransactionInfo string `json:"signedTransactionInfo"`
	} `json:"data"`
}

func (v SignedDataVerifier) decode(compact string, dst any) error {
	payload, err := v.Verify(compact)
	if err != nil {
		return err
	}
	if json.Unmarshal(payload, dst) != nil {
		return errSignedData
	}
	return nil
}

// Transaction verifies and decodes a signed transaction.
func (v SignedDataVerifier) Transaction(compact string) (Transaction, error) {
	var t Transaction
	return t, v.decode(compact, &t)
}

// RenewalInfo verifies and decodes signed renewal information.
func (v SignedDataVerifier) RenewalInfo(compact string) (RenewalInfo, error) {
	var r RenewalInfo
	return r, v.decode(compact, &r)
}

// Notification verifies and decodes an App Store Server Notification V2.
func (v SignedDataVerifier) Notification(compact string) (Notification, error) {
	var n Notification
	return n, v.decode(compact, &n)
}
