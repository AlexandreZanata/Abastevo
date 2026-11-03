package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
)

type reviewBundleSpec struct {
	Directory       string `json:"directory"`
	RightsSHA256    string `json:"rights_sha256"`
	ManifestSHA256  string `json:"manifest_sha256"`
	GeographySHA256 string `json:"geography_sha256"`
	BoundarySHA256  string `json:"boundary_sha256"`
}

func runReview(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("charging-catalog review", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	config := flags.String("config", "", "owner-pinned review configuration")
	reportDir := flags.String("output-dir", "", "private directory for review reports")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *config == "" {
		return errors.New("expected review --config FILE [--output-dir DIR]")
	}
	f, err := os.Open(*config)
	if err != nil {
		return errors.New("review configuration unavailable")
	}
	defer f.Close()
	raw, err := readBoundedRegularFile(f, 16<<10)
	if err != nil {
		return err
	}
	var spec struct {
		Candidate reviewBundleSpec  `json:"candidate"`
		Baseline  *reviewBundleSpec `json:"baseline,omitempty"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&spec) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("review configuration refused")
	}
	candidateRoot, candidate, err := openReviewBundle(spec.Candidate)
	if err != nil {
		return err
	}
	defer candidateRoot.Close()
	conflated, err := fs.Sub(candidateRoot.FS(), "conflated/generation")
	if err != nil {
		return err
	}
	normalized, err := fs.Sub(candidateRoot.FS(), "normalized/generation")
	if err != nil {
		return err
	}
	inventory, err := prepared.VerifyNationalSourceInventory(conflated, normalized, candidate.ManifestSHA256)
	if err != nil {
		return err
	}
	var baseline *prepared.NationalImport
	if spec.Baseline != nil {
		root, input, err := openReviewBundle(*spec.Baseline)
		if err != nil {
			return err
		}
		defer root.Close()
		baseline = &input
	}
	report, err := prepared.CompareNationalCatalogs(baseline, candidate, inventory)
	if err != nil {
		return err
	}
	if *reportDir == "" {
		err = json.NewEncoder(output).Encode(report)
	} else {
		err = writeReviewReport(*reportDir, report)
	}
	if err != nil {
		return err
	}
	if len(report.Review) != 0 {
		return errors.New("catalog changes require owner review")
	}
	return nil
}

func openReviewBundle(spec reviewBundleSpec) (*os.Root, prepared.NationalImport, error) {
	if !filepath.IsAbs(spec.Directory) {
		return nil, prepared.NationalImport{}, errors.New("review bundle requires an absolute directory")
	}
	root, err := os.OpenRoot(spec.Directory)
	if err != nil {
		return nil, prepared.NationalImport{}, errors.New("review bundle unavailable")
	}
	f, err := root.Open("geography.json")
	if err != nil {
		root.Close()
		return nil, prepared.NationalImport{}, errors.New("review geography unavailable")
	}
	geography, err := readBoundedRegularFile(f, 64<<20)
	f.Close()
	if err != nil {
		root.Close()
		return nil, prepared.NationalImport{}, err
	}
	rights, err := fs.Sub(root.FS(), "rights")
	if err != nil {
		root.Close()
		return nil, prepared.NationalImport{}, err
	}
	return root, prepared.NationalImport{
		Files: rights, RightsSHA256: spec.RightsSHA256, ManifestSHA256: spec.ManifestSHA256,
		Geography: geography, GeographySHA256: spec.GeographySHA256, BoundarySHA256: spec.BoundarySHA256,
	}, nil
}

func readBoundedRegularFile(file *os.File, limit int64) ([]byte, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.New("local file size or type refused")
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, errors.New("local file read refused")
	}
	return raw, nil
}

func writeReviewReport(dir string, report prepared.NationalCatalogDiff) error {
	prefix := time.Now().UTC().Format("20060102T150405Z") + "-" + report.CandidateSHA256 + "-"
	file, err := os.CreateTemp(dir, "."+prefix)
	if err != nil {
		return errors.New("review report directory unavailable")
	}
	defer os.Remove(file.Name())
	writeErr := json.NewEncoder(file).Encode(report)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if closeErr := file.Close(); writeErr != nil || closeErr != nil {
		return errors.New("review report write failed")
	}
	name := strings.TrimPrefix(filepath.Base(file.Name()), ".") + ".json"
	if err := os.Rename(file.Name(), filepath.Join(dir, name)); err != nil {
		return errors.New("review report publication failed")
	}
	return nil
}
