package prepared

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
)

const (
	rightsFile   = "rights.json"
	offerNotice  = "derived-database-offer.json"
	offerData    = "derived-database.ndjson"
	receiptLimit = 8 << 20
	offerLimit   = 512 << 20
)

// NationalBundle carries the offer bytes that the importer must parse.
type NationalBundle struct {
	RightsSHA256 string
	OfferSHA256  string
	Catalog      []byte
	Sites        int64
}

type nationalArtifact struct {
	Path string `json:"path"`
	ArtifactDescriptor
}

type nationalRightsReceipt struct {
	Format         string `json:"format"`
	CatalogSHA256  string `json:"catalog_sha256"`
	ManifestSHA256 string `json:"catalog_manifest_sha256"`
	Sites          int64  `json:"catalog_sites"`
	RequiresOffer  bool   `json:"requires_derived_database_offer"`
	Offer          *struct {
		Data   nationalArtifact `json:"data"`
		Notice nationalArtifact `json:"notice"`
	} `json:"offer"`
}

// VerifyNationalBundle admits a rights receipt pinned after the local Rust
// verifier checked source rights. Its caller supplies the trusted digests,
// never hashes asserted by an untrusted import request.
func VerifyNationalBundle(files fs.FS, rightsSHA256, manifestSHA256 string) (NationalBundle, error) {
	if !validHash(rightsSHA256) || !validHash(manifestSHA256) {
		return NationalBundle{}, errors.New("prepared: national rights pins required")
	}
	entries, err := fs.ReadDir(files, ".")
	if err != nil || len(entries) != 3 {
		return NationalBundle{}, errors.New("prepared: incomplete national rights bundle")
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || (entry.Name() != rightsFile && entry.Name() != offerNotice && entry.Name() != offerData) {
			return NationalBundle{}, errors.New("prepared: unexpected national rights artifact")
		}
	}
	raw, err := readNationalArtifact(files, rightsFile, rightsSHA256, receiptLimit)
	if err != nil {
		return NationalBundle{}, err
	}
	var receipt nationalRightsReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil ||
		receipt.Format != "abastevo.ev-generation-rights-v1" || receipt.ManifestSHA256 != manifestSHA256 ||
		receipt.Sites < 1 || receipt.Sites > 205000 || !receipt.RequiresOffer || receipt.Offer == nil {
		return NationalBundle{}, errors.New("prepared: national rights receipt refused")
	}
	data, notice := receipt.Offer.Data, receipt.Offer.Notice
	if data.Path != offerData || data.SHA256 != receipt.CatalogSHA256 || data.Records != receipt.Sites ||
		data.Bytes < 1 || data.Bytes > offerLimit ||
		notice.Path != offerNotice || notice.Bytes < 1 || notice.Bytes > receiptLimit {
		return NationalBundle{}, errors.New("prepared: national offer descriptors refused")
	}
	noticeBytes, err := readNationalArtifact(files, offerNotice, notice.SHA256, notice.Bytes)
	if err != nil {
		return NationalBundle{}, err
	}
	var terms struct {
		Format  string           `json:"format"`
		License string           `json:"license"`
		Data    nationalArtifact `json:"data"`
	}
	if err := json.Unmarshal(noticeBytes, &terms); err != nil || int64(len(noticeBytes)) != notice.Bytes ||
		terms.Format != "abastevo.ev-derived-database-offer-v1" || terms.License != "ODbL-1.0" || terms.Data != data {
		return NationalBundle{}, errors.New("prepared: national offer terms refused")
	}
	catalog, err := readNationalArtifact(files, offerData, data.SHA256, data.Bytes)
	if err != nil {
		return NationalBundle{}, err
	}
	if int64(len(catalog)) != data.Bytes || catalog[len(catalog)-1] != '\n' ||
		int64(bytes.Count(catalog, []byte{'\n'})) != data.Records {
		return NationalBundle{}, errors.New("prepared: national offer framing refused")
	}
	return NationalBundle{RightsSHA256: rightsSHA256, OfferSHA256: data.SHA256, Catalog: catalog, Sites: receipt.Sites}, nil
}

func readNationalArtifact(files fs.FS, name, digest string, limit int64) ([]byte, error) {
	if !validHash(digest) {
		return nil, errors.New("prepared: invalid national artifact hash")
	}
	file, err := files.Open(name)
	if err != nil {
		return nil, errors.New("prepared: national artifact unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("prepared: national artifact size refused")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("prepared: national artifact read refused")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("prepared: national artifact hash mismatch")
	}
	return raw, nil
}
