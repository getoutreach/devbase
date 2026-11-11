// Copyright 2026 Outreach Corporation. All Rights Reserved.

// Description: Decodes YAML and rejects unknown keys.

// Package strictyaml decodes YAML and rejects unknown keys, so that
// release configuration loaders fail closed.
package strictyaml

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Decode decodes YAML into v and rejects unknown keys.
func Decode(b []byte, v any) error {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(v)
}

// DecodeNode decodes a YAML node into v and rejects unknown keys. A nil
// node leaves v unchanged. yaml.Node.Decode has no option for unknown
// keys, so the node is encoded and decoded again.
func DecodeNode(n *yaml.Node, v any) error {
	if n == nil {
		return nil
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return fmt.Errorf("re-encode YAML node: %w", err)
	}
	return Decode(b, v)
}
