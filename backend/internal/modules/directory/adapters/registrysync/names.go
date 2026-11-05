package registrysync

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const nameFileCap = 64 << 20

type namePin struct {
	Source    string `json:"source"`
	Profile   string `json:"profile"`
	File      string `json:"file"`
	Reference string `json:"reference"`
	Edition   string `json:"edition"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	Rows      int    `json:"rows"`
}
type nameDescriptor struct {
	Format string    `json:"format_version"`
	Inputs []namePin `json:"inputs"`
}

func readNameDescriptor(path string) ([]byte, nameDescriptor, error) {
	var descriptor nameDescriptor
	raw, err := smallFile(path, 128<<10)
	if err != nil {
		return nil, descriptor, fmt.Errorf("name descriptor unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&descriptor) != nil || descriptor.Format != "registry-name-inputs-v1" || len(descriptor.Inputs) > 2 {
		return nil, descriptor, fmt.Errorf("invalid name descriptor")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, descriptor, fmt.Errorf("trailing name descriptor")
	}
	seenSource, seenFile := map[string]bool{}, map[string]bool{}
	for _, pin := range descriptor.Inputs {
		profile := map[string]string{"rfb": "rfb-station-names-v1", "anp-survey": "anp-station-price-v1"}[pin.Source]
		if profile == "" || pin.Profile != profile || seenSource[pin.Source] || seenFile[pin.File] ||
			!regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(pin.File) || pin.File == "manifest.json" ||
			!validHash(pin.SHA256) || pin.Bytes < 0 || pin.Bytes > nameFileCap || pin.Rows < 0 || pin.Rows > 500000 ||
			pin.Reference == "" || len(pin.Reference) > 2048 || pin.Edition == "" || len(pin.Edition) > 256 {
			return nil, descriptor, fmt.Errorf("invalid name input pin")
		}
		seenSource[pin.Source], seenFile[pin.File] = true, true
	}
	return raw, descriptor, nil
}

// Pin the actual copied bytes, so changing the mutable input cannot rebind a
// pending refresh. The preparer validates JSONL identities and row counts.
func freezeNameInputs(path, dir string) (string, error) {
	raw, descriptor, err := readNameDescriptor(path)
	if err != nil {
		return "", err
	}
	names := filepath.Join(dir, "names")
	if err = os.Mkdir(names, 0700); err != nil {
		return "", err
	}
	for _, pin := range descriptor.Inputs {
		source := filepath.Join(filepath.Dir(path), pin.File)
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() || info.Size() != pin.Bytes {
			return "", fmt.Errorf("unsafe name input file")
		}
		input, err := os.Open(source)
		if err != nil {
			return "", err
		}
		output, err := os.OpenFile(filepath.Join(names, pin.File), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return "", err
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(input, nameFileCap+1))
		input.Close()
		syncErr := output.Sync()
		closeErr := output.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil || written != pin.Bytes || hex.EncodeToString(hash.Sum(nil)) != pin.SHA256 {
			return "", fmt.Errorf("copied name input checksum mismatch")
		}
	}
	if err = atomicFile(names, "manifest.json", raw); err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func verifyNameInputs(dir, expected string) error {
	path := filepath.Join(dir, "names", "manifest.json")
	raw, descriptor, err := readNameDescriptor(path)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != expected {
		return fmt.Errorf("frozen name descriptor checksum mismatch")
	}
	for _, pin := range descriptor.Inputs {
		file := filepath.Join(filepath.Dir(path), pin.File)
		digest, err := fileHash(file, nameFileCap)
		info, statErr := os.Lstat(file)
		if err != nil || statErr != nil || digest != pin.SHA256 || info.Size() != pin.Bytes {
			return fmt.Errorf("frozen name input checksum mismatch")
		}
	}
	return nil
}
