package enterpriseexport

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Snapshot struct {
	Stations  []Station
	Revisions []Revision
	Prices    []Price
}

// ReadSnapshot decodes the deterministic JSONL payload of a candidate
// directory. The caller owns the byte/row budget; this function does not add a
// second unbounded source.
func ReadSnapshot(dir string) (Snapshot, error) {
	var snapshot Snapshot
	if err := decodeJSONL(filepath.Join(dir, "stations.jsonl"), func(dec *json.Decoder) error {
		var row Station
		if err := dec.Decode(&row); err != nil {
			return err
		}
		snapshot.Stations = append(snapshot.Stations, row)
		return nil
	}); err != nil {
		return Snapshot{}, err
	}
	if err := decodeJSONL(filepath.Join(dir, "revisions.jsonl"), func(dec *json.Decoder) error {
		var row Revision
		if err := dec.Decode(&row); err != nil {
			return err
		}
		snapshot.Revisions = append(snapshot.Revisions, row)
		return nil
	}); err != nil {
		return Snapshot{}, err
	}
	if err := decodeJSONL(filepath.Join(dir, "prices.jsonl"), func(dec *json.Decoder) error {
		var row Price
		if err := dec.Decode(&row); err != nil {
			return err
		}
		snapshot.Prices = append(snapshot.Prices, row)
		return nil
	}); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func decodeJSONL(path string, decode func(*json.Decoder) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	for {
		err := decode(dec)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("export: decode %s: %w", filepath.Base(path), err)
		}
	}
}
