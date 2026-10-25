// Command mapverify checks a native-map manifest and its local package files.
// It is a read-only release/activation gate: no download, generation or write.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

type result struct {
	Packages int   `json:"packages"`
	Bytes    int64 `json:"bytes"`
}

func safeJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("unsafe package path %q", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("unsafe package path %q", name)
	}
	full := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, full)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("package escapes root: %q", name)
	}
	return full, nil
}

func verify(manifestPath, root string) (result, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return result{}, err
	}
	manifest, err := mapcontract.ParseNativeManifest(raw)
	if err != nil {
		return result{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return result{}, err
	}
	ids := map[string]bool{}
	for _, pkg := range manifest.Packages {
		ids[pkg.ID] = true
	}
	for _, dependency := range manifest.Dependencies {
		if !ids[dependency] {
			return result{}, fmt.Errorf("missing dependency %q", dependency)
		}
	}
	var out result
	for _, pkg := range manifest.Packages {
		path, err := safeJoin(root, pkg.Path)
		if err != nil {
			return result{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return result{}, fmt.Errorf("%s: %w", pkg.ID, err)
		}
		if !info.Mode().IsRegular() {
			return result{}, fmt.Errorf("%s: not a regular file", pkg.ID)
		}
		if info.Size() != pkg.Bytes {
			return result{}, fmt.Errorf("%s: bytes=%d want %d", pkg.ID, info.Size(), pkg.Bytes)
		}
		file, err := os.Open(path)
		if err != nil {
			return result{}, err
		}
		hasher := sha256.New()
		bytes, err := io.Copy(hasher, file)
		_ = file.Close()
		if err != nil {
			return result{}, err
		}
		if bytes != pkg.Bytes {
			return result{}, fmt.Errorf("%s: read %d bytes", pkg.ID, bytes)
		}
		if hex.EncodeToString(hasher.Sum(nil)) != pkg.SHA256 {
			return result{}, fmt.Errorf("%s: sha256 mismatch", pkg.ID)
		}
		out.Packages++
		out.Bytes += pkg.Bytes
	}
	return out, nil
}

func run(args []string) error {
	flags := flag.NewFlagSet("mapverify", flag.ContinueOnError)
	manifest := flags.String("manifest", "", "native-map manifest path")
	root := flags.String("root", ".", "directory containing manifest package paths")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *manifest == "" {
		return errors.New("--manifest is required")
	}
	out, err := verify(*manifest, *root)
	if err != nil {
		return err
	}
	fmt.Printf("{\"packages\":%d,\"bytes\":%d}\n", out.Packages, out.Bytes)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mapverify:", err)
		os.Exit(2)
	}
}
