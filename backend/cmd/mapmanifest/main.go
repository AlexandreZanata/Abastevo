// Command mapmanifest builds and validates one native-map manifest from
// already-produced packages. It never downloads, generates or serves maps.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type bboxArg struct {
	minLon, minLat, maxLon, maxLat float64
}

func (b *bboxArg) String() string {
	return fmt.Sprintf("%g,%g,%g,%g", b.minLon, b.minLat, b.maxLon, b.maxLat)
}

func (b *bboxArg) Set(value string) error {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return errors.New("bbox requires min_lon,min_lat,max_lon,max_lat")
	}
	for i, dst := range []*float64{&b.minLon, &b.minLat, &b.maxLon, &b.maxLat} {
		if _, err := fmt.Sscanf(parts[i], "%f", dst); err != nil {
			return fmt.Errorf("invalid bbox value %q", parts[i])
		}
	}
	return nil
}

func digest(path string) (int64, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	hasher := sha256.New()
	bytes, err := io.Copy(hasher, file)
	if err != nil {
		return 0, "", err
	}
	return bytes, hex.EncodeToString(hasher.Sum(nil)), nil
}

func split(value string, parts int) ([]string, error) {
	result := strings.SplitN(value, "=", parts)
	if len(result) != parts || strings.TrimSpace(result[0]) == "" {
		return nil, fmt.Errorf("invalid value %q", value)
	}
	return result, nil
}

type config struct {
	output, generation, engineSHA, generatorSHA, formatVersion string
	regionID, regionName, overview, rightsLicense, attribution string
	generatedAt                                                string
	bbox                                                       bboxArg
	packages                                                   []string
	dependencies                                               []string
	sources                                                    []string
	replace                                                    bool
}

func build(args config) ([]byte, error) {
	// The Android parser requires an array; a nil slice would encode as null.
	dependencies := args.dependencies
	if dependencies == nil {
		dependencies = []string{}
	}
	manifest := mapcontract.NativeManifest{
		Format:     mapcontract.ManifestFormat,
		Version:    mapcontract.Version,
		Generation: args.generation,
		Engine: mapcontract.Engine{
			Name:          "comaps",
			UpstreamSHA:   args.engineSHA,
			GeneratorSHA:  args.generatorSHA,
			FormatVersion: args.formatVersion,
		},
		Region: mapcontract.Region{
			ID:       args.regionID,
			Name:     args.regionName,
			Overview: args.overview,
			BBox: mapcontract.BBox{
				MinLonDeg: args.bbox.minLon,
				MinLatDeg: args.bbox.minLat,
				MaxLonDeg: args.bbox.maxLon,
				MaxLatDeg: args.bbox.maxLat,
			},
		},
		Dependencies: dependencies,
		Rights: mapcontract.ManifestRights{
			License:        args.rightsLicense,
			Attribution:    args.attribution,
			Commercial:     true,
			Offline:        true,
			Redistribution: true,
		},
		GeneratedAt: args.generatedAt,
	}
	for _, raw := range args.packages {
		fields, err := split(raw, 4)
		if err != nil {
			return nil, err
		}
		bytes, hash, err := digest(fields[3])
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", fields[0], err)
		}
		manifest.Packages = append(manifest.Packages, mapcontract.Package{
			ID: fields[0], Role: fields[1], Path: fields[2], Bytes: bytes, SHA256: hash,
		})
	}
	for _, raw := range args.sources {
		fields, err := split(raw, 3)
		if err != nil {
			return nil, err
		}
		_, hash, err := digest(fields[2])
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", fields[0], err)
		}
		manifest.Sources = append(manifest.Sources, mapcontract.Source{
			ID: fields[0], License: fields[1], Attribution: args.attribution, SHA256: hash,
		})
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	raw = append(raw, '\n')
	if _, err := mapcontract.ParseNativeManifest(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func writeAtomic(path string, raw []byte, replace bool) error {
	if _, err := os.Stat(path); err == nil && !replace {
		return errors.New("output exists; pass --replace to supersede it")
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".mapmanifest-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func run(args []string) error {
	var config config
	var packages, dependencies, sources stringList
	flags := flag.NewFlagSet("mapmanifest", flag.ContinueOnError)
	flags.StringVar(&config.output, "out", "", "output manifest path")
	flags.StringVar(&config.generation, "generation", "", "generation identifier")
	flags.StringVar(&config.engineSHA, "engine-sha", "", "pinned CoMaps upstream SHA")
	flags.StringVar(&config.generatorSHA, "generator-sha", "", "pinned generator SHA")
	flags.StringVar(&config.formatVersion, "format-version", "", "native map format version")
	flags.StringVar(&config.regionID, "region-id", "", "lowercase region id")
	flags.StringVar(&config.regionName, "region-name", "", "human region name")
	flags.StringVar(&config.overview, "overview", "world", "overview package id")
	flags.StringVar(&config.rightsLicense, "rights-license", "", "data license")
	flags.StringVar(&config.attribution, "attribution", "", "required attribution")
	flags.StringVar(&config.generatedAt, "generated-at", time.Now().UTC().Format(time.RFC3339), "RFC3339 generation time")
	flags.Var(&config.bbox, "bbox", "min_lon,min_lat,max_lon,max_lat")
	flags.Var(&packages, "package", "id=role=manifestPath=localPath (repeatable)")
	flags.Var(&dependencies, "dependency", "package id (repeatable)")
	flags.Var(&sources, "source", "id=license=localPath (repeatable)")
	flags.BoolVar(&config.replace, "replace", false, "atomically replace an existing manifest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if config.output == "" || config.generation == "" || config.engineSHA == "" ||
		config.generatorSHA == "" || config.formatVersion == "" || config.regionID == "" ||
		config.regionName == "" || config.rightsLicense == "" || config.attribution == "" {
		return errors.New("out, generation, engine-sha, generator-sha, format-version, region-id, region-name, rights-license and attribution are required")
	}
	config.packages = packages
	config.dependencies = dependencies
	config.sources = sources
	raw, err := build(config)
	if err != nil {
		return err
	}
	return writeAtomic(config.output, raw, config.replace)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mapmanifest:", err)
		os.Exit(2)
	}
}
