package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a manifest from path. It does not validate
// cross-references or value ranges; call Validate for that.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("parse manifest: manifest is empty")
		}
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	// Reject any further document in the stream.
	var extra yaml.Node
	switch err := dec.Decode(&extra); {
	case err == nil:
		return nil, errors.New("parse manifest: multiple YAML documents; only one manifest per file")
	case !errors.Is(err, io.EOF):
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}
